package service

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type captureHub struct {
	userID          uuid.UUID
	excludeDeviceID string
	messages        [][]byte
}

func (h *captureHub) SendToUserExceptDevice(userID uuid.UUID, excludeDeviceID string, message []byte) {
	h.userID = userID
	h.excludeDeviceID = excludeDeviceID
	h.messages = append(h.messages, append([]byte(nil), message...))
}

func TestSyncServiceRecordEventAssignsMonotonicSequencesAndNotifiesOtherDevices(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()

	if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, "created", datatypes.JSONMap{"n": 1}); err != nil {
		t.Fatalf("record event 1: %v", err)
	}
	if err := svc.RecordEvent(userID, "device-a", SyncEventProfile, "updated", datatypes.JSONMap{"n": 2}); err != nil {
		t.Fatalf("record event 2: %v", err)
	}

	events, err := svc.GetEvents(userID, "device-b", 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("expected sequences 1,2 got %d,%d", events[0].Sequence, events[1].Sequence)
	}
	if events[0].EventType != "message.created" || events[1].EventType != "profile.updated" {
		t.Fatalf("unexpected event types: %q %q", events[0].EventType, events[1].EventType)
	}

	hub := svc.hub.(*captureHub)
	if hub.userID != userID || hub.excludeDeviceID != "device-a" || len(hub.messages) != 2 {
		t.Fatalf("unexpected notifications: user=%s exclude=%q count=%d", hub.userID, hub.excludeDeviceID, len(hub.messages))
	}
	var envelope map[string]any
	if err := json.Unmarshal(hub.messages[1], &envelope); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if envelope["type"] != "sync.event" {
		t.Fatalf("unexpected notification type: %v", envelope["type"])
	}
}

func TestSyncServiceAckAdvancesCursorAndFiltersEvents(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()
	for i := 0; i < 3; i++ {
		if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, "created", datatypes.JSONMap{"n": i}); err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
	}

	if err := svc.AckEvents(userID, "device-b", 2); err != nil {
		t.Fatalf("ack events: %v", err)
	}

	events, err := svc.GetEvents(userID, "device-b", 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != 3 {
		t.Fatalf("expected only sequence 3 after ack, got %+v", events)
	}
}

func TestSyncServiceGetEventsAfterUsesExplicitSequenceWithoutCursor(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()
	for i := 0; i < 3; i++ {
		if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, "created", datatypes.JSONMap{"n": i}); err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
	}
	if err := svc.AckEvents(userID, "device-b", 2); err != nil {
		t.Fatalf("ack events: %v", err)
	}

	events, err := svc.GetEventsAfter(userID, 1, 100)
	if err != nil {
		t.Fatalf("get events after explicit sequence: %v", err)
	}
	if len(events) != 2 || events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("expected sequences 2 and 3 from explicit after_sequence, got %+v", events)
	}

	cursorEvents, err := svc.GetEvents(userID, "device-b", 100)
	if err != nil {
		t.Fatalf("get cursor events: %v", err)
	}
	if len(cursorEvents) != 1 || cursorEvents[0].Sequence != 3 {
		t.Fatalf("expected cursor to remain at acked sequence 2, got %+v", cursorEvents)
	}
}

func TestSyncServiceEventsAreIsolatedByUser(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userA := uuid.New()
	userB := uuid.New()

	if err := svc.RecordEvent(userA, "device-a", SyncEventMessage, "created", datatypes.JSONMap{"user": "a"}); err != nil {
		t.Fatalf("record user A event: %v", err)
	}
	if err := svc.RecordEvent(userB, "device-b", SyncEventMessage, "created", datatypes.JSONMap{"user": "b"}); err != nil {
		t.Fatalf("record user B event: %v", err)
	}

	events, err := svc.GetEvents(userA, "device-a2", 100)
	if err != nil {
		t.Fatalf("get user A events: %v", err)
	}
	if len(events) != 1 || events[0].UserID != userA {
		t.Fatalf("expected only user A event, got %+v", events)
	}
}

func TestSyncServiceLimitDefaultsAndCaps(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()
	for i := 0; i < 3; i++ {
		if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, "created", datatypes.JSONMap{"n": i}); err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
	}

	events, err := svc.GetEvents(userID, "device-b", 0)
	if err != nil {
		t.Fatalf("get default limit events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events with default limit, got %d", len(events))
	}
}

func newSyncTestService(t *testing.T) (*SyncService, *repository.SyncRepo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.SyncEvent{}, &model.SyncCursor{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewSyncRepo(db)
	return NewSyncService(repo, &captureHub{}), repo
}
