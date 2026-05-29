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
}

type MessageDelivery struct {
	Message      *model.Message                  `json:"message"`
	Participants []model.ConversationParticipant `json:"-"`
}

// SendMessage validates, stores, and prepares a message for delivery.
func (s *MessageService) SendMessage(senderID uuid.UUID, req *SendMessageRequest) (*MessageDelivery, error) {
	// Verify sender is a participant
	isParticipant, err := s.convRepo.IsParticipant(req.ConversationID, senderID)
	if err != nil || !isParticipant {
		return nil, fmt.Errorf("access denied: not a participant")
	}

	// Default message type
	msgType := req.Type
	if msgType == "" {
		msgType = "text"
	}

	msg := &model.Message{
		ConversationID: req.ConversationID,
		SenderID:       senderID,
		Type:           msgType,
		Content:        req.Content,
		Metadata:       req.Metadata,
	}
	if err := s.convRepo.CreateMessage(msg); err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	// Get all participants for delivery and cross-device sync
	participants, err := s.convRepo.GetParticipants(req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("get participants: %w", err)
	}

	// Record sync events for every participant so both recipient devices and the sender's
	// other devices can converge through the sync cursor. Offline message delivery remains
	// recipient-only and is handled separately by SaveOfflineMessages.
	if s.syncSvc != nil {
		metadata := msg.Metadata
		if metadata == nil {
			metadata = datatypes.JSONMap{}
		}
		for _, p := range participants {
			syncPayload := datatypes.JSONMap{
				"object_id":       msg.ID.String(),
				"conversation_id": req.ConversationID.String(),
				"message_id":      msg.ID.String(),
				"sender_id":       senderID.String(),
				"type":            msgType,
				"content":         msg.Content,
				"metadata":        metadata,
				"created_at":      msg.CreatedAt,
			}
			_, _ = s.syncSvc.RecordEvent(p.UserID, "", SyncEventMessage, SyncActionCreated, syncPayload)
		}
	}

	return &MessageDelivery{
		Message:      msg,
		Participants: participants,
	}, nil
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
