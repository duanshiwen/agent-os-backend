package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
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

// GetNextSequence returns the next sequence number for a user.
func (r *SyncRepo) GetNextSequence(userID uuid.UUID) (uint64, error) {
	var maxSeq struct {
		Max uint64
	}
	err := r.db.Model(&model.SyncEvent{}).
		Where("user_id = ?", userID).
		Select("COALESCE(MAX(sequence), 0) as max").
		Scan(&maxSeq).Error
	return maxSeq.Max + 1, err
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
	cursor := &model.SyncCursor{
		UserID:             userID,
		DeviceID:           deviceID,
		LastSyncedSequence: sequence,
		UpdatedAt:          time.Now(),
	}
	return r.db.Save(cursor).Error
}

func (r *SyncRepo) CleanupOldEvents(before time.Time) (int64, error) {
	result := r.db.Where("timestamp < ?", before).Delete(&model.SyncEvent{})
	return result.RowsAffected, result.Error
}
