package repository

import (
	"github.com/agent-os/backend/internal/model"
	"gorm.io/gorm"
)

type AdmissionRepo struct {
	db *gorm.DB
}

func NewAdmissionRepo(db *gorm.DB) *AdmissionRepo {
	return &AdmissionRepo{db: db}
}

func (r *AdmissionRepo) GetServerAdmission(serverID string) (*model.ServerAdmission, error) {
	var admission model.ServerAdmission
	err := r.db.Where("server_id = ?", serverID).First(&admission).Error
	return &admission, err
}

func (r *AdmissionRepo) SaveServerAdmission(admission *model.ServerAdmission) error {
	return r.db.Save(admission).Error
}

func (r *AdmissionRepo) CreateServerAdmission(admission *model.ServerAdmission) error {
	return r.db.Create(admission).Error
}
