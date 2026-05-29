package service

import (
	"errors"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentSettingsUpdateEmitsAgentUpdatedSyncEvent(t *testing.T) {
	svc, syncRepo, user := newAgentSettingsTestService(t)

	config := datatypes.JSONMap{"model": "gpt-4.1", "temperature": 0.3}
	setting, event, err := svc.UpdateAgent(user.ID, "device-a", "default", stringPtr("Assistant"), config, "agent-update-1")
	if err != nil {
		t.Fatalf("update agent: %v", err)
	}
	if setting.AgentID != "default" || setting.DisplayName != "Assistant" || setting.UpdatedByDeviceID != "device-a" || setting.Config["model"] != "gpt-4.1" {
		t.Fatalf("unexpected setting: %+v", setting)
	}
	assertAgentSyncEvent(t, event, user.ID, "device-a", "default", "agent-update-1")

	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "agent.updated" {
		t.Fatalf("expected one agent.updated event, got %+v", events)
	}
}

func TestAgentSettingsClientEventIDIsIdempotent(t *testing.T) {
	svc, syncRepo, user := newAgentSettingsTestService(t)

	_, first, err := svc.UpdateAgent(user.ID, "device-a", "default", stringPtr("Assistant"), datatypes.JSONMap{"model": "fast"}, "agent-idempotent-1")
	if err != nil {
		t.Fatalf("first update: %v", err)
	}
	_, second, err := svc.UpdateAgent(user.ID, "device-a", "default", stringPtr("Assistant"), datatypes.JSONMap{"model": "fast"}, "agent-idempotent-1")
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if first.ID != second.ID || first.Sequence != second.Sequence {
		t.Fatalf("expected idempotent replay to return same event, first=%+v second=%+v", first, second)
	}
	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one persisted event after idempotent replay, got %+v", events)
	}
}

func TestAgentSettingsClientEventIDConflictIsRejected(t *testing.T) {
	svc, _, user := newAgentSettingsTestService(t)

	_, _, err := svc.UpdateAgent(user.ID, "device-a", "default", stringPtr("Assistant"), datatypes.JSONMap{"model": "fast"}, "agent-conflict-1")
	if err != nil {
		t.Fatalf("first update: %v", err)
	}
	_, _, err = svc.UpdateAgent(user.ID, "device-a", "other-agent", stringPtr("Other"), datatypes.JSONMap{"model": "fast"}, "agent-conflict-1")
	if !errors.Is(err, ErrSyncIdempotencyConflict) {
		t.Fatalf("expected ErrSyncIdempotencyConflict, got %v", err)
	}
}

func assertAgentSyncEvent(t *testing.T, event *model.SyncEvent, userID uuid.UUID, sourceDeviceID, agentID, clientEventID string) {
	t.Helper()
	if event == nil {
		t.Fatal("expected sync event")
	}
	if event.UserID != userID || event.SourceDeviceID != sourceDeviceID || event.ObjectType != SyncObjectAgent || event.ObjectID != agentID || event.Operation != SyncOperationUpdated || event.ClientEventID != clientEventID {
		t.Fatalf("unexpected sync event: %+v", event)
	}
	if event.EventType != "agent.updated" || event.Payload["agent_id"] != agentID || event.Payload["object_id"] != agentID || event.Payload["updated_by_device_id"] != sourceDeviceID {
		t.Fatalf("unexpected agent sync payload/event: %+v payload=%+v", event, event.Payload)
	}
}

func newAgentSettingsTestService(t *testing.T) (*AgentSettingsService, *repository.SyncRepo, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserAgentSetting{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &model.User{PubKeyEd25519: "agent-settings-pubkey", DisplayName: "Agent User", Status: "active"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	syncRepo := repository.NewSyncRepo(db)
	syncSvc := NewSyncService(syncRepo, &captureHub{})
	svc := NewAgentSettingsService(repository.NewAgentSettingsRepo(db), syncSvc)
	return svc, syncRepo, user
}

func stringPtr(value string) *string {
	return &value
}
