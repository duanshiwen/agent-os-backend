package service

import (
	"context"
	"log"
	"time"
)

// BackgroundTasks manages periodic cleanup and maintenance tasks.
type BackgroundTasks struct {
	msgService  *MessageService
	syncService *SyncService
}

func NewBackgroundTasks(msgService *MessageService, syncService *SyncService) *BackgroundTasks {
	return &BackgroundTasks{
		msgService:  msgService,
		syncService: syncService,
	}
}

// Start begins all background tasks. Cancel the context to stop them.
func (bt *BackgroundTasks) Start(ctx context.Context) {
	go bt.cleanupOfflineMessages(ctx)
	go bt.cleanupSyncEvents(ctx)
}

func (bt *BackgroundTasks) cleanupOfflineMessages(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	log.Println("background: offline message cleanup started (every 1h)")
	for {
		select {
		case <-ctx.Done():
			log.Println("background: offline message cleanup stopped")
			return
		case <-ticker.C:
			count, err := bt.msgService.CleanupExpiredOffline(7) // 7 days retention
			if err != nil {
				log.Printf("background: offline cleanup error: %v", err)
			} else if count > 0 {
				log.Printf("background: cleaned up %d expired offline messages", count)
			}
		}
	}
}

func (bt *BackgroundTasks) cleanupSyncEvents(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	log.Println("background: sync event cleanup started (every 6h)")
	for {
		select {
		case <-ctx.Done():
			log.Println("background: sync event cleanup stopped")
			return
		case <-ticker.C:
			count, err := bt.syncService.CleanupOldEvents(30) // 30 days retention
			if err != nil {
				log.Printf("background: sync cleanup error: %v", err)
			} else if count > 0 {
				log.Printf("background: cleaned up %d old sync events", count)
			}
		}
	}
}
