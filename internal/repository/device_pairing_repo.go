package repository

import (
	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
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
