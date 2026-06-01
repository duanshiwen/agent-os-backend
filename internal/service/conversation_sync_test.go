package service

import (
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConversationServiceCreateGroupRecordsConversationCreatedForParticipants(t *testing.T) {
	svc, _, syncSvc := newConversationSyncTestService(t)
	creatorID := uuid.New()
	memberID := uuid.New()

	conv, err := svc.CreateConversation(creatorID, &CreateConversationRequest{
		Type:           "group",
		Name:           "sync group",
		ParticipantIDs: []uuid.UUID{memberID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	creatorEvents, err := syncSvc.GetEvents(creatorID, "creator-device", 100)
	if err != nil {
		t.Fatalf("get creator events: %v", err)
	}
	if len(creatorEvents) != 1 {
		t.Fatalf("expected creator conversation.created event, got %+v", creatorEvents)
	}
	assertConversationCreatedSyncEvent(t, creatorEvents[0], conv)

	memberEvents, err := syncSvc.GetEvents(memberID, "member-device", 100)
	if err != nil {
		t.Fatalf("get member events: %v", err)
	}
	if len(memberEvents) != 1 {
		t.Fatalf("expected member conversation.created event, got %+v", memberEvents)
	}
	assertConversationCreatedSyncEvent(t, memberEvents[0], conv)
}

func TestConversationServiceAddParticipantRecordsParticipantAddedForExistingAndNewParticipants(t *testing.T) {
	svc, convRepo, syncSvc := newConversationSyncTestService(t)
	convID := uuid.New()
	adminID := uuid.New()
	existingID := uuid.New()
	newID := uuid.New()
	seedConversationParticipant(t, convRepo, convID, adminID, "admin")
	if err := convRepo.AddParticipant(&model.ConversationParticipant{ConversationID: convID, UserID: existingID, Role: "member"}); err != nil {
		t.Fatalf("add existing participant: %v", err)
	}

	if err := svc.AddParticipant(convID, adminID, newID); err != nil {
		t.Fatalf("add participant: %v", err)
	}

	for _, uid := range []uuid.UUID{adminID, existingID, newID} {
		events, err := syncSvc.GetEvents(uid, uid.String()+"-device", 100)
		if err != nil {
			t.Fatalf("get events for %s: %v", uid, err)
		}
		if len(events) != 1 {
			t.Fatalf("expected participant.added for %s, got %+v", uid, events)
		}
		if events[0].EventType != "participant.added" {
			t.Fatalf("expected participant.added, got %q", events[0].EventType)
		}
		if events[0].Payload["conversation_id"] != convID.String() || events[0].Payload["user_id"] != newID.String() || events[0].Payload["role"] != "member" || events[0].Payload["added_by"] != adminID.String() {
			t.Fatalf("unexpected participant.added payload: %+v", events[0].Payload)
		}
	}
}

func TestConversationServiceLeaveConversationRecordsParticipantRemovedForRemovedAndRemainingParticipants(t *testing.T) {
	svc, convRepo, syncSvc := newConversationSyncTestService(t)
	convID := uuid.New()
	adminID := uuid.New()
	leaverID := uuid.New()
	seedConversationParticipant(t, convRepo, convID, adminID, "admin")
	if err := convRepo.AddParticipant(&model.ConversationParticipant{ConversationID: convID, UserID: leaverID, Role: "member"}); err != nil {
		t.Fatalf("add leaver participant: %v", err)
	}

	if err := svc.LeaveConversation(convID, leaverID); err != nil {
		t.Fatalf("leave conversation: %v", err)
	}

	for _, uid := range []uuid.UUID{adminID, leaverID} {
		events, err := syncSvc.GetEvents(uid, uid.String()+"-device", 100)
		if err != nil {
			t.Fatalf("get events for %s: %v", uid, err)
		}
		if len(events) != 1 {
			t.Fatalf("expected participant.removed for %s, got %+v", uid, events)
		}
		if events[0].EventType != "participant.removed" {
			t.Fatalf("expected participant.removed, got %q", events[0].EventType)
		}
		if events[0].Payload["conversation_id"] != convID.String() || events[0].Payload["user_id"] != leaverID.String() || events[0].Payload["status"] != "removed" || events[0].Payload["removed_by"] != leaverID.String() {
			t.Fatalf("unexpected participant.removed payload: %+v", events[0].Payload)
		}
	}
}

func assertConversationCreatedSyncEvent(t *testing.T, event model.SyncEvent, conv *model.Conversation) {
	t.Helper()
	if event.EventType != "conversation.created" {
		t.Fatalf("expected conversation.created, got %q", event.EventType)
	}
	if event.Payload["object_id"] != conv.ID.String() || event.Payload["conversation_id"] != conv.ID.String() {
		t.Fatalf("unexpected conversation IDs in payload: %+v", event.Payload)
	}
	if event.Payload["type"] != conv.Type || event.Payload["name"] != conv.Name || event.Payload["created_by"] != conv.CreatedBy.String() || event.Payload["created_at"] == nil || event.Payload["updated_at"] == nil {
		t.Fatalf("expected conversation reconstruction fields, got %+v", event.Payload)
	}
}

func newConversationSyncTestService(t *testing.T) (*ConversationService, *repository.ConversationRepo, *SyncService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Conversation{}, &model.ConversationParticipant{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	convRepo := repository.NewConversationRepo(db)
	userRepo := repository.NewUserRepo(db)
	syncSvc := NewSyncService(repository.NewSyncRepo(db), &captureHub{})
	convSvc := NewConversationService(convRepo, userRepo)
	convSvc.SetSyncService(syncSvc)
	return convSvc, convRepo, syncSvc
}
