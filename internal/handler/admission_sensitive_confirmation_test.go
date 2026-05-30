package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAdmissionInvitationCodeUpdateRequiresConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.ServerAdmission{}, &model.AdmissionRequest{}, &model.SensitiveOperationConfirmation{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	auditSvc := service.NewAuditService(repository.NewAuditRepo(db))
	sensitiveSvc := service.NewSensitiveOperationService(userRepo, repository.NewSensitiveOperationRepo(db), auditSvc)
	admissionSvc := service.NewAdmissionServiceWithRepo(userRepo, repository.NewAdmissionRepo(db), config.AdmissionConfig{PolicyType: "invitation", InvitationCode: "old-code"})
	admissionH := NewAdmissionHandler(admissionSvc)
	admissionH.SetAuditService(auditSvc)
	admissionH.SetSensitiveOperationService(sensitiveSvc)

	admin := &model.User{PubKeyEd25519: "admin-sensitive-pubkey", DisplayName: "Admin Sensitive", Status: "active", Role: "admin", IsAdmin: true}
	if err := userRepo.Create(admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := sensitiveSvc.SetPassword(admin.ID, "admin-device", "secret123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	secret := "admission-sensitive-secret"
	token, err := middleware.GenerateToken(secret, admin.ID, "admin-device", "agent-os-test", 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	r := gin.New()
	adminRoutes := r.Group("/api/v1/admin")
	adminRoutes.Use(middleware.JWTAuth(secret))
	adminRoutes.Use(middleware.AdminMiddleware(userRepo))
	adminRoutes.PUT("/admission/invitation-code", admissionH.UpdateInvitationCode)

	missingReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/admission/invitation-code", strings.NewReader(`{"invitation_code":"new-code"}`))
	missingReq.Header.Set("Content-Type", "application/json")
	missingReq.Header.Set("Authorization", "Bearer "+token)
	missingW := httptest.NewRecorder()
	r.ServeHTTP(missingW, missingReq)
	if missingW.Code != http.StatusBadRequest {
		t.Fatalf("expected missing confirmation 400, got %d body=%s", missingW.Code, missingW.Body.String())
	}

	wrongConfirmation, err := sensitiveSvc.IssueConfirmation(admin.ID, "admin-device", "secret123", service.SensitiveOperationDeviceRevoke)
	if err != nil {
		t.Fatalf("issue wrong confirmation: %v", err)
	}
	wrongReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/admission/invitation-code", strings.NewReader(`{"invitation_code":"new-code","confirmation_token":"`+wrongConfirmation.ConfirmationToken+`"}`))
	wrongReq.Header.Set("Content-Type", "application/json")
	wrongReq.Header.Set("Authorization", "Bearer "+token)
	wrongW := httptest.NewRecorder()
	r.ServeHTTP(wrongW, wrongReq)
	if wrongW.Code != http.StatusForbidden {
		t.Fatalf("expected wrong operation confirmation 403, got %d body=%s", wrongW.Code, wrongW.Body.String())
	}

	confirmation, err := sensitiveSvc.IssueConfirmation(admin.ID, "admin-device", "secret123", service.SensitiveOperationAdmissionInvitationUpdate)
	if err != nil {
		t.Fatalf("issue confirmation: %v", err)
	}
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/admission/invitation-code", strings.NewReader(`{"invitation_code":"new-code","confirmation_token":"`+confirmation.ConfirmationToken+`"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer "+token)
	updateW := httptest.NewRecorder()
	r.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", updateW.Code, updateW.Body.String())
	}

	reuseReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/admission/invitation-code", strings.NewReader(`{"invitation_code":"new-code-2","confirmation_token":"`+confirmation.ConfirmationToken+`"}`))
	reuseReq.Header.Set("Content-Type", "application/json")
	reuseReq.Header.Set("Authorization", "Bearer "+token)
	reuseW := httptest.NewRecorder()
	r.ServeHTTP(reuseW, reuseReq)
	if reuseW.Code != http.StatusForbidden {
		t.Fatalf("expected reused confirmation 403, got %d body=%s", reuseW.Code, reuseW.Body.String())
	}
}
