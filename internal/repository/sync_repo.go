package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SyncRepo struct {
	db *gorm.DB
}

func NewSyncRepo(db *gorm.DB) *SyncRepo {
	return &SyncRepo{db: db}
}

// === SyncEvent ===

func (r *SyncRepo) CreateEvent(event *model.SyncEvent) error {
	return r.db.Create(event).Error
}

func (r *SyncRepo) GetEventsSince(userID uuid.UUID, deviceID string, sinceSequence uint64, limit int) ([]model.SyncEvent, error) {
	var events []model.SyncEvent
	err := r.db.
		Where("user_id = ? AND sequence > ?", userID, sinceSequence).
		Order("sequence ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *SyncRepo) GetEventByClientEventID(userID uuid.UUID, clientEventID string) (*model.SyncEvent, error) {
	if clientEventID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var event model.SyncEvent
	err := r.db.
		Where("user_id = ? AND client_event_id = ?", userID, clientEventID).
		First(&event).Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *SyncRepo) GetEventsByType(userID uuid.UUID, eventType string, since time.Time, limit int) ([]model.SyncEvent, error) {
	var events []model.SyncEvent
	err := r.db.
		Where("user_id = ? AND event_type LIKE ? AND timestamp > ?", userID, eventType+"%", since).
		Order("sequence ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

// GetNextSequence atomically reserves the next sequence number for a user.
func (r *SyncRepo) GetNextSequence(userID uuid.UUID) (uint64, error) {
	var reserved uint64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		seed := model.SyncSequence{UserID: userID, NextSequence: 1, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}

		var seq model.SyncSequence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userID).
			First(&seq).Error; err != nil {
			return err
		}

		reserved = seq.NextSequence
		return tx.Model(&model.SyncSequence{}).
			Where("user_id = ?", userID).
			Updates(map[string]any{
				"next_sequence": seq.NextSequence + 1,
				"updated_at":    now,
			}).Error
	})
	return reserved, err
}

// === SyncCursor ===

func (r *SyncRepo) GetCursor(userID uuid.UUID, deviceID string) (*model.SyncCursor, error) {
	var cursor model.SyncCursor
	err := r.db.
		Where("user_id = ? AND device_id = ?", userID, deviceID).
		First(&cursor).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &model.SyncCursor{
				UserID:             userID,
				DeviceID:           deviceID,
				LastSyncedSequence: 0,
			}, nil
		}
		return nil, err
	}
	return &cursor, nil
}

func (r *SyncRepo) UpdateCursor(userID uuid.UUID, deviceID string, sequence uint64) error {
	cursor, err := r.GetCursor(userID, deviceID)
	if err != nil {
		return err
	}
	if sequence < cursor.LastSyncedSequence {
		sequence = cursor.LastSyncedSequence
	}
	cursor.LastSyncedSequence = sequence
	cursor.UpdatedAt = time.Now()
	return r.db.Save(cursor).Error
}

func (r *SyncRepo) CleanupOldEvents(before time.Time) (int64, error) {
	result := r.db.Where("timestamp < ?", before).Delete(&model.SyncEvent{})
	return result.RowsAffected, result.Error
}
