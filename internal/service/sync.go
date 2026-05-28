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
)

type SyncEventMsg struct {
	EventType string `json:"event_type"`
	Sequence  uint64 `json:"sequence"`
	Timestamp int64  `json:"timestamp"`
	Payload   any    `json:"payload,omitempty"`
}

func (s *SyncService) RecordEvent(userID uuid.UUID, deviceID, eventType string, action string, payload datatypes.JSONMap) error {
	seq, err := s.syncRepo.GetNextSequence(userID)
	if err != nil {
		return fmt.Errorf("get next sequence: %w", err)
	}

	event := &model.SyncEvent{
		UserID:    userID,
		DeviceID:  deviceID,
		EventType: eventType + "." + action,
		Payload:   payload,
		Timestamp: time.Now(),
		Sequence:  seq,
	}

	if err := s.syncRepo.CreateEvent(event); err != nil {
		return fmt.Errorf("create sync event: %w", err)
	}

	s.notifyDevices(userID, deviceID, event)
	return nil
}

func (s *SyncService) GetEvents(userID uuid.UUID, deviceID string, limit int) ([]model.SyncEvent, error) {
	cursor, err := s.syncRepo.GetCursor(userID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("get cursor: %w", err)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.syncRepo.GetEventsSince(userID, deviceID, cursor.LastSyncedSequence, limit)
}

func (s *SyncService) AckEvents(userID uuid.UUID, deviceID string, lastSequence uint64) error {
	return s.syncRepo.UpdateCursor(userID, deviceID, lastSequence)
}

func (s *SyncService) notifyDevices(userID uuid.UUID, sourceDeviceID string, event *model.SyncEvent) {
	msg := SyncEventMsg{
		EventType: event.EventType,
		Sequence:  event.Sequence,
		Timestamp: event.Timestamp.UnixMilli(),
		Payload:   event.Payload,
	}

	env := map[string]any{"type": "sync.event", "payload": msg}
	data, _ := json.Marshal(env)
	s.hub.SendToUserExceptDevice(userID, sourceDeviceID, data)
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
