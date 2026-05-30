package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
)

type KBEmbeddingWorker struct {
	repo         *repository.KBEmbeddingRepo
	provider     EmbeddingProvider
	workerID     string
	batchSize    int
	pollInterval time.Duration
}

type KBEmbeddingWorkerConfig struct {
	WorkerID     string
	BatchSize    int
	PollInterval time.Duration
}

func NewKBEmbeddingWorker(repo *repository.KBEmbeddingRepo, provider EmbeddingProvider, cfg KBEmbeddingWorkerConfig) *KBEmbeddingWorker {
	if cfg.WorkerID == "" {
		cfg.WorkerID = fmt.Sprintf("kb-embedding-worker-%d", time.Now().UnixNano())
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 8
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	return &KBEmbeddingWorker{repo: repo, provider: provider, workerID: cfg.WorkerID, batchSize: cfg.BatchSize, pollInterval: cfg.PollInterval}
}

func (w *KBEmbeddingWorker) Run(ctx context.Context) error {
	if w.repo == nil || w.provider == nil {
		return fmt.Errorf("embedding worker requires repo and provider")
	}
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		if err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
			// Keep worker alive; job-level failures are already persisted.
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *KBEmbeddingWorker) ProcessOnce(ctx context.Context) error {
	jobs, err := w.repo.ClaimJobs(repository.ClaimKBEmbeddingJobsInput{WorkerID: w.workerID, Limit: w.batchSize})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := w.processJob(ctx, job); err != nil && ctx.Err() != nil {
			return err
		}
	}
	return nil
}

func (w *KBEmbeddingWorker) processJob(ctx context.Context, job model.KBEmbeddingJob) error {
	doc, err := w.repo.GetDocument(job.SearchDocumentID)
	if err != nil {
		_ = w.repo.FailJob(job, err, backoffForAttempt(job.Attempts))
		return err
	}
	if doc.ContentHash != job.ContentHash {
		return w.repo.FailJob(job, fmt.Errorf("content hash mismatch: job=%s document=%s", job.ContentHash, doc.ContentHash), backoffForAttempt(job.Attempts))
	}
	text := BuildKBEmbeddingText(*doc)
	vectors, err := w.provider.EmbedTexts(ctx, []string{text})
	if err != nil {
		_ = w.repo.FailJob(job, err, backoffForAttempt(job.Attempts))
		return err
	}
	if len(vectors) != 1 || len(vectors[0].Vector) != job.Dimensions {
		err := fmt.Errorf("invalid embedding response dimensions")
		_ = w.repo.FailJob(job, err, backoffForAttempt(job.Attempts))
		return err
	}
	if err := w.repo.CompleteJob(job, vectors[0].Vector); err != nil {
		_ = w.repo.FailJob(job, err, backoffForAttempt(job.Attempts))
		return err
	}
	return nil
}

func BuildKBEmbeddingText(doc model.KBSearchDocument) string {
	parts := []string{
		"Title:\n" + doc.Title,
		"Summary:\n" + doc.Summary,
	}
	if len(doc.Tags) > 0 {
		parts = append(parts, "Tags:\n"+strings.Join([]string(doc.Tags), ", "))
	}
	if source, ok := doc.Metadata["source"].(string); ok && strings.TrimSpace(source) != "" {
		parts = append(parts, "Source:\n"+source)
	}
	content := doc.ContentText
	const maxChars = 8000
	if len(content) > maxChars {
		content = content[:maxChars]
	}
	parts = append(parts, "Content:\n"+content)
	return strings.Join(parts, "\n\n")
}

func backoffForAttempt(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	backoff := time.Duration(1<<min(attempt, 8)) * 30 * time.Second
	max := 2 * time.Hour
	if backoff > max {
		return max
	}
	return backoff
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
