package service

import (
	"fmt"
	"strings"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type SkillSettingsService struct {
	repo    *repository.SkillSettingsRepo
	syncSvc *SyncService
}

func NewSkillSettingsService(repo *repository.SkillSettingsRepo, syncSvc *SyncService) *SkillSettingsService {
	return &SkillSettingsService{repo: repo, syncSvc: syncSvc}
}

func (s *SkillSettingsService) ListSettings(userID uuid.UUID) ([]model.UserSkillSetting, error) {
	return s.repo.ListByUser(userID)
}

func (s *SkillSettingsService) EnableSkill(userID uuid.UUID, sourceDeviceID, skillID, clientEventID string) (*model.UserSkillSetting, *model.SyncEvent, error) {
	return s.upsertAndSync(userID, sourceDeviceID, skillID, true, nil, SyncOperationEnabled, clientEventID)
}

func (s *SkillSettingsService) DisableSkill(userID uuid.UUID, sourceDeviceID, skillID, clientEventID string) (*model.UserSkillSetting, *model.SyncEvent, error) {
	return s.upsertAndSync(userID, sourceDeviceID, skillID, false, nil, SyncOperationDisabled, clientEventID)
}

func (s *SkillSettingsService) UpdateSkill(userID uuid.UUID, sourceDeviceID, skillID string, config datatypes.JSONMap, clientEventID string) (*model.UserSkillSetting, *model.SyncEvent, error) {
	return s.upsertAndSync(userID, sourceDeviceID, skillID, true, config, SyncOperationUpdated, clientEventID)
}

func (s *SkillSettingsService) upsertAndSync(userID uuid.UUID, sourceDeviceID, skillID string, enabled bool, config datatypes.JSONMap, operation, clientEventID string) (*model.UserSkillSetting, *model.SyncEvent, error) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return nil, nil, fmt.Errorf("skill_id is required")
	}
	if config == nil {
		config = datatypes.JSONMap{}
	}

	setting := &model.UserSkillSetting{
		UserID:            userID,
		SkillID:           skillID,
		Enabled:           enabled,
		Config:            config,
		UpdatedByDeviceID: sourceDeviceID,
	}
	if err := s.repo.Upsert(setting); err != nil {
		return nil, nil, fmt.Errorf("upsert skill setting: %w", err)
	}

	persisted, err := s.repo.GetByUserAndSkill(userID, skillID)
	if err != nil {
		return nil, nil, fmt.Errorf("get skill setting: %w", err)
	}

	var event *model.SyncEvent
	if s.syncSvc != nil {
		payload := datatypes.JSONMap{
			"object_id":            skillID,
			"skill_id":             skillID,
			"enabled":              persisted.Enabled,
			"config":               persisted.Config,
			"updated_by_device_id": persisted.UpdatedByDeviceID,
			"updated_at":           persisted.UpdatedAt,
		}
		event, err = s.syncSvc.RecordEnvelope(SyncEnvelope{
			UserID:         userID,
			SourceDeviceID: sourceDeviceID,
			ObjectType:     SyncObjectSkill,
			ObjectID:       skillID,
			Operation:      operation,
			ClientEventID:  clientEventID,
			Payload:        payload,
		})
		if err != nil {
			return nil, nil, err
		}
	}

	return persisted, event, nil
}
