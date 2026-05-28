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

func Setup(cfg *config.Config, db *gorm.DB, rdb *redis.Client) *gin.Engine {
	r := gin.Default()

	// Middleware
	r.Use(middleware.CORS(cfg.CORS.AllowOrigins))
	r.Use(middleware.RateLimiter(10, 50)) // 10 req/s, burst 50

	// Repositories
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	syncRepo := repository.NewSyncRepo(db)

	// WebSocket (must be created before services that need hub)
	hub := ws.NewHub()
	go hub.Run()

	// Services
	identitySvc := service.NewIdentityService(userRepo, cfg.JWT)
	admissionSvc := service.NewAdmissionService(userRepo, cfg.Admission)
	convSvc := service.NewConversationService(convRepo, userRepo)
	msgSvc := service.NewMessageService(convRepo, userRepo)
	syncSvc := service.NewSyncService(syncRepo, hub)

	// Handlers
	identityH := handler.NewIdentityHandler(identitySvc)
	admissionH := handler.NewAdmissionHandler(admissionSvc)
	convH := handler.NewConversationHandler(convSvc, msgSvc)
	syncH := handler.NewSyncHandler(syncSvc)

	dispatcher := ws.NewDispatcher(hub, msgSvc, convSvc, convRepo)
	wsH := handler.NewWebSocketHandler(hub, dispatcher, cfg.JWT.Secret)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": cfg.App.Name})
	})

	// API v1
	v1 := r.Group("/api/v1")
	{
		// === Auth (public) ===
		auth := v1.Group("/auth")
		{
			auth.POST("/challenge", identityH.Challenge)
			auth.POST("/verify", identityH.Verify)
			auth.POST("/register", identityH.Register)
		}

		// === WebSocket (token in query) ===
		v1.GET("/ws", wsH.HandleWS)

		// === Protected routes ===
		protected := v1.Group("")
		protected.Use(middleware.JWTAuth(cfg.JWT.Secret))
		{
			// User profile
			protected.GET("/users/me", identityH.GetProfile)
			protected.PUT("/users/me", identityH.UpdateProfile)

			// Devices
			protected.POST("/users/me/devices", identityH.PairDevice)
			protected.GET("/users/me/devices", identityH.GetDevices)

			// Conversations
			protected.POST("/conversations", convH.Create)
			protected.GET("/conversations", convH.List)
			protected.GET("/conversations/:id", convH.Get)
			protected.GET("/conversations/:id/messages", convH.GetMessages)
			protected.POST("/conversations/:id/participants", convH.AddParticipant)
			protected.DELETE("/conversations/:id/participants/me", convH.Leave)

			// Sync
			protected.GET("/sync/events", syncH.GetEvents)
			protected.POST("/sync/ack", syncH.AckEvents)
		}

		// === Admin routes ===
		admin := v1.Group("/admin")
		admin.Use(middleware.JWTAuth(cfg.JWT.Secret))
		{
			admin.GET("/admission/requests", admissionH.GetPending)
			admin.POST("/admission/requests/:id/approve", admissionH.Approve)
			admin.POST("/admission/requests/:id/reject", admissionH.Reject)
		}
	}

	return r
}
