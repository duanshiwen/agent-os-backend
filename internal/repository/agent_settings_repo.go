package repository

import (
	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentSettingsRepo struct {
	db *gorm.DB
}

func NewAgentSettingsRepo(db *gorm.DB) *AgentSettingsRepo {
	return &AgentSettingsRepo{db: db}
}

func (r *AgentSettingsRepo) ListByUser(userID uuid.UUID) ([]model.UserAgentSetting, error) {
	var settings []model.UserAgentSetting
	err := r.db.Where("user_id = ?", userID).Order("agent_id ASC").Find(&settings).Error
	return settings, err
}

func (r *AgentSettingsRepo) GetByUserAndAgent(userID uuid.UUID, agentID string) (*model.UserAgentSetting, error) {
	var setting model.UserAgentSetting
	err := r.db.Where("user_id = ? AND agent_id = ?", userID, agentID).First(&setting).Error
	return &setting, err
}

func (r *AgentSettingsRepo) Upsert(setting *model.UserAgentSetting) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "agent_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"display_name",
			"config",
			"updated_by_device_id",
			"updated_at",
		}),
	}).Create(setting).Error
}
