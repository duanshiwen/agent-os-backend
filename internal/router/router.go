package router

import (
	"time"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/handler"
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/agent-os/backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func Setup(
	cfg *config.Config,
	db *gorm.DB,
	rdb *redis.Client,
	hub *ws.Hub,
	userRepo *repository.UserRepo,
	convRepo *repository.ConversationRepo,
	syncRepo *repository.SyncRepo,
	msgSvc *service.MessageService,
	syncSvc *service.SyncService,
	signatureVerifier service.SignatureVerifier,
) *gin.Engine {
	r := gin.Default()

	// Middleware
	r.Use(middleware.CORS(cfg.CORS.AllowOrigins))
	r.Use(middleware.RateLimiter(10, 50))

	// Services (using injected repos)
	auditRepo := repository.NewAuditRepo(db)
	auditSvc := service.NewAuditService(auditRepo)
	admissionRepo := repository.NewAdmissionRepo(db)
	admissionSvc := service.NewAdmissionServiceWithRepo(userRepo, admissionRepo, cfg.Admission)
	identitySvc := service.NewIdentityServiceWithAdmission(userRepo, cfg.JWT, signatureVerifier, admissionSvc)
	identitySvc.SetSyncService(syncSvc)
	convSvc := service.NewConversationService(convRepo, userRepo)
	pairingRepo := repository.NewDevicePairingRepo(db)
	pairingSvc := service.NewDevicePairingService(pairingRepo, userRepo, signatureVerifier)
	pairingSvc.SetAuditService(auditSvc)
	skillSettingsRepo := repository.NewSkillSettingsRepo(db)
	skillSettingsSvc := service.NewSkillSettingsService(skillSettingsRepo, syncSvc)
	agentSettingsRepo := repository.NewAgentSettingsRepo(db)
	agentSettingsSvc := service.NewAgentSettingsService(agentSettingsRepo, syncSvc)
	serverConnectionsRepo := repository.NewServerConnectionsRepo(db)
	serverConnectionsSvc := service.NewServerConnectionsService(serverConnectionsRepo, syncSvc)
	knowledgeEntriesRepo := repository.NewKnowledgeEntriesRepo(db)
	knowledgeEntriesSvc := service.NewKnowledgeEntriesService(knowledgeEntriesRepo, syncSvc)
	objectRecordsRepo := repository.NewObjectRecordsRepo(db)
	objectStorageCfg := cfg.ObjectStorage
	if objectStorageCfg.Endpoint == "" {
		objectStorageCfg = config.ObjectStorageConfig{Endpoint: "localhost:9000", PublicEndpoint: "localhost:9000", AccessKey: "minioadmin", SecretKey: "minioadmin", Bucket: "agentos-objects", UploadTTLSecs: 900, DownloadTTLSecs: 900}
	}
	objectStorageBackend, err := service.NewMinIOStorageService(objectStorageCfg)
	if err != nil {
		panic(err)
	}
	objectSvc := service.NewObjectService(objectRecordsRepo, objectStorageBackend, objectStorageCfg)
	kbHubRepo := repository.NewKBHubRepo(db)
	billingRepo := repository.NewBillingRepo(db)
	billingSvc := service.NewKBBillingService(billingRepo)
	kbSearchRepo := repository.NewKBSearchRepo(db)
	kbEmbeddingRepo := repository.NewKBEmbeddingRepo(db)
	embeddingProvider := service.NewEmbeddingProvider(service.EmbeddingProviderConfig{Provider: cfg.Embedding.Provider, Endpoint: cfg.Embedding.Endpoint, Model: cfg.Embedding.Model, Dimensions: cfg.Embedding.Dimensions, Timeout: time.Duration(cfg.Embedding.TimeoutSecs) * time.Second, MaxBatchSize: cfg.Embedding.MaxBatchSize})
	kbSearchSvc := service.NewKBSearchService(kbSearchRepo, kbHubRepo, billingSvc)
	kbSearchSvc.SetEmbedding(kbEmbeddingRepo, embeddingProvider)
	kbHubSvc := service.NewKBHubService(kbHubRepo, knowledgeEntriesRepo, objectSvc)
	kbHubSvc.SetBillingService(billingSvc)
	kbHubSvc.SetSearchService(kbSearchSvc)

	// Handlers
	identityH := handler.NewIdentityHandler(identitySvc)
	identityH.SetAuditService(auditSvc)
	auditH := handler.NewAuditHandler(auditSvc)
	admissionH := handler.NewAdmissionHandler(admissionSvc)
	admissionH.SetAuditService(auditSvc)
	convH := handler.NewConversationHandler(convSvc, msgSvc)
	syncH := handler.NewSyncHandler(syncSvc)
	pairingH := handler.NewDevicePairingHandler(pairingSvc)
	skillSettingsH := handler.NewSkillSettingsHandler(skillSettingsSvc)
	agentSettingsH := handler.NewAgentSettingsHandler(agentSettingsSvc)
	serverConnectionsH := handler.NewServerConnectionsHandler(serverConnectionsSvc)
	knowledgeEntriesH := handler.NewKnowledgeEntriesHandler(knowledgeEntriesSvc)
	objectH := handler.NewObjectHandler(objectSvc)
	kbHubH := handler.NewKBHubHandler(kbHubSvc)
	kbHubH.SetSearchService(kbSearchSvc)
	kbHubH.SetBillingService(billingSvc)

	// WebSocket dispatcher
	dispatcher := ws.NewDispatcher(hub, msgSvc, convSvc, convRepo)
	wsH := handler.NewWebSocketHandler(hub, dispatcher, cfg.JWT.Secret)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		dbOK := true
		if sqlDB, err := db.DB(); err != nil || sqlDB.Ping() != nil {
			dbOK = false
		}
		redisOK := true
		if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
			redisOK = false
		}
		identityVerifierOK := signatureVerifier != nil
		status := "ok"
		statusCode := 200
		if !dbOK || !redisOK || !identityVerifierOK {
			status = "degraded"
			statusCode = 503
		}
		c.JSON(statusCode, gin.H{
			"status":  status,
			"service": cfg.App.Name,
			"checks":  gin.H{"database": dbOK, "redis": redisOK, "identity_verifier": identityVerifierOK},
			"identity_verifier": gin.H{
				"backend": service.SignatureVerifierBackend(signatureVerifier),
				"version": service.SignatureVerifierVersion(signatureVerifier),
			},
		})
	})

	// API v1
	v1 := r.Group("/api/v1")
	{
		// Auth (public)
		auth := v1.Group("/auth")
		{
			auth.POST("/challenge", identityH.Challenge)
			auth.POST("/verify", identityH.Verify)
			auth.POST("/register", identityH.Register)
		}

		// WebSocket
		v1.GET("/ws", wsH.HandleWS)

		// Public QR pairing claim route. The qr_payload itself carries the one-time pairing proof.
		v1.POST("/devices/pairing/claim", pairingH.Claim)

		// Public KB Hub read routes
		v1.GET("/kb/public/collections", kbHubH.ListPublicCollections)
		v1.GET("/kb/public/search", kbHubH.Search)
		v1.GET("/kb/public/collections/:id", kbHubH.GetPublicCollection)
		v1.GET("/kb/public/collections/:id/snapshots/:snapshot_id", kbHubH.GetPublicSnapshot)
		v1.POST("/kb/public/collections/:id/snapshots/:snapshot_id/manifest-download-url", kbHubH.CreateManifestDownloadURL)

		// Protected routes
		protected := v1.Group("")
		protected.Use(middleware.JWTAuth(cfg.JWT.Secret))
		{
			protected.POST("/devices/pairing/start", pairingH.Start)
			protected.GET("/users/me", identityH.GetProfile)
			protected.PUT("/users/me", identityH.UpdateProfile)
			protected.POST("/users/me/devices", identityH.PairDevice)
			protected.GET("/users/me/devices", identityH.GetDevices)
			protected.PUT("/users/me/devices/:device_id", identityH.RenameDevice)
			protected.DELETE("/users/me/devices/:device_id", identityH.RevokeDevice)

			protected.POST("/conversations", convH.Create)
			protected.GET("/conversations", convH.List)
			protected.GET("/conversations/:id", convH.Get)
			protected.GET("/conversations/:id/messages", convH.GetMessages)
			protected.POST("/conversations/:id/participants", convH.AddParticipant)
			protected.DELETE("/conversations/:id/participants/me", convH.Leave)

			protected.GET("/sync/events", syncH.GetEvents)
			protected.POST("/sync/ack", syncH.AckEvents)

			protected.GET("/skills/settings", skillSettingsH.List)
			protected.PUT("/skills/settings/:skill_id", skillSettingsH.Update)
			protected.POST("/skills/settings/:skill_id/enable", skillSettingsH.Enable)
			protected.POST("/skills/settings/:skill_id/disable", skillSettingsH.Disable)

			protected.GET("/agents/settings", agentSettingsH.List)
			protected.PUT("/agents/settings/:agent_id", agentSettingsH.Update)

			protected.GET("/servers", serverConnectionsH.List)
			protected.POST("/servers", serverConnectionsH.Create)
			protected.PUT("/servers/:id", serverConnectionsH.Update)
			protected.DELETE("/servers/:id", serverConnectionsH.Delete)

			protected.GET("/knowledge/entries", knowledgeEntriesH.List)
			protected.POST("/knowledge/entries", knowledgeEntriesH.Create)
			protected.GET("/knowledge/entries/*entry_id", knowledgeEntriesH.Get)
			protected.PUT("/knowledge/entries/*entry_id", knowledgeEntriesH.Update)
			protected.DELETE("/knowledge/entries/*entry_id", knowledgeEntriesH.Delete)

			protected.POST("/objects/upload-intents", objectH.CreateUploadIntent)
			protected.POST("/objects/uploads/:id/complete", objectH.CompleteUpload)
			protected.GET("/objects/:id", objectH.Get)
			protected.POST("/objects/:id/download-url", objectH.CreateDownloadURL)
			protected.DELETE("/objects/:id", objectH.Delete)

			protected.GET("/billing/account", kbHubH.GetBillingAccount)
			protected.GET("/billing/transactions", kbHubH.ListBillingTransactions)

			protected.POST("/kb/collections", kbHubH.CreateCollection)
			protected.GET("/kb/collections", kbHubH.ListCollections)
			protected.PUT("/kb/collections/:id/pricing", kbHubH.UpdateCollectionPricing)
			protected.GET("/kb/collections/:id/stats", kbHubH.GetCollectionStats)
			protected.GET("/kb/collections/:id/earnings", kbHubH.ListContributorEarnings)
			protected.POST("/kb/collections/:id/snapshots", kbHubH.PublishSnapshot)
			protected.GET("/kb/collections/:id/snapshots", kbHubH.ListSnapshots)
			protected.GET("/kb/collections/:id/snapshots/:snapshot_id", kbHubH.GetSnapshot)
			protected.GET("/kb/collections/:id/snapshots/:snapshot_id/embedding-status", kbHubH.GetSnapshotEmbeddingStatus)
			protected.POST("/kb/collections/:id/snapshots/:snapshot_id/embedding-jobs/retry-failed", kbHubH.RetrySnapshotEmbeddingJobs)
			protected.POST("/kb/collections/:id/install", kbHubH.InstallCollection)
			protected.DELETE("/kb/collections/:id/install", kbHubH.CancelSubscription)
			protected.POST("/kb/collections/:id/snapshots/:snapshot_id/manifest-download-url", kbHubH.CreateInstalledManifestDownloadURL)
			protected.POST("/kb/collections/:id/snapshots/:snapshot_id/entries/:entry_id/content-download-url", kbHubH.CreateInstalledEntryContentDownloadURL)
			protected.POST("/kb/collections/:id/snapshots/:snapshot_id/entries/:entry_id/fulltext", kbHubH.FetchInstalledEntryFullText)
			protected.GET("/kb/collections/:id", kbHubH.GetCollection)
			protected.GET("/kb/subscriptions", kbHubH.ListSubscriptions)
		}

		// Admin routes
		admin := v1.Group("/admin")
		admin.Use(middleware.JWTAuth(cfg.JWT.Secret))
		admin.Use(middleware.AdminMiddleware(userRepo))
		{
			admin.GET("/audit/events", auditH.List)
			admin.GET("/admission/policy", admissionH.GetPolicy)
			admin.PUT("/admission/policy", admissionH.UpdatePolicy)
			admin.PUT("/admission/invitation-code", admissionH.UpdateInvitationCode)
			admin.GET("/admission/requests", admissionH.GetPending)
			admin.POST("/admission/requests/:id/approve", admissionH.Approve)
			admin.POST("/admission/requests/:id/reject", admissionH.Reject)
		}
	}

	return r
}
