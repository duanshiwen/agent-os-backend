package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/router"
	"github.com/agent-os/backend/internal/service"
	"github.com/agent-os/backend/internal/ws"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := config.InitDatabase(&cfg.Database, cfg.App.AutoMigrate)
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	log.Println("Database initialized")

	// Initialize Redis
	rdb, err := config.InitRedis(&cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to init redis: %v", err)
	}
	defer rdb.Close()
	log.Println("Redis initialized")

	// Create context for background tasks
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize identity verifier.
	signatureVerifier, cleanupVerifier, err := service.NewSignatureVerifierFromConfig(cfg.IdentityVerifier)
	if err != nil {
		log.Fatalf("Failed to initialize identity verifier: %v", err)
	}
	defer cleanupVerifier()
	log.Printf("Identity verifier initialized (backend=%s, version=%s)", service.SignatureVerifierBackend(signatureVerifier), service.SignatureVerifierVersion(signatureVerifier))

	// Initialize WebSocket hub
	hub := ws.NewHub()
	go hub.Run()
	log.Println("WebSocket hub started")

	// Initialize repositories
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	syncRepo := repository.NewSyncRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	sensitiveOperationRepo := repository.NewSensitiveOperationRepo(db)
	knowledgeEntriesRepo := repository.NewKnowledgeEntriesRepo(db)
	objectRecordsRepo := repository.NewObjectRecordsRepo(db)
	kbHubRepo := repository.NewKBHubRepo(db)
	billingRepo := repository.NewBillingRepo(db)
	kbEmbeddingRepo := repository.NewKBEmbeddingRepo(db)
	backgroundJobRunRepo := repository.NewBackgroundJobRunRepo(db)

	// Initialize services
	msgService := service.NewMessageService(convRepo, userRepo)
	syncService := service.NewSyncService(syncRepo, hub)
	auditSvc := service.NewAuditService(auditRepo)
	sensitiveOperationSvc := service.NewSensitiveOperationService(userRepo, sensitiveOperationRepo, auditSvc)
	billingSvc := service.NewKBBillingService(billingRepo)
	objectStorageCfg := cfg.ObjectStorage
	if objectStorageCfg.Endpoint == "" {
		objectStorageCfg = config.ObjectStorageConfig{Endpoint: "localhost:9000", PublicEndpoint: "localhost:9000", AccessKey: "minioadmin", SecretKey: "minioadmin", Bucket: "agentos-objects", UploadTTLSecs: 900, DownloadTTLSecs: 900}
	}
	objectStorageBackend, err := service.NewMinIOStorageService(objectStorageCfg)
	if err != nil {
		log.Fatalf("Failed to init object storage: %v", err)
	}
	objectSvc := service.NewObjectService(objectRecordsRepo, objectStorageBackend, objectStorageCfg)
	kbHubSvc := service.NewKBHubService(kbHubRepo, knowledgeEntriesRepo, objectSvc)
	kbHubSvc.SetBillingService(billingSvc)

	// Inject sync service into message service (avoids import cycle)
	msgService.SetSyncService(syncService)

	// Start background tasks
	embeddingProvider := service.NewEmbeddingProvider(service.EmbeddingProviderConfig{Provider: cfg.Embedding.Provider, Endpoint: cfg.Embedding.Endpoint, Model: cfg.Embedding.Model, Dimensions: cfg.Embedding.Dimensions, Timeout: time.Duration(cfg.Embedding.TimeoutSecs) * time.Second, MaxBatchSize: cfg.Embedding.MaxBatchSize})
	embeddingWorker := service.NewKBEmbeddingWorker(kbEmbeddingRepo, embeddingProvider, service.KBEmbeddingWorkerConfig{WorkerID: cfg.Background.EmbeddingWorkerID, BatchSize: cfg.Background.EmbeddingWorkerBatchSize, PollInterval: time.Duration(cfg.Background.EmbeddingWorkerPollIntervalSeconds) * time.Second})
	bgTasks := service.NewBackgroundTasksWithConfig(service.BackgroundTasksConfig{
		OfflineMessageCleanupInterval:        time.Duration(cfg.Background.OfflineMessageCleanupIntervalSeconds) * time.Second,
		SyncEventCleanupInterval:             time.Duration(cfg.Background.SyncEventCleanupIntervalSeconds) * time.Second,
		SensitiveConfirmationCleanupInterval: time.Duration(cfg.Background.SensitiveConfirmationIntervalSeconds) * time.Second,
		KBSubscriptionExpiryInterval:         time.Duration(cfg.Background.KBSubscriptionExpiryIntervalSeconds) * time.Second,
		EmbeddingWorkerEnabled:               cfg.Background.EmbeddingWorkerEnabled,
		EmbeddingWorkerPollInterval:          time.Duration(cfg.Background.EmbeddingWorkerPollIntervalSeconds) * time.Second,
		RunOnStart:                           cfg.Background.RunOnStart,
	}, msgService, syncService, sensitiveOperationSvc, kbHubSvc, embeddingWorker)
	bgTasks.SetJobRunRepo(backgroundJobRunRepo)
	bgTasks.Start(ctx)
	log.Println("Background tasks started")

	// Setup routes
	r := router.Setup(cfg, db, rdb, hub, userRepo, convRepo, syncRepo, msgService, syncService, signatureVerifier, bgTasks)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down gracefully...")
		cancel() // Stop background tasks
	}()

	// Start server
	port := cfg.App.Port
	if p := os.Getenv("APP_PORT"); p != "" {
		port = p
	}
	log.Printf("AgentOS Backend starting on :%s (env=%s)", port, cfg.App.Env)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
