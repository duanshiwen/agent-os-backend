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

func TestSkillHubRatingsUpdateAggregates(t *testing.T) {
	svc, _, publisher, user := newSkillHubTestService(t)
	other := uuid.New()
	skill := publishTestSkill(t, svc, publisher.ID, "market.rating", "Market Rating")

	agg, err := svc.RateSkill(user.ID, "market.rating", RateSkillInput{Rating: 5})
	if err != nil {
		t.Fatalf("rate skill: %v", err)
	}
	if agg.RatingCount != 1 || agg.RatingAverage != 5 || agg.UserRating == nil || *agg.UserRating != 5 {
		t.Fatalf("unexpected first aggregate: %+v", agg)
	}
	if _, err := svc.RateSkill(other, "market.rating", RateSkillInput{Rating: 3}); err != nil {
		t.Fatalf("other rate: %v", err)
	}
	agg, err = svc.RateSkill(user.ID, "market.rating", RateSkillInput{Rating: 4})
	if err != nil {
		t.Fatalf("update rating: %v", err)
	}
	if agg.SkillID != skill.ID || agg.RatingCount != 2 || agg.RatingAverage != 3.5 || agg.UserRating == nil || *agg.UserRating != 4 {
		t.Fatalf("unexpected updated aggregate: %+v", agg)
	}
	agg, err = svc.DeleteRating(user.ID, "market.rating")
	if err != nil {
		t.Fatalf("delete rating: %v", err)
	}
	if agg.RatingCount != 1 || agg.RatingAverage != 3 || agg.UserRating != nil {
		t.Fatalf("unexpected aggregate after delete: %+v", agg)
	}
	if _, err := svc.RateSkill(user.ID, "market.rating", RateSkillInput{Rating: 6}); err == nil {
		t.Fatalf("expected invalid rating error")
	}
}

func TestSkillHubInstallDownloadCountOnlyOnNewOrReactivatedInstall(t *testing.T) {
	svc, _, publisher, user := newSkillHubTestService(t)
	publishTestSkill(t, svc, publisher.ID, "market.download", "Market Download")
	if _, err := svc.InstallSkill(user.ID, "device-a", "market.download", InstallSkillInput{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	item, err := svc.GetCatalogSkill("market.download")
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if item.DownloadCount != 1 {
		t.Fatalf("expected first install download count 1, got %d", item.DownloadCount)
	}
	if _, err := svc.InstallSkill(user.ID, "device-a", "market.download", InstallSkillInput{}); err != nil {
		t.Fatalf("reinstall active: %v", err)
	}
	item, _ = svc.GetCatalogSkill("market.download")
	if item.DownloadCount != 1 {
		t.Fatalf("active reinstall should not increment, got %d", item.DownloadCount)
	}
	items, _ := svc.ListInstallations(user.ID)
	if _, err := svc.SetInstallationStatus(user.ID, "device-a", items[0].ID, repository.SkillInstallationStatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := svc.SetInstallationStatus(user.ID, "device-a", items[0].ID, repository.SkillInstallationStatusActive); err != nil {
		t.Fatalf("enable: %v", err)
	}
	item, _ = svc.GetCatalogSkill("market.download")
	if item.DownloadCount != 1 {
		t.Fatalf("enable should not increment, got %d", item.DownloadCount)
	}
	if _, err := svc.SetInstallationStatus(user.ID, "device-a", items[0].ID, repository.SkillInstallationStatusUninstalled); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := svc.InstallSkill(user.ID, "device-a", "market.download", InstallSkillInput{}); err != nil {
		t.Fatalf("reactivate install: %v", err)
	}
	item, _ = svc.GetCatalogSkill("market.download")
	if item.DownloadCount != 2 {
		t.Fatalf("reactivated install should increment, got %d", item.DownloadCount)
	}
}

func TestSkillHubCatalogRecommendationSort(t *testing.T) {
	svc, _, publisher, user := newSkillHubTestService(t)
	publishTestSkill(t, svc, publisher.ID, "market.alpha", "Alpha")
	publishTestSkill(t, svc, publisher.ID, "market.beta", "Beta")
	publishTestSkill(t, svc, publisher.ID, "market.gamma", "Gamma")

	for i := 0; i < 3; i++ {
		if _, err := svc.InstallSkill(uuid.New(), "device-a", "market.beta", InstallSkillInput{}); err != nil {
			t.Fatalf("install beta: %v", err)
		}
	}
	if _, err := svc.RateSkill(user.ID, "market.gamma", RateSkillInput{Rating: 5}); err != nil {
		t.Fatalf("rate gamma: %v", err)
	}
	if _, err := svc.RateSkill(uuid.New(), "market.alpha", RateSkillInput{Rating: 2}); err != nil {
		t.Fatalf("rate alpha: %v", err)
	}

	page, err := svc.SearchCatalogSorted("market", "", repository.SkillCatalogSortRecommended, 10, 0)
	if err != nil {
		t.Fatalf("recommended catalog: %v", err)
	}
	if len(page.Items) != 3 || page.Sort != repository.SkillCatalogSortRecommended || page.Items[0].SkillKey != "market.gamma" {
		t.Fatalf("expected gamma first by recommendation, got %+v", page)
	}
	page, err = svc.SearchCatalogSorted("market", "", repository.SkillCatalogSortDownloads, 10, 0)
	if err != nil {
		t.Fatalf("downloads catalog: %v", err)
	}
	if page.Items[0].SkillKey != "market.beta" || page.Items[0].DownloadCount != 3 {
		t.Fatalf("expected beta first by downloads, got %+v", page.Items)
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
	if err := db.AutoMigrate(&model.User{}, &model.Skill{}, &model.SkillVersion{}, &model.SkillInstallation{}, &model.SkillPublisherRestriction{}, &model.SkillRating{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}, &model.AuditEvent{}); err != nil {
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

func publishTestSkill(t *testing.T, svc *SkillHubService, publisherID uuid.UUID, skillKey, name string) *model.Skill {
	t.Helper()
	skill, err := svc.CreateSkill(publisherID, CreateSkillInput{SkillKey: skillKey, Name: name})
	if err != nil {
		t.Fatalf("create %s: %v", skillKey, err)
	}
	if _, validation, err := svc.SubmitVersion(publisherID, skill.ID, SubmitSkillVersionInput{Manifest: validSkillManifest(skillKey, "1.0.0")}); err != nil || !validation.Valid {
		t.Fatalf("publish %s: validation=%+v err=%v", skillKey, validation, err)
	}
	return skill
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
