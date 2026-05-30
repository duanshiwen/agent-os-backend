package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	db, err := config.InitDatabase(&cfg.Database, cfg.App.AutoMigrate)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	provider := service.NewEmbeddingProvider(service.EmbeddingProviderConfig{Provider: cfg.Embedding.Provider, Endpoint: cfg.Embedding.Endpoint, Model: cfg.Embedding.Model, Dimensions: cfg.Embedding.Dimensions, Timeout: time.Duration(cfg.Embedding.TimeoutSecs) * time.Second, MaxBatchSize: cfg.Embedding.MaxBatchSize})
	worker := service.NewKBEmbeddingWorker(repository.NewKBEmbeddingRepo(db), provider, service.KBEmbeddingWorkerConfig{WorkerID: getenv("EMBEDDING_WORKER_ID", ""), BatchSize: getenvInt("EMBEDDING_WORKER_BATCH_SIZE", 8), PollInterval: time.Duration(getenvInt("EMBEDDING_WORKER_POLL_INTERVAL_SECONDS", 2)) * time.Second})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("starting KB embedding worker provider=%s model=%s dimensions=%d", provider.Name(), provider.Model(), provider.Dimensions())
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("embedding worker stopped: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}
