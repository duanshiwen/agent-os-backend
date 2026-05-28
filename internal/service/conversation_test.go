package service

import (
	"net/url"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConversationServiceAddParticipantRequiresAdminRole(t *testing.T) {
	svc, convRepo := newConversationTestService(t)
	convID := uuid.New()
	actorID := uuid.New()
	targetID := uuid.New()

	seedConversationParticipant(t, convRepo, convID, actorID, "member")

	err := svc.AddParticipant(convID, actorID, targetID)
	if err == nil || err.Error() != "access denied" {
		t.Fatalf("expected access denied, got %v", err)
	}

	isTargetParticipant, err := convRepo.IsParticipant(convID, targetID)
	if err != nil {
		t.Fatalf("check target participant: %v", err)
	}
	if isTargetParticipant {
		t.Fatal("target should not have been added by non-admin actor")
	}
}

func TestConversationServiceAddParticipantAllowsAdminRole(t *testing.T) {
	svc, convRepo := newConversationTestService(t)
	convID := uuid.New()
	actorID := uuid.New()
	targetID := uuid.New()

	seedConversationParticipant(t, convRepo, convID, actorID, "admin")

	if err := svc.AddParticipant(convID, actorID, targetID); err != nil {
		t.Fatalf("add participant: %v", err)
	}

	participant, err := convRepo.GetParticipant(convID, targetID)
	if err != nil {
		t.Fatalf("get target participant: %v", err)
	}
	if participant.Role != "member" {
		t.Fatalf("expected target role member, got %q", participant.Role)
	}
}

func TestConversationServiceAddParticipantAllowsOwnerRole(t *testing.T) {
	svc, convRepo := newConversationTestService(t)
	convID := uuid.New()
	actorID := uuid.New()
	targetID := uuid.New()

	seedConversationParticipant(t, convRepo, convID, actorID, "owner")

	if err := svc.AddParticipant(convID, actorID, targetID); err != nil {
		t.Fatalf("add participant: %v", err)
	}

	isTargetParticipant, err := convRepo.IsParticipant(convID, targetID)
	if err != nil {
		t.Fatalf("check target participant: %v", err)
	}
	if !isTargetParticipant {
		t.Fatal("expected owner to add target participant")
	}
}

func TestConversationServiceAddParticipantRejectsNonParticipant(t *testing.T) {
	svc, convRepo := newConversationTestService(t)
	convID := uuid.New()
	actorID := uuid.New()
	targetID := uuid.New()

	err := svc.AddParticipant(convID, actorID, targetID)
	if err == nil || err.Error() != "access denied" {
		t.Fatalf("expected access denied, got %v", err)
	}

	isTargetParticipant, err := convRepo.IsParticipant(convID, targetID)
	if err != nil {
		t.Fatalf("check target participant: %v", err)
	}
	if isTargetParticipant {
		t.Fatal("target should not have been added by non-participant actor")
	}
}

func newConversationTestService(t *testing.T) (*ConversationService, *repository.ConversationRepo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Conversation{}, &model.ConversationParticipant{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	convRepo := repository.NewConversationRepo(db)
	userRepo := repository.NewUserRepo(db)
	return NewConversationService(convRepo, userRepo), convRepo
}

func seedConversationParticipant(t *testing.T, convRepo *repository.ConversationRepo, convID, userID uuid.UUID, role string) {
	t.Helper()
	if err := convRepo.Create(&model.Conversation{Base: model.Base{ID: convID}, Type: "group", CreatedBy: userID}); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := convRepo.AddParticipant(&model.ConversationParticipant{
		ConversationID: convID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
	}); err != nil {
		t.Fatalf("add seed participant: %v", err)
	}
}
