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

type SyncEventMsg struct {
	EventType      string `json:"event_type"`
	SchemaVersion  int    `json:"schema_version"`
	ObjectType     string `json:"object_type"`
	ObjectID       string `json:"object_id"`
	Operation      string `json:"operation"`
	SourceDeviceID string `json:"source_device_id"`
	ClientEventID  string `json:"client_event_id"`
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
	ClientEventID  string
	Payload        datatypes.JSONMap
}

func (s *SyncService) RecordEvent(userID uuid.UUID, deviceID, eventType string, action string, payload datatypes.JSONMap) (*model.SyncEvent, error) {
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

func (s *SyncService) RecordEnvelope(envelope SyncEnvelope) (*model.SyncEvent, error) {
	eventType, err := BuildSyncEventType(envelope.ObjectType, envelope.Operation)
	if err != nil {
		return nil, err
	}
	if envelope.ClientEventID != "" {
		existing, err := s.syncRepo.GetEventByClientEventID(envelope.UserID, envelope.ClientEventID)
		if err == nil {
			return existing, s.validateIdempotentReplay(existing, envelope)
		}
	}

	seq, err := s.syncRepo.GetNextSequence(envelope.UserID)
	if err != nil {
		return nil, fmt.Errorf("get next sequence: %w", err)
	}

	event := &model.SyncEvent{
		UserID:         envelope.UserID,
		DeviceID:       envelope.SourceDeviceID,
		EventType:      eventType,
		SchemaVersion:  SyncSchemaVersion,
		ObjectType:     envelope.ObjectType,
		ObjectID:       envelope.ObjectID,
		Operation:      envelope.Operation,
		SourceDeviceID: envelope.SourceDeviceID,
		ClientEventID:  envelope.ClientEventID,
		Payload:        envelope.Payload,
		Timestamp:      time.Now(),
		Sequence:       seq,
	}

	if err := s.syncRepo.CreateEvent(event); err != nil {
		return nil, fmt.Errorf("create sync event: %w", err)
	}

	s.notifyDevices(envelope.UserID, envelope.SourceDeviceID, event)
	return event, nil
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
		ClientEventID:  event.ClientEventID,
		Sequence:       event.Sequence,
		Timestamp:      event.Timestamp.UnixMilli(),
		Payload:        event.Payload,
	}

	env := map[string]any{"type": "sync.event", "payload": msg}
	data, _ := json.Marshal(env)
	s.hub.SendToUserExceptDevice(userID, sourceDeviceID, data)
}

func (s *SyncService) validateIdempotentReplay(existing *model.SyncEvent, envelope SyncEnvelope) error {
	if existing.ObjectType != envelope.ObjectType || existing.ObjectID != envelope.ObjectID || existing.Operation != envelope.Operation || existing.SourceDeviceID != envelope.SourceDeviceID {
		return fmt.Errorf("%w: client_event_id already used for different sync event", ErrSyncIdempotencyConflict)
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
