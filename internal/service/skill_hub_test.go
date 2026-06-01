package service

import (
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSkillHubPublishesValidVersionAndCatalogInstallSync(t *testing.T) {
	svc, syncRepo, publisher, user := newSkillHubTestService(t)

	skill, err := svc.CreateSkill(publisher.ID, CreateSkillInput{SkillKey: "research.brief-writer", Name: "Research Brief Writer", Category: "research"})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	version, validation, err := svc.SubmitVersion(publisher.ID, skill.ID, SubmitSkillVersionInput{Manifest: validSkillManifest("research.brief-writer", "1.0.0")})
	if err != nil {
		t.Fatalf("submit version: %v", err)
	}
	if !validation.Valid || version.Status != repository.SkillVersionStatusPublished || version.PublishedAt == nil {
		t.Fatalf("expected published valid version, got version=%+v validation=%+v", version, validation)
	}

	page, err := svc.SearchCatalog("brief", "research", 20, 0)
	if err != nil {
		t.Fatalf("search catalog: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].SkillKey != "research.brief-writer" {
		t.Fatalf("expected catalog item, got %+v", page)
	}

	inst, err := svc.InstallSkill(user.ID, "device-a", "research.brief-writer", InstallSkillInput{TrackMode: "latest", Config: map[string]any{"tone": "concise"}})
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}
	if inst.Status != repository.SkillInstallationStatusActive || inst.TrackMode != repository.SkillTrackModeLatest {
		t.Fatalf("unexpected install: %+v", inst)
	}
	events, err := syncRepo.GetEventsSince(user.ID, "device-b", 0, 10)
	if err != nil {
		t.Fatalf("pull sync: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "skill.installed" || events[0].ObjectID != inst.ID.String() || events[0].Payload["skill_key"] != "research.brief-writer" {
		t.Fatalf("unexpected sync events: %+v", events)
	}
}

func TestSkillHubInvalidVersionDoesNotPublish(t *testing.T) {
	svc, _, publisher, _ := newSkillHubTestService(t)
	skill, err := svc.CreateSkill(publisher.ID, CreateSkillInput{SkillKey: "bad.skill", Name: "Bad Skill"})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	version, validation, err := svc.SubmitVersion(publisher.ID, skill.ID, SubmitSkillVersionInput{Manifest: map[string]any{"id": "bad.skill", "name": "Bad Skill", "version": "0.1.0"}})
	if err != nil {
		t.Fatalf("submit invalid version should persist validation result: %v", err)
	}
	if validation.Valid || version.Status != repository.SkillVersionStatusDraft || version.ValidationStatus != repository.SkillValidationStatusInvalid {
		t.Fatalf("expected invalid draft version, got version=%+v validation=%+v", version, validation)
	}
	page, err := svc.SearchCatalog("bad", "", 20, 0)
	if err != nil {
		t.Fatalf("search catalog: %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("invalid skill should not publish to catalog: %+v", page)
	}
}

func TestSkillHubTakedownHidesCatalogButKeepsInstallation(t *testing.T) {
	svc, _, publisher, user := newSkillHubTestService(t)
	skill, err := svc.CreateSkill(publisher.ID, CreateSkillInput{SkillKey: "ops.runbook", Name: "Ops Runbook"})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if _, _, err := svc.SubmitVersion(publisher.ID, skill.ID, SubmitSkillVersionInput{Manifest: validSkillManifest("ops.runbook", "1.0.0")}); err != nil {
		t.Fatalf("submit version: %v", err)
	}
	if _, err := svc.InstallSkill(user.ID, "device-a", "ops.runbook", InstallSkillInput{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := svc.TakedownSkill(publisher.ID, skill.ID, "policy"); err != nil {
		t.Fatalf("takedown: %v", err)
	}
	if _, err := svc.GetCatalogSkill("ops.runbook"); err != ErrSkillNotFound {
		t.Fatalf("expected takedown to hide catalog, got %v", err)
	}
	items, err := svc.ListInstallations(user.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("installation history should remain, items=%+v err=%v", items, err)
	}
}

func TestSkillHubPublisherRestrictionBlocksAndLiftAllows(t *testing.T) {
	svc, _, publisher, _ := newSkillHubTestService(t)
	admin := uuid.New()
	if _, err := svc.RestrictPublisher(admin, publisher.ID, "spam"); err != nil {
		t.Fatalf("restrict: %v", err)
	}
	if _, err := svc.CreateSkill(publisher.ID, CreateSkillInput{SkillKey: "blocked.skill", Name: "Blocked"}); err != ErrSkillPublisherBlocked {
		t.Fatalf("expected blocked publisher, got %v", err)
	}
	if _, err := svc.LiftPublisherRestriction(admin, publisher.ID); err != nil {
		t.Fatalf("lift: %v", err)
	}
	if _, err := svc.CreateSkill(publisher.ID, CreateSkillInput{SkillKey: "allowed.skill", Name: "Allowed"}); err != nil {
		t.Fatalf("create after lift: %v", err)
	}
}

func newSkillHubTestService(t *testing.T) (*SkillHubService, *repository.SyncRepo, *model.User, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Skill{}, &model.SkillVersion{}, &model.SkillInstallation{}, &model.SkillPublisherRestriction{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	publisher := &model.User{PubKeyEd25519: "publisher-" + uuid.NewString(), Status: "active"}
	user := &model.User{PubKeyEd25519: "user-" + uuid.NewString(), Status: "active"}
	if err := db.Create(publisher).Error; err != nil {
		t.Fatalf("create publisher: %v", err)
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	syncRepo := repository.NewSyncRepo(db)
	svc := NewSkillHubService(repository.NewSkillHubRepo(db))
	svc.SetSyncService(NewSyncService(syncRepo, nil))
	svc.SetAuditService(NewAuditService(repository.NewAuditRepo(db)))
	return svc, syncRepo, publisher, user
}

func validSkillManifest(skillKey, version string) map[string]any {
	return map[string]any{
		"id":          skillKey,
		"name":        "Research Brief Writer",
		"version":     version,
		"summary":     "Turn scattered notes into a research brief.",
		"description": "Turn scattered notes and web findings into a structured research brief.",
		"category":    "research",
		"tags":        []any{"research", "writing"},
		"instructions": []any{
			map[string]any{"kind": "system", "content": "Create concise structured research briefs."},
		},
	}
}
