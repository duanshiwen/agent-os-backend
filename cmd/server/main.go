package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/router"
	"github.com/agent-os/backend/internal/service"
	"github.com/agent-os/backend/internal/sidecar"
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

	// Initialize Rust sidecar client if available.
	var sidecarClient *sidecar.Client
	if cfg.Sidecar.Enabled {
		client, err := sidecar.NewClient(cfg.Sidecar.UnixSocket)
		if err != nil {
			log.Fatalf("Failed to create sidecar client: %v", err)
		}
		defer client.Close()
		if _, err := client.HealthCheck(ctx); err != nil {
			if cfg.App.Env == "production" {
				log.Fatalf("Rust sidecar health check failed in production: %v", err)
			}
			log.Printf("WARN: Rust sidecar unavailable, falling back to Go verifier: %v", err)
		} else {
			sidecarClient = client
			log.Println("Rust sidecar connected")
		}
	}

	// Initialize WebSocket hub
	hub := ws.NewHub()
	go hub.Run()
	log.Println("WebSocket hub started")

	// Initialize repositories
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	syncRepo := repository.NewSyncRepo(db)

	// Initialize services
	msgService := service.NewMessageService(convRepo, userRepo)
	syncService := service.NewSyncService(syncRepo, hub)

	// Inject sync service into message service (avoids import cycle)
	msgService.SetSyncService(syncService)

	// Start background tasks
	bgTasks := service.NewBackgroundTasks(msgService, syncService)
	bgTasks.Start(ctx)
	log.Println("Background tasks started")

	// Setup routes
	r := router.Setup(cfg, db, rdb, hub, userRepo, convRepo, syncRepo, msgService, syncService, sidecarClient)

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
