package service

import (
	"net/url"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMessageServiceSendMessageRejectsNonParticipant(t *testing.T) {
	svc, convRepo, _ := newMessageTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	if err := convRepo.Create(&model.Conversation{Base: model.Base{ID: convID}, Type: "group", CreatedBy: uuid.New()}); err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	_, err := svc.SendMessage(senderID, &SendMessageRequest{
		ConversationID: convID,
		Content:        "hello",
	})
	if err == nil || err.Error() != "access denied: not a participant" {
		t.Fatalf("expected access denied, got %v", err)
	}
}

func TestMessageServiceSendMessageStoresMessageAndDefaultsType(t *testing.T) {
	svc, convRepo, _ := newMessageTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	otherID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, otherID)

	delivery, err := svc.SendMessage(senderID, &SendMessageRequest{
		ConversationID: convID,
		Content:        "hello",
		Metadata:       datatypes.JSONMap{"client_msg_id": "c1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if delivery.Message.ID == uuid.Nil || delivery.Message.Type != "text" || delivery.Message.Content != "hello" || delivery.Message.Status != "active" {
		t.Fatalf("unexpected message: %+v", delivery.Message)
	}
	if delivery.Message.Visibility["scope"] != "conversation" {
		t.Fatalf("expected default conversation visibility, got %+v", delivery.Message.Visibility)
	}
	if len(delivery.Participants) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(delivery.Participants))
	}

	msgs, err := convRepo.GetMessages(convID, 10, nil)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != delivery.Message.ID {
		t.Fatalf("expected stored message, got %+v", msgs)
	}
}

func TestMessageServiceSendMessageStoresReplyThreadVisibilityAndClientEventID(t *testing.T) {
	svc, convRepo, _ := newMessageTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	otherID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, otherID)

	root, err := svc.SendMessage(senderID, &SendMessageRequest{ConversationID: convID, Content: "root"})
	if err != nil {
		t.Fatalf("send root: %v", err)
	}
	visibility := datatypes.JSONMap{"scope": "private_to_user", "user_id": otherID.String()}
	reply, err := svc.SendMessage(senderID, &SendMessageRequest{
		ConversationID: convID,
		Content:        "reply",
		ReplyTo:        &root.Message.ID,
		Visibility:     visibility,
		ClientEventID:  "send-reply-1",
	})
	if err != nil {
		t.Fatalf("send reply: %v", err)
	}
	if reply.Message.ReplyTo == nil || *reply.Message.ReplyTo != root.Message.ID {
		t.Fatalf("expected reply_to %s, got %+v", root.Message.ID, reply.Message.ReplyTo)
	}
	if reply.Message.ThreadID != root.Message.ID.String() {
		t.Fatalf("expected thread_id to default to root message id, got %q", reply.Message.ThreadID)
	}
	if reply.Message.Visibility["scope"] != "private_to_user" || reply.Message.ClientEventID != "send-reply-1" {
		t.Fatalf("unexpected reply contract fields: %+v", reply.Message)
	}

	replayed, err := svc.SendMessage(senderID, &SendMessageRequest{ConversationID: convID, Content: "reply duplicate", ClientEventID: "send-reply-1"})
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if replayed.Message.ID != reply.Message.ID || replayed.Message.Content != "reply" {
		t.Fatalf("expected replay to return original message, got %+v", replayed.Message)
	}

	msgs, err := convRepo.GetMessages(convID, 10, nil)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected root + one reply after idempotent replay, got %+v", msgs)
	}
}

func TestMessageServiceSendMessageRejectsReplyToInAnotherConversation(t *testing.T) {
	svc, convRepo, _ := newMessageTestService(t)
	senderID := uuid.New()
	otherID := uuid.New()
	convA := uuid.New()
	convB := uuid.New()
	seedMessageConversation(t, convRepo, convA, senderID, otherID)
	seedMessageConversation(t, convRepo, convB, senderID, otherID)

	foreign, err := svc.SendMessage(senderID, &SendMessageRequest{ConversationID: convB, Content: "foreign"})
	if err != nil {
		t.Fatalf("send foreign: %v", err)
	}
	_, err = svc.SendMessage(senderID, &SendMessageRequest{ConversationID: convA, Content: "bad reply", ReplyTo: &foreign.Message.ID})
	if err == nil || err.Error() != "reply_to message is not in the same conversation" {
		t.Fatalf("expected reply_to rejection, got %v", err)
	}
}

func TestMessageServiceUpdateMessageEmitsSyncEvent(t *testing.T) {
	svc, convRepo, _, syncSvc := newMessageSyncTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	recipientID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, recipientID)

	delivery, err := svc.SendMessage(senderID, &SendMessageRequest{ConversationID: convID, Content: "before"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	updated, err := svc.UpdateMessage(senderID, convID, delivery.Message.ID, UpdateMessageRequest{Content: "after", Metadata: datatypes.JSONMap{"edited": true}, ClientEventID: "edit-1"})
	if err != nil {
		t.Fatalf("update message: %v", err)
	}
	if updated.Content != "after" || updated.EditedAt == nil {
		t.Fatalf("expected edited message, got %+v", updated)
	}

	events, err := syncSvc.GetEventsAfter(recipientID, 1, 10)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "message.updated" || events[0].Payload["content"] != "after" || events[0].Payload["edited_at"] == nil {
		t.Fatalf("expected message.updated payload, got %+v", events)
	}
}

func TestMessageServiceUpdateMessageRejectsNonSender(t *testing.T) {
	svc, convRepo, _ := newMessageTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	recipientID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, recipientID)

	delivery, err := svc.SendMessage(senderID, &SendMessageRequest{ConversationID: convID, Content: "before"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	_, err = svc.UpdateMessage(recipientID, convID, delivery.Message.ID, UpdateMessageRequest{Content: "after"})
	if err == nil || err.Error() != "access denied: only sender can edit message" {
		t.Fatalf("expected non-sender rejection, got %v", err)
	}
}

func TestMessageServiceDeleteMessageSoftDeletesAndEmitsSyncEvent(t *testing.T) {
	svc, convRepo, _, syncSvc := newMessageSyncTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	recipientID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, recipientID)

	delivery, err := svc.SendMessage(senderID, &SendMessageRequest{ConversationID: convID, Content: "delete me"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	deleted, err := svc.DeleteMessage(senderID, convID, delivery.Message.ID, "delete-1")
	if err != nil {
		t.Fatalf("delete message: %v", err)
	}
	if deleted.Status != "deleted" || deleted.DeletedAt == nil || deleted.DeletedBy == nil || *deleted.DeletedBy != senderID {
		t.Fatalf("expected soft-deleted message, got %+v", deleted)
	}

	events, err := syncSvc.GetEventsAfter(recipientID, 1, 10)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "message.deleted" || events[0].Payload["status"] != "deleted" || events[0].Payload["deleted_at"] == nil {
		t.Fatalf("expected message.deleted payload, got %+v", events)
	}

	_, err = svc.UpdateMessage(senderID, convID, delivery.Message.ID, UpdateMessageRequest{Content: "revive"})
	if err == nil || err.Error() != "message is deleted" {
		t.Fatalf("expected deleted message edit rejection, got %v", err)
	}
}

func TestMessageServiceSaveOfflineMessagesSkipsSenderAndOnlineDevices(t *testing.T) {
	svc, convRepo, userRepo := newMessageTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	recipientID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, recipientID)
	seedDevice(t, userRepo, senderID, "sender-device")
	seedDevice(t, userRepo, recipientID, "recipient-online")
	seedDevice(t, userRepo, recipientID, "recipient-offline")

	msg := &model.Message{ConversationID: convID, SenderID: senderID, Type: "text", Content: "hello"}
	if err := convRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	participants, err := convRepo.GetParticipants(convID)
	if err != nil {
		t.Fatalf("get participants: %v", err)
	}

	if err := svc.SaveOfflineMessages(msg, participants, map[string]bool{"recipient-online": true}); err != nil {
		t.Fatalf("save offline messages: %v", err)
	}

	offline, err := svc.FetchOfflineMessages(recipientID, "recipient-offline")
	if err != nil {
		t.Fatalf("fetch recipient offline messages: %v", err)
	}
	if len(offline) != 1 || offline[0].MessageID != msg.ID || offline[0].Delivered {
		t.Fatalf("unexpected offline messages: %+v", offline)
	}

	online, err := svc.FetchOfflineMessages(recipientID, "recipient-online")
	if err != nil {
		t.Fatalf("fetch recipient online messages: %v", err)
	}
	if len(online) != 0 {
		t.Fatalf("expected no offline message for online device, got %+v", online)
	}

	senderOffline, err := svc.FetchOfflineMessages(senderID, "sender-device")
	if err != nil {
		t.Fatalf("fetch sender offline messages: %v", err)
	}
	if len(senderOffline) != 0 {
		t.Fatalf("sender should not receive own offline message, got %+v", senderOffline)
	}
}

func TestMessageServiceAckOfflineMessagesMarksDelivered(t *testing.T) {
	svc, convRepo, userRepo := newMessageTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	recipientID := uuid.New()
	seedMessageConversation(t, convRepo, convID, senderID, recipientID)
	seedDevice(t, userRepo, recipientID, "recipient-offline")

	msg := &model.Message{ConversationID: convID, SenderID: senderID, Type: "text", Content: "hello"}
	if err := convRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	participants, err := convRepo.GetParticipants(convID)
	if err != nil {
		t.Fatalf("get participants: %v", err)
	}
	if err := svc.SaveOfflineMessages(msg, participants, map[string]bool{}); err != nil {
		t.Fatalf("save offline messages: %v", err)
	}

	offline, err := svc.FetchOfflineMessages(recipientID, "recipient-offline")
	if err != nil {
		t.Fatalf("fetch offline messages: %v", err)
	}
	if len(offline) != 1 {
		t.Fatalf("expected one offline message, got %d", len(offline))
	}
	if err := svc.AckOfflineMessages([]uuid.UUID{offline[0].ID}); err != nil {
		t.Fatalf("ack offline message: %v", err)
	}
	offline, err = svc.FetchOfflineMessages(recipientID, "recipient-offline")
	if err != nil {
		t.Fatalf("fetch offline messages after ack: %v", err)
	}
	if len(offline) != 0 {
		t.Fatalf("expected no undelivered messages after ack, got %+v", offline)
	}
}

func newMessageTestService(t *testing.T) (*MessageService, *repository.ConversationRepo, *repository.UserRepo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.Conversation{}, &model.ConversationParticipant{}, &model.Message{}, &model.OfflineMessage{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	convRepo := repository.NewConversationRepo(db)
	userRepo := repository.NewUserRepo(db)
	return NewMessageService(convRepo, userRepo), convRepo, userRepo
}

func seedMessageConversation(t *testing.T, convRepo *repository.ConversationRepo, convID, userA, userB uuid.UUID) {
	t.Helper()
	if err := convRepo.Create(&model.Conversation{Base: model.Base{ID: convID}, Type: "group", CreatedBy: userA}); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	for _, uid := range []uuid.UUID{userA, userB} {
		if err := convRepo.AddParticipant(&model.ConversationParticipant{
			ConversationID: convID,
			UserID:         uid,
			Role:           "member",
			JoinedAt:       time.Now(),
		}); err != nil {
			t.Fatalf("add participant: %v", err)
		}
	}
}

func seedDevice(t *testing.T, userRepo *repository.UserRepo, userID uuid.UUID, deviceID string) {
	t.Helper()
	if err := userRepo.CreateDevice(&model.Device{
		UserID:       userID,
		DeviceID:     deviceID,
		DevicePubKey: "device-pub-key-" + deviceID,
		PairedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("create device: %v", err)
	}
}
