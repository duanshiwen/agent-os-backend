package repository

import (
	"errors"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	KBEmbeddingJobStatusPending    = "pending"
	KBEmbeddingJobStatusProcessing = "processing"
	KBEmbeddingJobStatusReady      = "ready"
	KBEmbeddingJobStatusRetrying   = "retrying"
	KBEmbeddingJobStatusFailed     = "failed"
	KBEmbeddingJobStatusCancelled  = "cancelled"
	KBEmbeddingJobStatusSkipped    = "skipped"
)

type KBEmbeddingRepo struct {
	db *gorm.DB
}

type EnqueueKBEmbeddingJobsInput struct {
	Provider    string
	Model       string
	Dimensions  int
	Priority    int
	MaxAttempts int
	Documents   []model.KBSearchDocument
}

type ClaimKBEmbeddingJobsInput struct {
	WorkerID string
	Limit    int
	Now      time.Time
}

type KBEmbeddingCoverage struct {
	TotalDocuments  int64   `json:"total_documents"`
	ReadyEmbeddings int64   `json:"ready_embeddings"`
	PendingJobs     int64   `json:"pending_jobs"`
	ProcessingJobs  int64   `json:"processing_jobs"`
	RetryingJobs    int64   `json:"retrying_jobs"`
	FailedJobs      int64   `json:"failed_jobs"`
	Coverage        float64 `json:"coverage"`
}

func NewKBEmbeddingRepo(db *gorm.DB) *KBEmbeddingRepo {
	return &KBEmbeddingRepo{db: db}
}

func (r *KBEmbeddingRepo) DB() *gorm.DB { return r.db }

func (r *KBEmbeddingRepo) EnqueueJobs(input EnqueueKBEmbeddingJobsInput) error {
	return r.EnqueueJobsTx(r.db, input)
}

func (r *KBEmbeddingRepo) EnqueueJobsTx(tx *gorm.DB, input EnqueueKBEmbeddingJobsInput) error {
	if tx == nil {
		tx = r.db
	}
	if len(input.Documents) == 0 || input.Provider == "" || input.Model == "" {
		return nil
	}
	now := time.Now().UTC()
	dimensions := input.Dimensions
	if dimensions <= 0 {
		dimensions = 1024
	}
	priority := input.Priority
	if priority <= 0 {
		priority = 100
	}
	maxAttempts := input.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	jobs := make([]model.KBEmbeddingJob, 0, len(input.Documents))
	for _, doc := range input.Documents {
		if doc.ID == uuid.Nil || doc.ContentHash == "" {
			continue
		}
		jobs = append(jobs, model.KBEmbeddingJob{
			SearchDocumentID: doc.ID,
			CollectionID:     doc.CollectionID,
			SnapshotID:       doc.SnapshotID,
			SnapshotEntryID:  doc.SnapshotEntryID,
			Provider:         input.Provider,
			Model:            input.Model,
			Dimensions:       dimensions,
			ContentHash:      doc.ContentHash,
			Status:           KBEmbeddingJobStatusPending,
			Priority:         priority,
			Attempts:         0,
			MaxAttempts:      maxAttempts,
			AvailableAt:      now,
		})
	}
	if len(jobs) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&jobs).Error
}

func (r *KBEmbeddingRepo) CountJobs(snapshotID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&model.KBEmbeddingJob{}).Where("snapshot_id = ?", snapshotID).Count(&count).Error
	return count, err
}

func (r *KBEmbeddingRepo) GetJob(id uuid.UUID) (*model.KBEmbeddingJob, error) {
	var job model.KBEmbeddingJob
	if err := r.db.First(&job, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *KBEmbeddingRepo) ClaimJobs(input ClaimKBEmbeddingJobsInput) ([]model.KBEmbeddingJob, error) {
	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = 8
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	workerID := input.WorkerID
	if workerID == "" {
		workerID = "embedding-worker"
	}
	var jobs []model.KBEmbeddingJob
	err := r.db.Transaction(func(tx *gorm.DB) error {
		dialect := tx.Dialector.Name()
		query := tx.Model(&model.KBEmbeddingJob{}).
			Where("status IN ? AND available_at <= ?", []string{KBEmbeddingJobStatusPending, KBEmbeddingJobStatusRetrying}, now).
			Order("priority ASC, created_at ASC").
			Limit(limit)
		if dialect == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		var candidates []model.KBEmbeddingJob
		if err := query.Find(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.ID)
		}
		updates := map[string]any{
			"status":     KBEmbeddingJobStatusProcessing,
			"locked_at":  now,
			"locked_by":  workerID,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": now,
		}
		if err := tx.Model(&model.KBEmbeddingJob{}).Where("id IN ?", ids).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Order("priority ASC, created_at ASC").Find(&jobs).Error
	})
	return jobs, err
}

func (r *KBEmbeddingRepo) CompleteJob(job model.KBEmbeddingJob, vector []float32) error {
	now := time.Now().UTC()
	return r.db.Transaction(func(tx *gorm.DB) error {
		embedding := model.KBSearchEmbedding{
			SearchDocumentID: job.SearchDocumentID,
			CollectionID:     job.CollectionID,
			SnapshotID:       job.SnapshotID,
			SnapshotEntryID:  job.SnapshotEntryID,
			Provider:         job.Provider,
			Model:            job.Model,
			Dimensions:       job.Dimensions,
			ContentHash:      job.ContentHash,
			Embedding:        FormatVector(vector),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "search_document_id"}, {Name: "provider"}, {Name: "model"}, {Name: "content_hash"}},
			DoUpdates: clause.Assignments(map[string]any{
				"embedding":  embedding.Embedding,
				"updated_at": now,
			}),
		}).Create(&embedding).Error; err != nil {
			return err
		}
		return tx.Model(&model.KBEmbeddingJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":       KBEmbeddingJobStatusReady,
			"locked_at":    nil,
			"locked_by":    "",
			"last_error":   "",
			"completed_at": now,
			"updated_at":   now,
		}).Error
	})
}

func (r *KBEmbeddingRepo) FailJob(job model.KBEmbeddingJob, cause error, backoff time.Duration) error {
	now := time.Now().UTC()
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	status := KBEmbeddingJobStatusRetrying
	availableAt := now.Add(backoff)
	updates := map[string]any{
		"status":       status,
		"available_at": availableAt,
		"locked_at":    nil,
		"locked_by":    "",
		"last_error":   message,
		"updated_at":   now,
	}
	if job.Attempts >= job.MaxAttempts {
		status = KBEmbeddingJobStatusFailed
		updates["status"] = status
		updates["failed_at"] = now
	}
	return r.db.Model(&model.KBEmbeddingJob{}).Where("id = ?", job.ID).Updates(updates).Error
}

func (r *KBEmbeddingRepo) RetryFailedJobs(snapshotID uuid.UUID, provider, modelName string) (int64, error) {
	if snapshotID == uuid.Nil {
		return 0, nil
	}
	now := time.Now().UTC()
	q := r.db.Model(&model.KBEmbeddingJob{}).
		Where("snapshot_id = ? AND status = ?", snapshotID, KBEmbeddingJobStatusFailed)
	if provider != "" {
		q = q.Where("provider = ?", provider)
	}
	if modelName != "" {
		q = q.Where("model = ?", modelName)
	}
	res := q.Updates(map[string]any{
		"status":       KBEmbeddingJobStatusPending,
		"attempts":     0,
		"available_at": now,
		"locked_at":    nil,
		"locked_by":    "",
		"last_error":   "",
		"failed_at":    nil,
		"updated_at":   now,
	})
	return res.RowsAffected, res.Error
}

func (r *KBEmbeddingRepo) Coverage(snapshotID uuid.UUID, provider, modelName string) (*KBEmbeddingCoverage, error) {
	var total int64
	if err := r.db.Model(&model.KBSearchDocument{}).Where("snapshot_id = ? AND status = ?", snapshotID, "active").Count(&total).Error; err != nil {
		return nil, err
	}
	coverage := &KBEmbeddingCoverage{TotalDocuments: total}
	baseJob := r.db.Model(&model.KBEmbeddingJob{}).Where("snapshot_id = ?", snapshotID)
	if provider != "" {
		baseJob = baseJob.Where("provider = ?", provider)
	}
	if modelName != "" {
		baseJob = baseJob.Where("model = ?", modelName)
	}
	counts := map[string]*int64{
		KBEmbeddingJobStatusPending:    &coverage.PendingJobs,
		KBEmbeddingJobStatusProcessing: &coverage.ProcessingJobs,
		KBEmbeddingJobStatusRetrying:   &coverage.RetryingJobs,
		KBEmbeddingJobStatusFailed:     &coverage.FailedJobs,
	}
	for status, target := range counts {
		q := baseJob.Session(&gorm.Session{})
		if err := q.Where("status = ?", status).Count(target).Error; err != nil {
			return nil, err
		}
	}
	embQ := r.db.Model(&model.KBSearchEmbedding{}).Where("snapshot_id = ?", snapshotID)
	if provider != "" {
		embQ = embQ.Where("provider = ?", provider)
	}
	if modelName != "" {
		embQ = embQ.Where("model = ?", modelName)
	}
	if err := embQ.Count(&coverage.ReadyEmbeddings).Error; err != nil {
		return nil, err
	}
	if total > 0 {
		coverage.Coverage = float64(coverage.ReadyEmbeddings) / float64(total)
	}
	return coverage, nil
}

func (r *KBEmbeddingRepo) GetDocument(id uuid.UUID) (*model.KBSearchDocument, error) {
	var doc model.KBSearchDocument
	if err := r.db.First(&doc, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

var ErrInvalidVector = errors.New("invalid embedding vector")
