package router

import (
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
	admissionRepo := repository.NewAdmissionRepo(db)
	admissionSvc := service.NewAdmissionServiceWithRepo(userRepo, admissionRepo, cfg.Admission)
	identitySvc := service.NewIdentityServiceWithAdmission(userRepo, cfg.JWT, signatureVerifier, admissionSvc)
	identitySvc.SetSyncService(syncSvc)
	convSvc := service.NewConversationService(convRepo, userRepo)
	pairingRepo := repository.NewDevicePairingRepo(db)
	pairingSvc := service.NewDevicePairingService(pairingRepo, userRepo, signatureVerifier)
	skillSettingsRepo := repository.NewSkillSettingsRepo(db)
	skillSettingsSvc := service.NewSkillSettingsService(skillSettingsRepo, syncSvc)

	// Handlers
	identityH := handler.NewIdentityHandler(identitySvc)
	admissionH := handler.NewAdmissionHandler(admissionSvc)
	convH := handler.NewConversationHandler(convSvc, msgSvc)
	syncH := handler.NewSyncHandler(syncSvc)
	pairingH := handler.NewDevicePairingHandler(pairingSvc)
	skillSettingsH := handler.NewSkillSettingsHandler(skillSettingsSvc)

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

		// Protected routes
		protected := v1.Group("")
		protected.Use(middleware.JWTAuth(cfg.JWT.Secret))
		{
			protected.POST("/devices/pairing/start", pairingH.Start)
			protected.GET("/users/me", identityH.GetProfile)
			protected.PUT("/users/me", identityH.UpdateProfile)
			protected.POST("/users/me/devices", identityH.PairDevice)
			protected.GET("/users/me/devices", identityH.GetDevices)

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
		}

		// Admin routes
		admin := v1.Group("/admin")
		admin.Use(middleware.JWTAuth(cfg.JWT.Secret))
		admin.Use(middleware.AdminMiddleware(userRepo))
		{
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
