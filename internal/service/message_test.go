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
	if delivery.Message.ID == uuid.Nil || delivery.Message.Type != "text" || delivery.Message.Content != "hello" {
		t.Fatalf("unexpected message: %+v", delivery.Message)
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
