package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ObjectStatusPending = "pending"
	ObjectStatusActive  = "active"
	ObjectStatusDeleted = "deleted"
)

type ObjectRecordsRepo struct {
	db *gorm.DB
}

func NewObjectRecordsRepo(db *gorm.DB) *ObjectRecordsRepo {
	return &ObjectRecordsRepo{db: db}
}

func (r *ObjectRecordsRepo) Create(record *model.ObjectRecord) error {
	return r.db.Create(record).Error
}

func (r *ObjectRecordsRepo) GetByID(id uuid.UUID) (*model.ObjectRecord, error) {
	var record model.ObjectRecord
	err := r.db.First(&record, "id = ?", id).Error
	return &record, err
}

func (r *ObjectRecordsRepo) GetByIDForOwner(ownerID uuid.UUID, id uuid.UUID) (*model.ObjectRecord, error) {
	var record model.ObjectRecord
	err := r.db.First(&record, "id = ? AND owner_id = ?", id, ownerID).Error
	return &record, err
}

func (r *ObjectRecordsRepo) MarkActive(id uuid.UUID) error {
	now := time.Now()
	return r.db.Model(&model.ObjectRecord{}).
		Where("id = ? AND status = ?", id, ObjectStatusPending).
		Updates(map[string]any{"status": ObjectStatusActive, "completed_at": now, "updated_at": now}).Error
}

func (r *ObjectRecordsRepo) SoftDelete(id uuid.UUID) error {
	now := time.Now()
	res := r.db.Model(&model.ObjectRecord{}).
		Where("id = ? AND status <> ?", id, ObjectStatusDeleted).
		Updates(map[string]any{"status": ObjectStatusDeleted, "deleted_at": now, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
