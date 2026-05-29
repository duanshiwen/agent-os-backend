package service

import (
	"fmt"
	"strings"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type AgentSettingsService struct {
	repo    *repository.AgentSettingsRepo
	syncSvc *SyncService
}

func NewAgentSettingsService(repo *repository.AgentSettingsRepo, syncSvc *SyncService) *AgentSettingsService {
	return &AgentSettingsService{repo: repo, syncSvc: syncSvc}
}

func (s *AgentSettingsService) ListSettings(userID uuid.UUID) ([]model.UserAgentSetting, error) {
	return s.repo.ListByUser(userID)
}

func (s *AgentSettingsService) UpdateAgent(userID uuid.UUID, sourceDeviceID, agentID string, displayName *string, config datatypes.JSONMap, clientEventID string) (*model.UserAgentSetting, *model.SyncEvent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil, fmt.Errorf("agent_id is required")
	}
	if config == nil {
		config = datatypes.JSONMap{}
	}

	name := ""
	if displayName != nil {
		name = *displayName
	}

	setting := &model.UserAgentSetting{
		UserID:            userID,
		AgentID:           agentID,
		DisplayName:       name,
		Config:            config,
		UpdatedByDeviceID: sourceDeviceID,
	}
	if err := s.repo.Upsert(setting); err != nil {
		return nil, nil, fmt.Errorf("upsert agent setting: %w", err)
	}

	persisted, err := s.repo.GetByUserAndAgent(userID, agentID)
	if err != nil {
		return nil, nil, fmt.Errorf("get agent setting: %w", err)
	}

	var event *model.SyncEvent
	if s.syncSvc != nil {
		payload := datatypes.JSONMap{
			"object_id":            agentID,
			"agent_id":             agentID,
			"display_name":         persisted.DisplayName,
			"config":               persisted.Config,
			"updated_by_device_id": persisted.UpdatedByDeviceID,
			"updated_at":           persisted.UpdatedAt,
		}
		event, err = s.syncSvc.RecordEnvelope(SyncEnvelope{
			UserID:         userID,
			SourceDeviceID: sourceDeviceID,
			ObjectType:     SyncObjectAgent,
			ObjectID:       agentID,
			Operation:      SyncOperationUpdated,
			ClientEventID:  clientEventID,
			Payload:        payload,
		})
		if err != nil {
			return nil, nil, err
		}
	}

	return persisted, event, nil
}
