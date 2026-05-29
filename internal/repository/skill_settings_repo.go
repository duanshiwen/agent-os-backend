package repository

import (
	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SkillSettingsRepo struct {
	db *gorm.DB
}

func NewSkillSettingsRepo(db *gorm.DB) *SkillSettingsRepo {
	return &SkillSettingsRepo{db: db}
}

func (r *SkillSettingsRepo) ListByUser(userID uuid.UUID) ([]model.UserSkillSetting, error) {
	var settings []model.UserSkillSetting
	err := r.db.Where("user_id = ?", userID).Order("skill_id ASC").Find(&settings).Error
	return settings, err
}

func (r *SkillSettingsRepo) GetByUserAndSkill(userID uuid.UUID, skillID string) (*model.UserSkillSetting, error) {
	var setting model.UserSkillSetting
	err := r.db.Where("user_id = ? AND skill_id = ?", userID, skillID).First(&setting).Error
	return &setting, err
}

func (r *SkillSettingsRepo) Upsert(setting *model.UserSkillSetting) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "skill_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"enabled",
			"config",
			"updated_by_device_id",
			"updated_at",
		}),
	}).Create(setting).Error
}
