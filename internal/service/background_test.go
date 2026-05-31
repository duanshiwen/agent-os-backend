package service

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBackgroundTasksRunOnceCleansMaintenanceJobs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	syncRepo := repository.NewSyncRepo(db)
	sensitiveRepo := repository.NewSensitiveOperationRepo(db)
	kbRepo := repository.NewKBHubRepo(db)
	knowledgeRepo := repository.NewKnowledgeEntriesRepo(db)
	objectRepo := repository.NewObjectRecordsRepo(db)

	user := &model.User{PubKeyEd25519: "background-user", DisplayName: "Background User", Status: "active"}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -40)
	if err := db.Create(&model.OfflineMessage{Base: model.Base{CreatedAt: old}, UserID: user.ID, DeviceID: "device-1", MessageID: uuid.New()}).Error; err != nil {
		t.Fatalf("create offline message: %v", err)
	}
	if err := db.Create(&model.SyncEvent{UserID: user.ID, ObjectType: "profile", ObjectID: "user", EventType: "updated", Timestamp: old}).Error; err != nil {
		t.Fatalf("create sync event: %v", err)
	}
	if err := db.Create(&model.SensitiveOperationConfirmation{UserID: user.ID, Operation: SensitiveOperationDeviceRevoke, TokenHash: "expired-token", ExpiresAt: now.Add(-time.Minute)}).Error; err != nil {
		t.Fatalf("create sensitive confirmation: %v", err)
	}
	collection := &model.KBCollection{OwnerID: user.ID, Name: "Background KB", Status: "published", ReviewStatus: repository.KBReviewStatusApproved, IsFree: true, PricingModel: "free"}
	if err := db.Create(collection).Error; err != nil {
		t.Fatalf("create collection: %v", err)
	}
	snapshot := &model.KBSnapshot{CollectionID: collection.ID, Version: 1, Status: repository.KBSnapshotStatusActive, PublishedAt: now, Checksum: "checksum", ManifestObjectURI: "memory://manifest"}
	if err := db.Create(snapshot).Error; err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if err := db.Create(&model.KBSubscription{UserID: uuid.New(), CollectionID: collection.ID, SnapshotID: snapshot.ID, TrackMode: "latest", Status: "active", StartedAt: now.AddDate(0, -1, 0), ExpiresAt: ptrTime(now.Add(-time.Minute)), EntitlementType: KBEntitlementPaid, RenewalStatus: "active"}).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	msgSvc := NewMessageService(convRepo, userRepo)
	syncSvc := NewSyncService(syncRepo, nil)
	sensitiveSvc := NewSensitiveOperationService(userRepo, sensitiveRepo, nil)
	kbSvc := NewKBHubService(kbRepo, knowledgeRepo, NewObjectService(objectRepo, nil, config.ObjectStorageConfig{}))
	bt := NewBackgroundTasksWithConfig(BackgroundTasksConfig{}, msgSvc, syncSvc, sensitiveSvc, kbSvc, nil)
	results := bt.RunOnce(context.Background())
	for job, err := range results {
		if err != nil {
			t.Fatalf("job %s failed: %v", job, err)
		}
	}
	var offlineCount int64
	if err := db.Model(&model.OfflineMessage{}).Count(&offlineCount).Error; err != nil || offlineCount != 0 {
		t.Fatalf("expected offline cleanup, count=%d err=%v", offlineCount, err)
	}
	var syncCount int64
	if err := db.Model(&model.SyncEvent{}).Count(&syncCount).Error; err != nil || syncCount != 0 {
		t.Fatalf("expected sync cleanup, count=%d err=%v", syncCount, err)
	}
	var sensitiveCount int64
	if err := db.Model(&model.SensitiveOperationConfirmation{}).Count(&sensitiveCount).Error; err != nil || sensitiveCount != 0 {
		t.Fatalf("expected sensitive cleanup, count=%d err=%v", sensitiveCount, err)
	}
	var sub model.KBSubscription
	if err := db.First(&sub).Error; err != nil {
		t.Fatalf("load subscription: %v", err)
	}
	if sub.Status != "expired" || sub.RenewalStatus != "expired" {
		t.Fatalf("expected expired subscription, got %+v", sub)
	}
}

func TestBackgroundTasksRunOnceSkipsMissingServices(t *testing.T) {
	bt := NewBackgroundTasksWithConfig(BackgroundTasksConfig{}, nil, nil, nil, nil, nil)
	results := bt.RunOnce(context.Background())
	if len(results) != 0 {
		t.Fatalf("expected no jobs without services, got %+v", results)
	}
}

func ptrTime(v time.Time) *time.Time { return &v }
