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

func TestSyncRepoGetNextSequenceUsesDurableUserCursor(t *testing.T) {
	_, repo := newSyncTestService(t)
	userID := uuid.New()

	seq1, err := repo.GetNextSequence(userID)
	if err != nil {
		t.Fatalf("get first sequence: %v", err)
	}
	seq2, err := repo.GetNextSequence(userID)
	if err != nil {
		t.Fatalf("get second sequence: %v", err)
	}
	if seq1 != 1 || seq2 != 2 {
		t.Fatalf("expected durable cursor sequences 1,2 got %d,%d", seq1, seq2)
	}

	otherUserSeq, err := repo.GetNextSequence(uuid.New())
	if err != nil {
		t.Fatalf("get other user sequence: %v", err)
	}
	if otherUserSeq != 1 {
		t.Fatalf("expected independent per-user sequence to start at 1, got %d", otherUserSeq)
	}
}

func TestSyncServiceRecordEventAssignsMonotonicSequencesAndNotifiesOtherDevices(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()

	if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, SyncActionCreated, datatypes.JSONMap{"n": 1}); err != nil {
		t.Fatalf("record event 1: %v", err)
	}
	if err := svc.RecordEvent(userID, "device-a", SyncEventProfile, SyncActionUpdated, datatypes.JSONMap{"n": 2}); err != nil {
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
	if events[0].SchemaVersion != 1 || events[0].ObjectType != SyncEventMessage || events[0].Operation != SyncActionCreated || events[0].SourceDeviceID != "device-a" {
		t.Fatalf("unexpected message envelope fields: %+v", events[0])
	}
	if events[1].SchemaVersion != 1 || events[1].ObjectType != SyncEventProfile || events[1].Operation != SyncActionUpdated || events[1].SourceDeviceID != "device-a" {
		t.Fatalf("unexpected profile envelope fields: %+v", events[1])
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

func TestSyncServiceRecordEnvelopePersistsStableContractFields(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()
	objectID := uuid.New().String()

	if err := svc.RecordEnvelope(SyncEnvelope{
		UserID:         userID,
		SourceDeviceID: "device-a",
		ObjectType:     SyncEventProfile,
		ObjectID:       objectID,
		Operation:      SyncActionUpdated,
		Payload:        datatypes.JSONMap{"display_name": "Alice"},
	}); err != nil {
		t.Fatalf("record envelope: %v", err)
	}

	events, err := svc.GetEventsAfter(userID, 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %+v", events)
	}
	event := events[0]
	if event.EventType != "profile.updated" || event.SchemaVersion != 1 || event.ObjectType != SyncEventProfile || event.ObjectID != objectID || event.Operation != SyncActionUpdated || event.SourceDeviceID != "device-a" {
		t.Fatalf("unexpected persisted envelope: %+v", event)
	}
}

func TestSyncServiceRecordEnvelopeIsIdempotentByClientEventID(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()
	objectID := uuid.New().String()

	envelope := SyncEnvelope{
		UserID:         userID,
		SourceDeviceID: "device-a",
		ObjectType:     SyncEventProfile,
		ObjectID:       objectID,
		Operation:      SyncActionUpdated,
		ClientEventID:  "client-event-1",
		Payload:        datatypes.JSONMap{"display_name": "Alice"},
	}
	if err := svc.RecordEnvelope(envelope); err != nil {
		t.Fatalf("record first envelope: %v", err)
	}
	if err := svc.RecordEnvelope(envelope); err != nil {
		t.Fatalf("replay same envelope: %v", err)
	}

	events, err := svc.GetEventsAfter(userID, 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].ClientEventID != "client-event-1" {
		t.Fatalf("expected exactly one idempotent event, got %+v", events)
	}

	envelope.ObjectID = uuid.New().String()
	if err := svc.RecordEnvelope(envelope); err == nil {
		t.Fatal("expected conflicting client_event_id replay to be rejected")
	}
}

func TestSyncServiceAckAdvancesCursorAndFiltersEvents(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()
	for i := 0; i < 3; i++ {
		if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, SyncActionCreated, datatypes.JSONMap{"n": i}); err != nil {
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
		if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, SyncActionCreated, datatypes.JSONMap{"n": i}); err != nil {
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

func TestSyncServiceAckDoesNotMoveCursorBackwards(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()
	for i := 0; i < 5; i++ {
		if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, SyncActionCreated, datatypes.JSONMap{"n": i}); err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
	}

	if err := svc.AckEvents(userID, "device-b", 4); err != nil {
		t.Fatalf("ack sequence 4: %v", err)
	}
	if err := svc.AckEvents(userID, "device-b", 2); err != nil {
		t.Fatalf("ack older sequence 2: %v", err)
	}

	events, err := svc.GetEvents(userID, "device-b", 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != 5 {
		t.Fatalf("expected cursor to remain at 4 and return only sequence 5, got %+v", events)
	}
}

func TestSyncServiceEventsAreIsolatedByUser(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userA := uuid.New()
	userB := uuid.New()

	if err := svc.RecordEvent(userA, "device-a", SyncEventMessage, SyncActionCreated, datatypes.JSONMap{"user": "a"}); err != nil {
		t.Fatalf("record user A event: %v", err)
	}
	if err := svc.RecordEvent(userB, "device-b", SyncEventMessage, SyncActionCreated, datatypes.JSONMap{"user": "b"}); err != nil {
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
		if err := svc.RecordEvent(userID, "device-a", SyncEventMessage, SyncActionCreated, datatypes.JSONMap{"n": i}); err != nil {
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

func TestSyncServiceRejectsUnsupportedEventTypesAndActions(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()

	if err := svc.RecordEvent(userID, "device-a", "unknown", SyncActionCreated, datatypes.JSONMap{}); err == nil {
		t.Fatal("expected unsupported event type to be rejected")
	}
	if err := svc.RecordEvent(userID, "device-a", SyncEventProfile, SyncActionDeleted, datatypes.JSONMap{}); err == nil {
		t.Fatal("expected unsupported profile action to be rejected")
	}
}

func newSyncTestService(t *testing.T) (*SyncService, *repository.SyncRepo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewSyncRepo(db)
	return NewSyncService(repo, &captureHub{}), repo
}
