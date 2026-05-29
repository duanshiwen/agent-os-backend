package service

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// HubNotifier is an interface for sending notifications to connected clients.
type HubNotifier interface {
	SendToUserExceptDevice(userID uuid.UUID, excludeDeviceID string, message []byte)
}

type SyncService struct {
	syncRepo *repository.SyncRepo
	hub      HubNotifier
}

func NewSyncService(syncRepo *repository.SyncRepo, hub HubNotifier) *SyncService {
	return &SyncService{syncRepo: syncRepo, hub: hub}
}

const (
	SyncEventMessage   = "message"
	SyncEventKnowledge = "knowledge"
	SyncEventSkill     = "skill"
	SyncEventAgent     = "agent"
	SyncEventServer    = "server"
	SyncEventPlugin    = "plugin"
	SyncEventProfile   = "profile"

	SyncActionCreated  = "created"
	SyncActionUpdated  = "updated"
	SyncActionDeleted  = "deleted"
	SyncActionAdded    = "added"
	SyncActionRemoved  = "removed"
	SyncActionEnabled  = "enabled"
	SyncActionDisabled = "disabled"
)

var supportedSyncActions = map[string]map[string]bool{
	SyncEventMessage: {
		SyncActionCreated: true,
		SyncActionUpdated: true,
		SyncActionDeleted: true,
	},
	SyncEventKnowledge: {
		SyncActionCreated: true,
		SyncActionUpdated: true,
		SyncActionDeleted: true,
	},
	SyncEventSkill: {
		SyncActionEnabled:  true,
		SyncActionDisabled: true,
		SyncActionUpdated:  true,
	},
	SyncEventAgent: {
		SyncActionUpdated: true,
	},
	SyncEventServer: {
		SyncActionAdded:   true,
		SyncActionUpdated: true,
		SyncActionRemoved: true,
	},
	SyncEventPlugin: {
		SyncActionAdded:   true,
		SyncActionUpdated: true,
		SyncActionRemoved: true,
	},
	SyncEventProfile: {
		SyncActionUpdated: true,
	},
}

type SyncEventMsg struct {
	EventType      string `json:"event_type"`
	SchemaVersion  int    `json:"schema_version"`
	ObjectType     string `json:"object_type"`
	ObjectID       string `json:"object_id"`
	Operation      string `json:"operation"`
	SourceDeviceID string `json:"source_device_id"`
	Sequence       uint64 `json:"sequence"`
	Timestamp      int64  `json:"timestamp"`
	Payload        any    `json:"payload,omitempty"`
}

type SyncEnvelope struct {
	UserID         uuid.UUID
	SourceDeviceID string
	ObjectType     string
	ObjectID       string
	Operation      string
	Payload        datatypes.JSONMap
}

func (s *SyncService) RecordEvent(userID uuid.UUID, deviceID, eventType string, action string, payload datatypes.JSONMap) error {
	objectID := ""
	if payload != nil {
		if value, ok := payload["object_id"].(string); ok {
			objectID = value
		}
	}
	return s.RecordEnvelope(SyncEnvelope{
		UserID:         userID,
		SourceDeviceID: deviceID,
		ObjectType:     eventType,
		ObjectID:       objectID,
		Operation:      action,
		Payload:        payload,
	})
}

func (s *SyncService) RecordEnvelope(envelope SyncEnvelope) error {
	if err := validateSyncEvent(envelope.ObjectType, envelope.Operation); err != nil {
		return err
	}

	seq, err := s.syncRepo.GetNextSequence(envelope.UserID)
	if err != nil {
		return fmt.Errorf("get next sequence: %w", err)
	}

	event := &model.SyncEvent{
		UserID:         envelope.UserID,
		DeviceID:       envelope.SourceDeviceID,
		EventType:      envelope.ObjectType + "." + envelope.Operation,
		SchemaVersion:  1,
		ObjectType:     envelope.ObjectType,
		ObjectID:       envelope.ObjectID,
		Operation:      envelope.Operation,
		SourceDeviceID: envelope.SourceDeviceID,
		Payload:        envelope.Payload,
		Timestamp:      time.Now(),
		Sequence:       seq,
	}

	if err := s.syncRepo.CreateEvent(event); err != nil {
		return fmt.Errorf("create sync event: %w", err)
	}

	s.notifyDevices(envelope.UserID, envelope.SourceDeviceID, event)
	return nil
}

func (s *SyncService) GetEvents(userID uuid.UUID, deviceID string, limit int) ([]model.SyncEvent, error) {
	cursor, err := s.syncRepo.GetCursor(userID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("get cursor: %w", err)
	}
	return s.GetEventsAfter(userID, cursor.LastSyncedSequence, limit)
}

// GetEventsAfter returns sync events after an explicit sequence without reading
// or mutating the device cursor. This supports deterministic client catch-up and
// easier sync debugging while keeping AckEvents as the durable cursor mechanism.
func (s *SyncService) GetEventsAfter(userID uuid.UUID, afterSequence uint64, limit int) ([]model.SyncEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.syncRepo.GetEventsSince(userID, "", afterSequence, limit)
}

func (s *SyncService) AckEvents(userID uuid.UUID, deviceID string, lastSequence uint64) error {
	return s.syncRepo.UpdateCursor(userID, deviceID, lastSequence)
}

func (s *SyncService) notifyDevices(userID uuid.UUID, sourceDeviceID string, event *model.SyncEvent) {
	msg := SyncEventMsg{
		EventType:      event.EventType,
		SchemaVersion:  event.SchemaVersion,
		ObjectType:     event.ObjectType,
		ObjectID:       event.ObjectID,
		Operation:      event.Operation,
		SourceDeviceID: event.SourceDeviceID,
		Sequence:       event.Sequence,
		Timestamp:      event.Timestamp.UnixMilli(),
		Payload:        event.Payload,
	}

	env := map[string]any{"type": "sync.event", "payload": msg}
	data, _ := json.Marshal(env)
	s.hub.SendToUserExceptDevice(userID, sourceDeviceID, data)
}

func validateSyncEvent(eventType, action string) error {
	allowedActions, ok := supportedSyncActions[eventType]
	if !ok {
		return fmt.Errorf("unsupported sync event type: %s", eventType)
	}
	if !allowedActions[action] {
		return fmt.Errorf("unsupported sync action %s for event type %s", action, eventType)
	}
	return nil
}

func (s *SyncService) CleanupOldEvents(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	count, err := s.syncRepo.CleanupOldEvents(cutoff)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		log.Printf("sync: cleaned up %d old events", count)
	}
	return count, nil
}
