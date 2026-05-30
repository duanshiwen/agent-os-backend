package service

import (
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSensitiveOperationServicePasswordAndConfirmation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.SensitiveOperationConfirmation{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	auditSvc := NewAuditService(repository.NewAuditRepo(db))
	svc := NewSensitiveOperationService(userRepo, repository.NewSensitiveOperationRepo(db), auditSvc)
	user := &model.User{PubKeyEd25519: "sensitive-user-pubkey", DisplayName: "Sensitive User", Status: "active"}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := svc.SetPassword(user.ID, "device-1", "secret123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if err := svc.SetPassword(user.ID, "device-1", "secret456"); err == nil || err.Error() != "password already set" {
		t.Fatalf("expected password already set, got %v", err)
	}
	if err := svc.ChangePassword(user.ID, "device-1", "wrong", "secret456"); err == nil || err.Error() != "invalid current password" {
		t.Fatalf("expected invalid current password, got %v", err)
	}
	if err := svc.ChangePassword(user.ID, "device-1", "secret123", "secret456"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	confirmation, err := svc.IssueConfirmation(user.ID, "device-1", "secret456", "device.revoke")
	if err != nil {
		t.Fatalf("issue confirmation: %v", err)
	}
	if confirmation.ConfirmationToken == "" || confirmation.Operation != "device.revoke" {
		t.Fatalf("unexpected confirmation: %+v", confirmation)
	}
	if err := svc.ConsumeConfirmation(user.ID, confirmation.ConfirmationToken, "device.revoke", "test"); err != nil {
		t.Fatalf("consume confirmation: %v", err)
	}
	if err := svc.ConsumeConfirmation(user.ID, confirmation.ConfirmationToken, "device.revoke", "test-again"); err == nil || err.Error() != "invalid or expired confirmation token" {
		t.Fatalf("expected one-time token failure, got %v", err)
	}

	page, err := auditSvc.List(ListAuditEventsInput{Limit: 20})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if page.Total < 4 {
		t.Fatalf("expected password/confirmation audit events, got %+v", page)
	}
}
