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

func TestMessageServiceSendMessageRecordsSyncEventForEveryParticipant(t *testing.T) {
	svc, convRepo, _, syncSvc := newMessageSyncTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	recipientID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, recipientID)

	delivery, err := svc.SendMessage(senderID, &SendMessageRequest{
		ConversationID: convID,
		Content:        "hello sync",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	senderEvents, err := syncSvc.GetEvents(senderID, "sender-other-device", 100)
	if err != nil {
		t.Fatalf("get sender events: %v", err)
	}
	if len(senderEvents) != 1 {
		t.Fatalf("expected one sender sync event for sender's other devices, got %+v", senderEvents)
	}
	assertMessageCreatedSyncEvent(t, senderEvents[0], delivery.Message, senderID)

	recipientEvents, err := syncSvc.GetEvents(recipientID, "recipient-device", 100)
	if err != nil {
		t.Fatalf("get recipient events: %v", err)
	}
	if len(recipientEvents) != 1 {
		t.Fatalf("expected one recipient sync event, got %+v", recipientEvents)
	}
	assertMessageCreatedSyncEvent(t, recipientEvents[0], delivery.Message, senderID)
}

func TestMessageCreatedSyncSequenceCanBeAcked(t *testing.T) {
	svc, convRepo, _, syncSvc := newMessageSyncTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	recipientID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, recipientID)

	delivery, err := svc.SendMessage(senderID, &SendMessageRequest{
		ConversationID: convID,
		Content:        "ackable sync message",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	recipientEvents, err := syncSvc.GetEvents(recipientID, "recipient-device", 100)
	if err != nil {
		t.Fatalf("get recipient events: %v", err)
	}
	if len(recipientEvents) != 1 {
		t.Fatalf("expected one recipient event, got %+v", recipientEvents)
	}
	assertMessageCreatedSyncEvent(t, recipientEvents[0], delivery.Message, senderID)

	if err := syncSvc.AckEvents(recipientID, "recipient-device", recipientEvents[0].Sequence); err != nil {
		t.Fatalf("ack recipient event: %v", err)
	}
	afterAck, err := syncSvc.GetEvents(recipientID, "recipient-device", 100)
	if err != nil {
		t.Fatalf("get recipient events after ack: %v", err)
	}
	if len(afterAck) != 0 {
		t.Fatalf("expected no cursor-path duplicate after ack, got %+v", afterAck)
	}
}

func TestMessageServiceSyncEventsComplementButDoNotReplaceOfflineDelivery(t *testing.T) {
	svc, convRepo, userRepo, syncSvc := newMessageSyncTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	recipientID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, recipientID)
	seedDevice(t, userRepo, senderID, "sender-offline-other")
	seedDevice(t, userRepo, recipientID, "recipient-offline")

	delivery, err := svc.SendMessage(senderID, &SendMessageRequest{
		ConversationID: convID,
		Content:        "hello offline and sync",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if err := svc.SaveOfflineMessages(delivery.Message, delivery.Participants, map[string]bool{}); err != nil {
		t.Fatalf("save offline messages: %v", err)
	}

	senderOffline, err := svc.FetchOfflineMessages(senderID, "sender-offline-other")
	if err != nil {
		t.Fatalf("fetch sender offline: %v", err)
	}
	if len(senderOffline) != 0 {
		t.Fatalf("sender's other devices should use sync events, not offline message delivery, got %+v", senderOffline)
	}

	recipientOffline, err := svc.FetchOfflineMessages(recipientID, "recipient-offline")
	if err != nil {
		t.Fatalf("fetch recipient offline: %v", err)
	}
	if len(recipientOffline) != 1 || recipientOffline[0].MessageID != delivery.Message.ID {
		t.Fatalf("recipient offline device should still get offline message delivery, got %+v", recipientOffline)
	}

	senderEvents, err := syncSvc.GetEvents(senderID, "sender-offline-other", 100)
	if err != nil {
		t.Fatalf("get sender sync events: %v", err)
	}
	if len(senderEvents) != 1 {
		t.Fatalf("sender's other devices should get sync event, got %+v", senderEvents)
	}
}

func assertMessageCreatedSyncEvent(t *testing.T, event model.SyncEvent, msg *model.Message, senderID uuid.UUID) {
	t.Helper()
	if event.EventType != "message.created" {
		t.Fatalf("expected message.created event, got %q", event.EventType)
	}
	if event.Payload["object_id"] != msg.ID.String() || event.Payload["conversation_id"] != msg.ConversationID.String() || event.Payload["message_id"] != msg.ID.String() {
		t.Fatalf("unexpected sync payload IDs: %+v", event.Payload)
	}
	if event.Payload["sender_id"] != senderID.String() {
		t.Fatalf("expected sender_id %s in sync payload, got %+v", senderID, event.Payload)
	}
	if event.Payload["type"] != msg.Type || event.Payload["content"] != msg.Content || event.Payload["metadata"] == nil || event.Payload["created_at"] == nil {
		t.Fatalf("expected message reconstruction fields in sync payload, got %+v", event.Payload)
	}
}

func newMessageSyncTestService(t *testing.T) (*MessageService, *repository.ConversationRepo, *repository.UserRepo, *SyncService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.Conversation{}, &model.ConversationParticipant{}, &model.Message{}, &model.OfflineMessage{}, &model.MessageReaction{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	convRepo := repository.NewConversationRepo(db)
	userRepo := repository.NewUserRepo(db)
	syncSvc := NewSyncService(repository.NewSyncRepo(db), &captureHub{})
	msgSvc := NewMessageService(convRepo, userRepo)
	msgSvc.SetSyncService(syncSvc)
	return msgSvc, convRepo, userRepo, syncSvc
}
