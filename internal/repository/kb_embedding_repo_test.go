package repository

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestKBEmbeddingRepoQueueLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.KBSearchDocument{}, &model.KBEmbeddingJob{}, &model.KBSearchEmbedding{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	doc := model.KBSearchDocument{CollectionID: uuid.New(), SnapshotID: uuid.New(), SnapshotEntryID: uuid.New(), EntryID: "entry", Title: "Entry", ContentHash: "hash", Status: "active", IndexedAt: time.Now()}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	repo := NewKBEmbeddingRepo(db)
	input := EnqueueKBEmbeddingJobsInput{Provider: "deterministic", Model: "test", Dimensions: 4, Documents: []model.KBSearchDocument{doc}}
	if err := repo.EnqueueJobs(input); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := repo.EnqueueJobs(input); err != nil {
		t.Fatalf("enqueue again: %v", err)
	}
	count, err := repo.CountJobs(doc.SnapshotID)
	if err != nil || count != 1 {
		t.Fatalf("expected one job, count=%d err=%v", count, err)
	}
	claimed, err := repo.ClaimJobs(ClaimKBEmbeddingJobsInput{WorkerID: "test-worker", Limit: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Status != KBEmbeddingJobStatusProcessing || claimed[0].Attempts != 1 {
		t.Fatalf("unexpected claimed jobs: %+v", claimed)
	}
	if err := repo.FailJob(claimed[0], errors.New("temporary"), time.Second); err != nil {
		t.Fatalf("fail: %v", err)
	}
	job, err := repo.GetJob(claimed[0].ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job.Status != KBEmbeddingJobStatusRetrying || job.LastError == "" {
		t.Fatalf("expected retrying job, got %+v", job)
	}
	job.Attempts = job.MaxAttempts
	if err := repo.CompleteJob(*job, []float32{1, 0, 0, 0}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	coverage, err := repo.Coverage(doc.SnapshotID, "deterministic", "test")
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if coverage.TotalDocuments != 1 || coverage.ReadyEmbeddings != 1 || coverage.Coverage != 1 {
		t.Fatalf("unexpected coverage: %+v", coverage)
	}
}
