package service

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	BackgroundJobOfflineMessageCleanup        = "offline_message_cleanup"
	BackgroundJobSyncEventCleanup             = "sync_event_cleanup"
	BackgroundJobSensitiveConfirmationCleanup = "sensitive_confirmation_cleanup"
	BackgroundJobKBSubscriptionExpiry         = "kb_subscription_expiry"
	BackgroundJobKBEmbeddingWorker            = "kb_embedding_worker"
)

// BackgroundJobConfig controls one periodic background job.
type BackgroundJobConfig struct {
	Name       string
	Interval   time.Duration
	RunOnStart bool
	Enabled    bool
}

// BackgroundTasksConfig controls the unified background task runner.
type BackgroundTasksConfig struct {
	OfflineMessageCleanupInterval        time.Duration
	SyncEventCleanupInterval             time.Duration
	SensitiveConfirmationCleanupInterval time.Duration
	KBSubscriptionExpiryInterval         time.Duration
	EmbeddingWorkerEnabled               bool
	EmbeddingWorkerPollInterval          time.Duration
	RunOnStart                           bool
}

// BackgroundTasks manages periodic cleanup and maintenance tasks.
type BackgroundTasks struct {
	msgService                *MessageService
	syncService               *SyncService
	sensitiveOperationService *SensitiveOperationService
	kbHubService              *KBHubService
	embeddingWorker           *KBEmbeddingWorker
	config                    BackgroundTasksConfig
}

func DefaultBackgroundTasksConfig() BackgroundTasksConfig {
	return BackgroundTasksConfig{
		OfflineMessageCleanupInterval:        1 * time.Hour,
		SyncEventCleanupInterval:             6 * time.Hour,
		SensitiveConfirmationCleanupInterval: 15 * time.Minute,
		KBSubscriptionExpiryInterval:         15 * time.Minute,
		EmbeddingWorkerEnabled:               false,
		EmbeddingWorkerPollInterval:          2 * time.Second,
		RunOnStart:                           false,
	}
}

func NewBackgroundTasksWithConfig(cfg BackgroundTasksConfig, msgService *MessageService, syncService *SyncService, sensitiveOperationService *SensitiveOperationService, kbHubService *KBHubService, embeddingWorker *KBEmbeddingWorker) *BackgroundTasks {
	cfg = normalizeBackgroundConfig(cfg)
	return &BackgroundTasks{
		msgService:                msgService,
		syncService:               syncService,
		sensitiveOperationService: sensitiveOperationService,
		kbHubService:              kbHubService,
		embeddingWorker:           embeddingWorker,
		config:                    cfg,
	}
}

func normalizeBackgroundConfig(cfg BackgroundTasksConfig) BackgroundTasksConfig {
	defaults := DefaultBackgroundTasksConfig()
	if cfg.OfflineMessageCleanupInterval <= 0 {
		cfg.OfflineMessageCleanupInterval = defaults.OfflineMessageCleanupInterval
	}
	if cfg.SyncEventCleanupInterval <= 0 {
		cfg.SyncEventCleanupInterval = defaults.SyncEventCleanupInterval
	}
	if cfg.SensitiveConfirmationCleanupInterval <= 0 {
		cfg.SensitiveConfirmationCleanupInterval = defaults.SensitiveConfirmationCleanupInterval
	}
	if cfg.KBSubscriptionExpiryInterval <= 0 {
		cfg.KBSubscriptionExpiryInterval = defaults.KBSubscriptionExpiryInterval
	}
	if cfg.EmbeddingWorkerPollInterval <= 0 {
		cfg.EmbeddingWorkerPollInterval = defaults.EmbeddingWorkerPollInterval
	}
	return cfg
}

// Start begins all configured background tasks. Cancel the context to stop them.
func (bt *BackgroundTasks) Start(ctx context.Context) {
	for _, job := range bt.jobs() {
		bt.startPeriodicJob(ctx, job)
	}
	if bt.config.EmbeddingWorkerEnabled && bt.embeddingWorker != nil {
		go func() {
			log.Println("background: kb embedding worker started")
			if err := bt.embeddingWorker.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("background: kb embedding worker stopped with error: %v", err)
			} else {
				log.Println("background: kb embedding worker stopped")
			}
		}()
	}
}

// RunOnce executes every available maintenance job once. It is intended for tests and operational one-shots.
func (bt *BackgroundTasks) RunOnce(ctx context.Context) map[string]error {
	results := map[string]error{}
	for _, job := range bt.jobs() {
		if job.Enabled && job.run != nil {
			results[job.Name] = job.run(ctx)
		}
	}
	return results
}

type backgroundJob struct {
	BackgroundJobConfig
	run func(context.Context) error
}

func (bt *BackgroundTasks) jobs() []backgroundJob {
	return []backgroundJob{
		{
			BackgroundJobConfig: BackgroundJobConfig{Name: BackgroundJobOfflineMessageCleanup, Interval: bt.config.OfflineMessageCleanupInterval, RunOnStart: bt.config.RunOnStart, Enabled: bt.msgService != nil},
			run: func(ctx context.Context) error {
				count, err := bt.msgService.CleanupExpiredOffline(7)
				logBackgroundCount(BackgroundJobOfflineMessageCleanup, count, err)
				return err
			},
		},
		{
			BackgroundJobConfig: BackgroundJobConfig{Name: BackgroundJobSyncEventCleanup, Interval: bt.config.SyncEventCleanupInterval, RunOnStart: bt.config.RunOnStart, Enabled: bt.syncService != nil},
			run: func(ctx context.Context) error {
				count, err := bt.syncService.CleanupOldEvents(30)
				logBackgroundCount(BackgroundJobSyncEventCleanup, count, err)
				return err
			},
		},
		{
			BackgroundJobConfig: BackgroundJobConfig{Name: BackgroundJobSensitiveConfirmationCleanup, Interval: bt.config.SensitiveConfirmationCleanupInterval, RunOnStart: bt.config.RunOnStart, Enabled: bt.sensitiveOperationService != nil},
			run: func(ctx context.Context) error {
				count, err := bt.sensitiveOperationService.CleanupExpiredConfirmations(time.Now().UTC())
				logBackgroundCount(BackgroundJobSensitiveConfirmationCleanup, count, err)
				return err
			},
		},
		{
			BackgroundJobConfig: BackgroundJobConfig{Name: BackgroundJobKBSubscriptionExpiry, Interval: bt.config.KBSubscriptionExpiryInterval, RunOnStart: bt.config.RunOnStart, Enabled: bt.kbHubService != nil},
			run: func(ctx context.Context) error {
				count, err := bt.kbHubService.ExpireSubscriptions(time.Now().UTC())
				logBackgroundCount(BackgroundJobKBSubscriptionExpiry, count, err)
				return err
			},
		},
	}
}

func (bt *BackgroundTasks) startPeriodicJob(ctx context.Context, job backgroundJob) {
	if !job.Enabled || job.run == nil {
		return
	}
	go func() {
		log.Printf("background: %s started (every %s)", job.Name, job.Interval)
		if job.RunOnStart {
			_ = job.run(ctx)
		}
		ticker := time.NewTicker(job.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("background: %s stopped", job.Name)
				return
			case <-ticker.C:
				_ = job.run(ctx)
			}
		}
	}()
}

func logBackgroundCount(jobName string, count int64, err error) {
	if err != nil {
		log.Printf("background: %s error: %v", jobName, err)
		return
	}
	if count > 0 {
		log.Printf("background: %s affected %d rows", jobName, count)
	}
}

// StartAndWait is a test helper that starts jobs and waits until context cancellation is observed.
func (bt *BackgroundTasks) StartAndWait(ctx context.Context) {
	var wg sync.WaitGroup
	for _, job := range bt.jobs() {
		if !job.Enabled || job.run == nil {
			continue
		}
		wg.Add(1)
		go func(job backgroundJob) {
			defer wg.Done()
			bt.startPeriodicJob(ctx, job)
		}(job)
	}
	<-ctx.Done()
	wg.Wait()
}
