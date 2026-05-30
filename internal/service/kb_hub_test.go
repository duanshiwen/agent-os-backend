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

func newKBHubServiceTestEnv(t *testing.T) (*KBHubService, *repository.KnowledgeEntriesRepo, *recordingObjectStorageBackend, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.UserKnowledgeEntry{}, &model.ObjectRecord{}, &model.KBCollection{}, &model.KBSnapshot{}, &model.KBSnapshotEntry{}); err != nil {
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
