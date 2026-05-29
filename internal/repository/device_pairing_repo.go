package repository

import (
	"fmt"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DevicePairingRepo struct {
	db *gorm.DB
}

func NewDevicePairingRepo(db *gorm.DB) *DevicePairingRepo {
	return &DevicePairingRepo{db: db}
}

func (r *DevicePairingRepo) CreatePairingSession(session *model.DevicePairingSession) error {
	return r.db.Create(session).Error
}

func (r *DevicePairingRepo) GetPairingSession(id uuid.UUID) (*model.DevicePairingSession, error) {
	var session model.DevicePairingSession
	err := r.db.Where("id = ?", id).First(&session).Error
	return &session, err
}

func (r *DevicePairingRepo) SavePairingSession(session *model.DevicePairingSession) error {
	return r.db.Save(session).Error
}

// ClaimPairingSession atomically creates the claimed device and marks the pairing
// session as used. The row lock plus used_at guard prevent duplicate concurrent
// claims for the same QR payload.
func (r *DevicePairingRepo) ClaimPairingSession(sessionID uuid.UUID, device *model.Device) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var session model.DevicePairingSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&session).Error; err != nil {
			return fmt.Errorf("pairing session not found: %w", err)
		}
		if session.UsedAt != nil {
			return fmt.Errorf("pairing session already used")
		}
		if session.ExpiresAt.Before(time.Now()) {
			return fmt.Errorf("pairing session expired")
		}
		if err := tx.Create(device).Error; err != nil {
			return fmt.Errorf("create paired device: %w", err)
		}
		now := time.Now()
		updates := map[string]any{
			"used_at":              &now,
			"claimed_by_device_id": device.DeviceID,
			"updated_at":           now,
		}
		result := tx.Model(&model.DevicePairingSession{}).
			Where("id = ? AND used_at IS NULL", sessionID).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("mark pairing session used: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("pairing session already used")
		}
		return nil
	})
}
