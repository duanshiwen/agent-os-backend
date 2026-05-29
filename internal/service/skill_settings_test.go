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

func TestSkillSettingsEnableEmitsSkillEnabledSyncEvent(t *testing.T) {
	svc, syncRepo, user := newSkillSettingsTestService(t)

	setting, event, err := svc.EnableSkill(user.ID, "device-a", "skill-alpha", "client-enable-1")
	if err != nil {
		t.Fatalf("enable skill: %v", err)
	}
	if !setting.Enabled || setting.SkillID != "skill-alpha" || setting.UpdatedByDeviceID != "device-a" {
		t.Fatalf("unexpected setting: %+v", setting)
	}
	assertSkillSyncEvent(t, event, user.ID, "device-a", "skill-alpha", SyncOperationEnabled, "client-enable-1")

	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "skill.enabled" {
		t.Fatalf("expected one skill.enabled event, got %+v", events)
	}
}

func TestSkillSettingsDisableEmitsSkillDisabledSyncEvent(t *testing.T) {
	svc, _, user := newSkillSettingsTestService(t)

	setting, event, err := svc.DisableSkill(user.ID, "device-a", "skill-alpha", "client-disable-1")
	if err != nil {
		t.Fatalf("disable skill: %v", err)
	}
	if setting.Enabled {
		t.Fatalf("expected disabled setting, got %+v", setting)
	}
	assertSkillSyncEvent(t, event, user.ID, "device-a", "skill-alpha", SyncOperationDisabled, "client-disable-1")
}

func TestSkillSettingsUpdateConfigEmitsSkillUpdatedSyncEvent(t *testing.T) {
	svc, _, user := newSkillSettingsTestService(t)

	config := datatypes.JSONMap{"model": "fast", "temperature": 0.2}
	setting, event, err := svc.UpdateSkill(user.ID, "device-a", "skill-alpha", config, "client-update-1")
	if err != nil {
		t.Fatalf("update skill: %v", err)
	}
	if setting.Config["model"] != "fast" {
		t.Fatalf("unexpected config: %+v", setting.Config)
	}
	assertSkillSyncEvent(t, event, user.ID, "device-a", "skill-alpha", SyncOperationUpdated, "client-update-1")
	if event.Payload["config"].(datatypes.JSONMap)["model"] != "fast" {
		t.Fatalf("expected config in payload, got %+v", event.Payload)
	}
}

func TestSkillSettingsClientEventIDIsIdempotent(t *testing.T) {
	svc, syncRepo, user := newSkillSettingsTestService(t)

	_, first, err := svc.EnableSkill(user.ID, "device-a", "skill-alpha", "client-idempotent-1")
	if err != nil {
		t.Fatalf("first enable: %v", err)
	}
	_, second, err := svc.EnableSkill(user.ID, "device-a", "skill-alpha", "client-idempotent-1")
	if err != nil {
		t.Fatalf("second enable: %v", err)
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

func TestSkillSettingsClientEventIDConflictIsRejected(t *testing.T) {
	svc, _, user := newSkillSettingsTestService(t)

	_, _, err := svc.EnableSkill(user.ID, "device-a", "skill-alpha", "client-conflict-1")
	if err != nil {
		t.Fatalf("enable skill: %v", err)
	}
	_, _, err = svc.DisableSkill(user.ID, "device-a", "skill-alpha", "client-conflict-1")
	if !errors.Is(err, ErrSyncIdempotencyConflict) {
		t.Fatalf("expected ErrSyncIdempotencyConflict, got %v", err)
	}
}

func assertSkillSyncEvent(t *testing.T, event *model.SyncEvent, userID uuid.UUID, sourceDeviceID, skillID, operation, clientEventID string) {
	t.Helper()
	if event == nil {
		t.Fatal("expected sync event")
	}
	if event.UserID != userID || event.SourceDeviceID != sourceDeviceID || event.ObjectType != SyncObjectSkill || event.ObjectID != skillID || event.Operation != operation || event.ClientEventID != clientEventID {
		t.Fatalf("unexpected sync event: %+v", event)
	}
	if event.EventType != "skill."+operation || event.Payload["skill_id"] != skillID || event.Payload["object_id"] != skillID || event.Payload["updated_by_device_id"] != sourceDeviceID {
		t.Fatalf("unexpected skill sync payload/event: %+v payload=%+v", event, event.Payload)
	}
}

func newSkillSettingsTestService(t *testing.T) (*SkillSettingsService, *repository.SyncRepo, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserSkillSetting{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &model.User{PubKeyEd25519: "skill-settings-pubkey", DisplayName: "Skill User", Status: "active"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	syncRepo := repository.NewSyncRepo(db)
	syncSvc := NewSyncService(syncRepo, &captureHub{})
	svc := NewSkillSettingsService(repository.NewSkillSettingsRepo(db), syncSvc)
	return svc, syncRepo, user
}
