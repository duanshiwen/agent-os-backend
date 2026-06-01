package service

import (
	"fmt"
	"log"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type MessageService struct {
	convRepo *repository.ConversationRepo
	userRepo *repository.UserRepo
	syncSvc  *SyncService
}

func NewMessageService(convRepo *repository.ConversationRepo, userRepo *repository.UserRepo) *MessageService {
	return &MessageService{convRepo: convRepo, userRepo: userRepo}
}

// SetSyncService injects the sync service (called after initialization to avoid cycles).
func (s *MessageService) SetSyncService(svc *SyncService) {
	s.syncSvc = svc
}

type SendMessageRequest struct {
	ConversationID uuid.UUID         `json:"conversation_id" binding:"required"`
	Type           string            `json:"type"` // text, image, voice, file, video, link_card, kb_card
	Content        string            `json:"content" binding:"required"`
	Metadata       datatypes.JSONMap `json:"metadata"`
	ReplyTo        *uuid.UUID        `json:"reply_to"`
	ThreadID       string            `json:"thread_id"`
	Visibility     datatypes.JSONMap `json:"visibility"`
	ClientEventID  string            `json:"client_event_id"`
}

type UpdateMessageRequest struct {
	Content       string            `json:"content" binding:"required"`
	Metadata      datatypes.JSONMap `json:"metadata"`
	ClientEventID string            `json:"client_event_id"`
}

type MessageDelivery struct {
	Message      *model.Message                  `json:"message"`
	Participants []model.ConversationParticipant `json:"-"`
}

// SendMessage validates, stores, and prepares a message for delivery.
func (s *MessageService) SendMessage(senderID uuid.UUID, req *SendMessageRequest) (*MessageDelivery, error) {
	isParticipant, err := s.convRepo.IsParticipant(req.ConversationID, senderID)
	if err != nil || !isParticipant {
		return nil, fmt.Errorf("access denied: not a participant")
	}

	if req.ClientEventID != "" {
		if existing, err := s.convRepo.FindMessageByClientEventID(senderID, req.ClientEventID); err == nil {
			participants, pErr := s.convRepo.GetParticipants(existing.ConversationID)
			if pErr != nil {
				return nil, fmt.Errorf("get participants: %w", pErr)
			}
			return &MessageDelivery{Message: existing, Participants: participants}, nil
		}
	}

	msgType := req.Type
	if msgType == "" {
		msgType = "text"
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = datatypes.JSONMap{}
	}
	visibility := req.Visibility
	if visibility == nil {
		visibility = datatypes.JSONMap{"scope": "conversation"}
	}
	threadID := req.ThreadID
	if req.ReplyTo != nil {
		replyTo, err := s.convRepo.GetMessageByID(*req.ReplyTo)
		if err != nil {
			return nil, fmt.Errorf("reply_to message not found")
		}
		if replyTo.ConversationID != req.ConversationID {
			return nil, fmt.Errorf("reply_to message is not in the same conversation")
		}
		if threadID == "" {
			if replyTo.ThreadID != "" {
				threadID = replyTo.ThreadID
			} else {
				threadID = replyTo.ID.String()
			}
		}
	}

	msg := &model.Message{
		ConversationID: req.ConversationID,
		SenderID:       senderID,
		Type:           msgType,
		Content:        req.Content,
		Metadata:       metadata,
		ReplyTo:        req.ReplyTo,
		ThreadID:       threadID,
		Visibility:     visibility,
		ClientEventID:  req.ClientEventID,
		Status:         "active",
	}
	if err := s.convRepo.CreateMessage(msg); err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	participants, err := s.convRepo.GetParticipants(req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("get participants: %w", err)
	}
	s.recordMessageSyncEvents(participants, msg, SyncActionCreated, req.ClientEventID)

	return &MessageDelivery{Message: msg, Participants: participants}, nil
}

func (s *MessageService) UpdateMessage(actorID, conversationID, messageID uuid.UUID, req UpdateMessageRequest) (*model.Message, error) {
	msg, err := s.convRepo.GetMessageByID(messageID)
	if err != nil {
		return nil, fmt.Errorf("message not found")
	}
	if msg.ConversationID != conversationID {
		return nil, fmt.Errorf("message is not in the conversation")
	}
	if msg.SenderID != actorID {
		return nil, fmt.Errorf("access denied: only sender can edit message")
	}
	if msg.Status == "deleted" {
		return nil, fmt.Errorf("message is deleted")
	}
	if req.Metadata == nil {
		req.Metadata = datatypes.JSONMap{}
	}
	now := time.Now()
	msg.Content = req.Content
	msg.Metadata = req.Metadata
	msg.EditedAt = &now
	if err := s.convRepo.UpdateMessage(msg); err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}
	participants, err := s.convRepo.GetParticipants(conversationID)
	if err != nil {
		return nil, fmt.Errorf("get participants: %w", err)
	}
	s.recordMessageSyncEvents(participants, msg, SyncActionUpdated, req.ClientEventID)
	return msg, nil
}

func (s *MessageService) DeleteMessage(actorID, conversationID, messageID uuid.UUID, clientEventID string) (*model.Message, error) {
	msg, err := s.convRepo.GetMessageByID(messageID)
	if err != nil {
		return nil, fmt.Errorf("message not found")
	}
	if msg.ConversationID != conversationID {
		return nil, fmt.Errorf("message is not in the conversation")
	}
	if msg.SenderID != actorID {
		return nil, fmt.Errorf("access denied: only sender can delete message")
	}
	if msg.Status == "deleted" {
		return msg, nil
	}
	deleted, err := s.convRepo.SoftDeleteMessage(messageID, actorID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("delete message: %w", err)
	}
	participants, err := s.convRepo.GetParticipants(conversationID)
	if err != nil {
		return nil, fmt.Errorf("get participants: %w", err)
	}
	s.recordMessageSyncEvents(participants, deleted, SyncActionDeleted, clientEventID)
	return deleted, nil
}

func (s *MessageService) recordMessageSyncEvents(participants []model.ConversationParticipant, msg *model.Message, operation, clientEventID string) {
	if s.syncSvc == nil {
		return
	}
	payload := messageSyncPayload(msg)
	for _, p := range participants {
		_, _ = s.syncSvc.RecordEnvelope(SyncEnvelope{
			UserID:        p.UserID,
			ObjectType:    SyncEventMessage,
			ObjectID:      msg.ID.String(),
			Operation:     operation,
			ClientEventID: clientEventID,
			Payload:       payload,
		})
	}
}

func messageSyncPayload(msg *model.Message) datatypes.JSONMap {
	metadata := msg.Metadata
	if metadata == nil {
		metadata = datatypes.JSONMap{}
	}
	visibility := msg.Visibility
	if visibility == nil {
		visibility = datatypes.JSONMap{"scope": "conversation"}
	}
	payload := datatypes.JSONMap{
		"object_id":       msg.ID.String(),
		"conversation_id": msg.ConversationID.String(),
		"message_id":      msg.ID.String(),
		"sender_id":       msg.SenderID.String(),
		"type":            msg.Type,
		"content":         msg.Content,
		"metadata":        metadata,
		"thread_id":       msg.ThreadID,
		"visibility":      visibility,
		"status":          msg.Status,
		"created_at":      msg.CreatedAt,
		"edited_at":       msg.EditedAt,
		"deleted_at":      msg.DeletedAt,
	}
	if msg.ReplyTo != nil {
		payload["reply_to"] = msg.ReplyTo.String()
	}
	if msg.DeletedBy != nil {
		payload["deleted_by"] = msg.DeletedBy.String()
	}
	return payload
}

// SaveOfflineMessages creates offline message records for offline devices.
// onlineDeviceIDs should contain the device IDs that are currently connected.
func (s *MessageService) SaveOfflineMessages(msg *model.Message, participants []model.ConversationParticipant, onlineDeviceIDs map[string]bool) error {
	for _, p := range participants {
		if p.UserID == msg.SenderID {
			continue // Don't send back to sender
		}

		// Get user's devices
		devices, err := s.userRepo.GetUserDevices(p.UserID)
		if err != nil {
			log.Printf("WARN: get devices for user %s: %v", p.UserID, err)
			continue
		}

		for _, d := range devices {
			// Skip online devices (they'll receive via WS)
			if onlineDeviceIDs[d.DeviceID] {
				continue
			}

			om := &model.OfflineMessage{
				UserID:    p.UserID,
				DeviceID:  d.DeviceID,
				MessageID: msg.ID,
				Delivered: false,
			}
			if err := s.convRepo.SaveOfflineMessage(om); err != nil {
				log.Printf("WARN: save offline message: %v", err)
			}
		}
	}
	return nil
}

// FetchOfflineMessages returns undelivered messages for a device.
func (s *MessageService) FetchOfflineMessages(userID uuid.UUID, deviceID string) ([]model.OfflineMessage, error) {
	return s.convRepo.GetUndeliveredMessages(userID, deviceID, 100)
}

// AckOfflineMessages marks offline messages as delivered.
func (s *MessageService) AckOfflineMessages(ids []uuid.UUID) error {
	return s.convRepo.MarkOfflineDelivered(ids)
}

// CleanupExpiredOffline removes offline messages older than the retention period.
func (s *MessageService) CleanupExpiredOffline(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	return s.convRepo.CleanupExpiredOffline(cutoff)
}
