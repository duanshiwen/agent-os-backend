package service

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestObjectServiceUploadCompleteDownloadAndDeleteLifecycle(t *testing.T) {
	svc, fake, ownerID := newObjectServiceTestEnv(t)
	ctx := context.Background()

	intent, err := svc.CreateUploadIntent(ctx, ownerID, CreateUploadIntentInput{
		Scope:       "kb/snapshots",
		Filename:    "alpha.md",
		ContentType: "text/markdown",
		ContentSize: 12,
		SHA256:      strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("create upload intent: %v", err)
	}
	if intent.Object.ID == uuid.Nil || intent.Object.OwnerID != ownerID || intent.Object.Status != repository.ObjectStatusPending || intent.Object.ObjectKey == "" || intent.Object.ObjectURI == "" || !strings.Contains(intent.UploadURL, intent.Object.ObjectKey) {
		t.Fatalf("unexpected upload intent: %+v", intent)
	}
	if !fake.bucketEnsured {
		t.Fatal("expected bucket to be ensured")
	}
	if strings.Contains(intent.Object.ObjectKey, "..") {
		t.Fatalf("object key should be sanitized, got %s", intent.Object.ObjectKey)
	}

	fake.head = ObjectHead{ContentHash: strings.Repeat("a", 64), ContentSize: 12, ContentType: "text/markdown"}
	completed, err := svc.CompleteUpload(ctx, ownerID, intent.Object.ID, CompleteUploadInput{})
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	if completed.Status != repository.ObjectStatusActive || completed.CompletedAt == nil {
		t.Fatalf("expected active completed object, got %+v", completed)
	}

	download, err := svc.CreateDownloadURL(ctx, ownerID, intent.Object.ID, CreateDownloadURLInput{Disposition: "inline"})
	if err != nil {
		t.Fatalf("create download url: %v", err)
	}
	if download.DownloadURL == "" || !strings.Contains(download.DownloadURL, intent.Object.ObjectKey) || !download.ExpiresAt.After(time.Now()) {
		t.Fatalf("unexpected download response: %+v", download)
	}

	deleted, err := svc.DeleteObject(ownerID, intent.Object.ID)
	if err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if deleted.Status != repository.ObjectStatusDeleted || deleted.DeletedAt == nil {
		t.Fatalf("expected soft-deleted object, got %+v", deleted)
	}
}

func TestObjectServiceRejectsMismatchedUploadCompletion(t *testing.T) {
	svc, fake, ownerID := newObjectServiceTestEnv(t)
	intent, err := svc.CreateUploadIntent(context.Background(), ownerID, CreateUploadIntentInput{Scope: "attachments", Filename: "a.txt", ContentType: "text/plain", ContentSize: 5, SHA256: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatalf("create upload intent: %v", err)
	}

	fake.head = ObjectHead{ContentHash: strings.Repeat("c", 64), ContentSize: 5, ContentType: "text/plain"}
	if _, err := svc.CompleteUpload(context.Background(), ownerID, intent.Object.ID, CompleteUploadInput{}); err == nil || !strings.Contains(err.Error(), ErrObjectHashMismatch.Error()) {
		t.Fatalf("expected hash mismatch, got %v", err)
	}

	fake.head = ObjectHead{ContentHash: strings.Repeat("b", 64), ContentSize: 6, ContentType: "text/plain"}
	if _, err := svc.CompleteUpload(context.Background(), ownerID, intent.Object.ID, CompleteUploadInput{}); err == nil || !strings.Contains(err.Error(), ErrObjectSizeMismatch.Error()) {
		t.Fatalf("expected size mismatch, got %v", err)
	}
}

func TestObjectServiceRequiresActiveObjectForDownload(t *testing.T) {
	svc, _, ownerID := newObjectServiceTestEnv(t)
	intent, err := svc.CreateUploadIntent(context.Background(), ownerID, CreateUploadIntentInput{Scope: "attachments", Filename: "a.txt", ContentType: "text/plain", ContentSize: 5, SHA256: strings.Repeat("d", 64)})
	if err != nil {
		t.Fatalf("create upload intent: %v", err)
	}
	if _, err := svc.CreateDownloadURL(context.Background(), ownerID, intent.Object.ID, CreateDownloadURLInput{}); err == nil || !strings.Contains(err.Error(), ErrObjectNotActive.Error()) {
		t.Fatalf("expected inactive download conflict, got %v", err)
	}
}

func TestObjectServiceEnforcesOwner(t *testing.T) {
	svc, _, ownerID := newObjectServiceTestEnv(t)
	intent, err := svc.CreateUploadIntent(context.Background(), ownerID, CreateUploadIntentInput{Scope: "attachments", Filename: "a.txt", ContentType: "text/plain", ContentSize: 5, SHA256: strings.Repeat("e", 64)})
	if err != nil {
		t.Fatalf("create upload intent: %v", err)
	}
	otherUser := uuid.New()
	if _, err := svc.GetObject(otherUser, intent.Object.ID); err == nil || !strings.Contains(err.Error(), ErrObjectNotFound.Error()) {
		t.Fatalf("expected not found for another owner, got %v", err)
	}
}

func TestObjectServiceValidatesUploadIntent(t *testing.T) {
	svc, _, ownerID := newObjectServiceTestEnv(t)
	_, err := svc.CreateUploadIntent(context.Background(), ownerID, CreateUploadIntentInput{Scope: "attachments", Filename: "a.txt", ContentSize: 5, SHA256: "short"})
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("expected sha validation error, got %v", err)
	}
	_, err = svc.CreateUploadIntent(context.Background(), ownerID, CreateUploadIntentInput{Scope: "../bad", Filename: "a.txt", ContentSize: 5, SHA256: strings.Repeat("f", 64)})
	if err == nil || !strings.Contains(err.Error(), "invalid scope") {
		t.Fatalf("expected scope validation error, got %v", err)
	}
}

func newObjectServiceTestEnv(t *testing.T) (*ObjectService, *fakeObjectStorageBackend, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ObjectRecord{}); err != nil {
		t.Fatalf("migrate object records: %v", err)
	}
	fake := &fakeObjectStorageBackend{}
	cfg := config.ObjectStorageConfig{Bucket: "agentos-test", UploadTTLSecs: 900, DownloadTTLSecs: 900}
	return NewObjectService(repository.NewObjectRecordsRepo(db), fake, cfg), fake, uuid.New()
}

type fakeObjectStorageBackend struct {
	bucketEnsured bool
	head          ObjectHead
}

func (f *fakeObjectStorageBackend) EnsureBucket(ctx context.Context, bucket string) error {
	_ = ctx
	_ = bucket
	f.bucketEnsured = true
	return nil
}

func (f *fakeObjectStorageBackend) PutObject(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string, metadata map[string]string) error {
	_ = ctx
	_ = bucket
	_ = key
	_ = reader
	_ = size
	_ = contentType
	_ = metadata
	return nil
}

func (f *fakeObjectStorageBackend) PresignedPutURL(ctx context.Context, bucket, key string, ttl time.Duration, contentType string) (string, error) {
	_ = ctx
	_ = bucket
	_ = ttl
	_ = contentType
	return "http://minio.local/" + key + "?put=1", nil
}

func (f *fakeObjectStorageBackend) PresignedGetURL(ctx context.Context, bucket, key string, ttl time.Duration, disposition string) (string, error) {
	_ = ctx
	_ = bucket
	_ = ttl
	return "http://minio.local/" + key + "?disposition=" + disposition, nil
}

func (f *fakeObjectStorageBackend) HeadObject(ctx context.Context, bucket, key string) (ObjectHead, error) {
	_ = ctx
	_ = bucket
	_ = key
	return f.head, nil
}

func (f *fakeObjectStorageBackend) ReadObject(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, error) {
	_ = ctx
	_ = bucket
	_ = key
	_ = maxBytes
	return nil, nil
}
