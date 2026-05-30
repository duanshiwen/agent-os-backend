package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SensitiveOperationRepo struct {
	db *gorm.DB
}

func NewSensitiveOperationRepo(db *gorm.DB) *SensitiveOperationRepo {
	return &SensitiveOperationRepo{db: db}
}

func (r *SensitiveOperationRepo) CreateConfirmation(confirmation *model.SensitiveOperationConfirmation) error {
	return r.db.Create(confirmation).Error
}

func (r *SensitiveOperationRepo) GetUsableConfirmation(userID uuid.UUID, tokenHash, operation string, now time.Time) (*model.SensitiveOperationConfirmation, error) {
	var confirmation model.SensitiveOperationConfirmation
	err := r.db.Where("user_id = ? AND token_hash = ? AND operation = ? AND used_at IS NULL AND expires_at > ?", userID, tokenHash, operation, now).First(&confirmation).Error
	return &confirmation, err
}

func (r *SensitiveOperationRepo) MarkUsed(id uuid.UUID, consumedBy string) error {
	now := time.Now().UTC()
	return r.db.Model(&model.SensitiveOperationConfirmation{}).Where("id = ? AND used_at IS NULL", id).Updates(map[string]any{"used_at": &now, "consumed_by": consumedBy}).Error
}
