package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestKBHubServicePublishesImmutableSnapshot(t *testing.T) {
	svc, knowledgeRepo, fake, ownerID := newKBHubServiceTestEnv(t)
	ctx := context.Background()

	entry := &model.UserKnowledgeEntry{
		UserID:          ownerID,
		EntryID:         "notes/alpha",
		Title:           "Alpha",
		ContentMarkdown: "# Alpha\n\nOriginal content.",
		Summary:         "Alpha summary",
		Tags:            datatypes.NewJSONSlice([]string{"alpha"}),
		Metadata:        datatypes.JSONMap{"source": "test"},
		Status:          repository.KnowledgeEntryStatusActive,
		Version:         1,
		ContentHash:     strings.Repeat("a", 64),
	}
	if err := knowledgeRepo.Create(entry); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}

	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "My KB", Description: "snapshot test"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	detail, err := svc.PublishSnapshot(ctx, ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	if detail.Snapshot.Version != 1 || detail.Snapshot.EntryCount != 1 || detail.Snapshot.ManifestObjectURI == "" || detail.Snapshot.Checksum == "" {
		t.Fatalf("unexpected snapshot: %+v", detail.Snapshot)
	}
	if len(detail.Entries) != 1 || detail.Entries[0].ContentObjectURI == "" || detail.Entries[0].Title != "Alpha" {
		t.Fatalf("unexpected snapshot entries: %+v", detail.Entries)
	}
	if len(fake.puts) != 2 {
		t.Fatalf("expected one content object and one manifest object, got %d", len(fake.puts))
	}

	entry.ContentMarkdown = "# Alpha\n\nChanged after snapshot."
	entry.Title = "Alpha changed"
	entry.ContentHash = strings.Repeat("b", 64)
	if err := knowledgeRepo.UpdateActive(entry); err != nil {
		t.Fatalf("update source entry: %v", err)
	}

	loaded, err := svc.GetSnapshot(ownerID, collection.ID, detail.Snapshot.ID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if loaded.Entries[0].Title != "Alpha" {
		t.Fatalf("snapshot entry mutated with source entry, got title %q", loaded.Entries[0].Title)
	}

	detail2, err := svc.PublishSnapshot(ctx, ownerID, collection.ID, PublishKBSnapshotInput{EntryIDs: []string{"notes/alpha"}})
	if err != nil {
		t.Fatalf("publish second snapshot: %v", err)
	}
	if detail2.Snapshot.Version != 2 || detail2.Entries[0].Title != "Alpha changed" {
		t.Fatalf("expected second snapshot to capture new source state, got snapshot=%+v entries=%+v", detail2.Snapshot, detail2.Entries)
	}
}

func TestKBHubServiceRejectsEmptySnapshot(t *testing.T) {
	svc, _, _, ownerID := newKBHubServiceTestEnv(t)
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Empty KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	_, err = svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err == nil || !strings.Contains(err.Error(), ErrKBNoEntries.Error()) {
		t.Fatalf("expected no entries error, got %v", err)
	}
}

func TestKBHubServicePublicReadManifestAndInstall(t *testing.T) {
	svc, knowledgeRepo, _, ownerID := newKBHubServiceTestEnv(t)
	consumerID := uuid.New()
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "notes/public", Title: "Public", ContentMarkdown: "# Public", Summary: "public", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("c", 64)}); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Public KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if public, err := svc.ListPublicCollections(); err != nil || len(public) != 0 {
		t.Fatalf("draft collection should not be public, got public=%+v err=%v", public, err)
	}
	detail, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	public, err := svc.ListPublicCollections()
	if err != nil || len(public) != 1 || public[0].ID != collection.ID {
		t.Fatalf("expected published collection in public list, got public=%+v err=%v", public, err)
	}
	publicDetail, err := svc.GetPublicCollection(collection.ID)
	if err != nil {
		t.Fatalf("get public collection: %v", err)
	}
	if publicDetail.LatestSnapshot == nil || publicDetail.LatestSnapshot.ID != detail.Snapshot.ID {
		t.Fatalf("unexpected public detail: %+v", publicDetail)
	}
	manifestURL, err := svc.CreateSnapshotManifestDownloadURL(context.Background(), collection.ID, detail.Snapshot.ID)
	if err != nil {
		t.Fatalf("manifest download url: %v", err)
	}
	if manifestURL.DownloadURL == "" || manifestURL.Object.ObjectURI != detail.Snapshot.ManifestObjectURI {
		t.Fatalf("unexpected manifest download response: %+v", manifestURL)
	}
	sub, err := svc.InstallCollection(consumerID, collection.ID, InstallKBCollectionInput{})
	if err != nil {
		t.Fatalf("install latest: %v", err)
	}
	if sub.TrackMode != "latest" || sub.SnapshotID != detail.Snapshot.ID || sub.Status != "active" {
		t.Fatalf("unexpected subscription: %+v", sub)
	}
	pinned := 1
	sub, err = svc.InstallCollection(consumerID, collection.ID, InstallKBCollectionInput{TrackMode: "pinned", PinnedVersion: &pinned})
	if err != nil {
		t.Fatalf("install pinned: %v", err)
	}
	if sub.TrackMode != "pinned" || sub.PinnedVersion == nil || *sub.PinnedVersion != 1 {
		t.Fatalf("unexpected pinned subscription: %+v", sub)
	}
	subs, err := svc.ListSubscriptions(consumerID)
	if err != nil || len(subs) != 1 {
		t.Fatalf("expected one upserted subscription, got subs=%+v err=%v", subs, err)
	}
}

func TestKBHubServiceInstalledAccessAndCancel(t *testing.T) {
	svc, knowledgeRepo, _, ownerID := newKBHubServiceTestEnv(t)
	consumerID := uuid.New()
	strangerID := uuid.New()
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "notes/access", Title: "Access", ContentMarkdown: "# Access", Summary: "access", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("d", 64)}); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Access KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	detail, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	entryRecordID := detail.Entries[0].ID
	if _, err := svc.CreateInstalledManifestDownloadURL(context.Background(), strangerID, collection.ID, detail.Snapshot.ID); err == nil || !strings.Contains(err.Error(), ErrKBSubscriptionNeeded.Error()) {
		t.Fatalf("expected stranger manifest access denial, got %v", err)
	}
	if _, err := svc.InstallCollection(consumerID, collection.ID, InstallKBCollectionInput{}); err != nil {
		t.Fatalf("install latest: %v", err)
	}
	manifestURL, err := svc.CreateInstalledManifestDownloadURL(context.Background(), consumerID, collection.ID, detail.Snapshot.ID)
	if err != nil {
		t.Fatalf("installed manifest url: %v", err)
	}
	if manifestURL.DownloadURL == "" {
		t.Fatal("expected manifest download URL")
	}
	contentURL, err := svc.CreateInstalledEntryContentDownloadURL(context.Background(), consumerID, collection.ID, detail.Snapshot.ID, entryRecordID)
	if err != nil {
		t.Fatalf("installed content url: %v", err)
	}
	if contentURL.DownloadURL == "" || contentURL.Object.ObjectURI != detail.Entries[0].ContentObjectURI {
		t.Fatalf("unexpected content url response: %+v", contentURL)
	}
	cancelled, err := svc.CancelSubscription(consumerID, collection.ID)
	if err != nil {
		t.Fatalf("cancel subscription: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled subscription, got %+v", cancelled)
	}
	if _, err := svc.CreateInstalledEntryContentDownloadURL(context.Background(), consumerID, collection.ID, detail.Snapshot.ID, entryRecordID); err == nil || !strings.Contains(err.Error(), ErrKBSubscriptionNeeded.Error()) {
		t.Fatalf("expected access denial after cancel, got %v", err)
	}
}

func newKBHubServiceTestEnv(t *testing.T) (*KBHubService, *repository.KnowledgeEntriesRepo, *recordingObjectStorageBackend, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.UserKnowledgeEntry{}, &model.ObjectRecord{}, &model.KBCollection{}, &model.KBSnapshot{}, &model.KBSnapshotEntry{}, &model.KBSubscription{}); err != nil {
		t.Fatalf("migrate models: %v", err)
	}
	fake := &recordingObjectStorageBackend{fakeObjectStorageBackend: fakeObjectStorageBackend{head: ObjectHead{ContentHash: strings.Repeat("a", 64), ContentSize: 1}}}
	objectSvc := NewObjectService(repository.NewObjectRecordsRepo(db), fake, config.ObjectStorageConfig{Bucket: "agentos-test", UploadTTLSecs: 900, DownloadTTLSecs: 900})
	return NewKBHubService(repository.NewKBHubRepo(db), repository.NewKnowledgeEntriesRepo(db), objectSvc), repository.NewKnowledgeEntriesRepo(db), fake, uuid.New()
}

type recordedPut struct {
	Bucket      string
	Key         string
	Size        int64
	ContentType string
	Content     string
}

type recordingObjectStorageBackend struct {
	fakeObjectStorageBackend
	puts []recordedPut
}

func (f *recordingObjectStorageBackend) PutObject(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string, metadata map[string]string) error {
	_ = ctx
	_ = metadata
	buf, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.puts = append(f.puts, recordedPut{Bucket: bucket, Key: key, Size: size, ContentType: contentType, Content: string(buf)})
	return nil
}
