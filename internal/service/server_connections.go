package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ServerConnectionsService struct {
	repo    *repository.ServerConnectionsRepo
	syncSvc *SyncService
}

func NewServerConnectionsService(repo *repository.ServerConnectionsRepo, syncSvc *SyncService) *ServerConnectionsService {
	return &ServerConnectionsService{repo: repo, syncSvc: syncSvc}
}

type AddServerConnectionRequest struct {
	ServerID      string            `json:"server_id" binding:"required"`
	Name          string            `json:"name"`
	BaseURL       string            `json:"base_url"`
	Status        string            `json:"status"`
	Config        datatypes.JSONMap `json:"config"`
	ClientEventID string            `json:"client_event_id"`
}

type UpdateServerConnectionRequest struct {
	Name          *string           `json:"name"`
	BaseURL       *string           `json:"base_url"`
	Status        *string           `json:"status"`
	Config        datatypes.JSONMap `json:"config"`
	ClientEventID string            `json:"client_event_id"`
}

func (s *ServerConnectionsService) ListServers(userID uuid.UUID) ([]model.UserServerConnection, error) {
	return s.repo.ListByUser(userID)
}

func (s *ServerConnectionsService) AddServer(userID uuid.UUID, sourceDeviceID string, req AddServerConnectionRequest) (*model.UserServerConnection, *model.SyncEvent, error) {
	serverID := strings.TrimSpace(req.ServerID)
	if serverID == "" {
		return nil, nil, fmt.Errorf("server_id is required")
	}
	status := req.Status
	if status == "" {
		status = "active"
	}
	config := req.Config
	if config == nil {
		config = datatypes.JSONMap{}
	}

	connection, err := s.repo.GetByUserAndServerID(userID, serverID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("get server connection: %w", err)
		}
		connection = &model.UserServerConnection{
			UserID:            userID,
			ServerID:          serverID,
			Name:              req.Name,
			BaseURL:           req.BaseURL,
			Status:            status,
			Config:            config,
			UpdatedByDeviceID: sourceDeviceID,
		}
		if err := s.repo.Create(connection); err != nil {
			return nil, nil, fmt.Errorf("create server connection: %w", err)
		}
	} else if req.ClientEventID == "" {
		return nil, nil, fmt.Errorf("server connection already exists")
	}

	event, err := s.recordServerEvent(userID, sourceDeviceID, connection, SyncOperationAdded, req.ClientEventID, false)
	if err != nil {
		return nil, nil, err
	}
	return connection, event, nil
}

func (s *ServerConnectionsService) UpdateServer(userID uuid.UUID, sourceDeviceID string, connectionID uuid.UUID, req UpdateServerConnectionRequest) (*model.UserServerConnection, *model.SyncEvent, error) {
	connection, err := s.repo.GetByUserAndID(userID, connectionID)
	if err != nil {
		return nil, nil, fmt.Errorf("server connection not found")
	}
	if req.Name != nil {
		connection.Name = *req.Name
	}
	if req.BaseURL != nil {
		connection.BaseURL = *req.BaseURL
	}
	if req.Status != nil {
		connection.Status = *req.Status
	}
	if req.Config != nil {
		connection.Config = req.Config
	}
	connection.UpdatedByDeviceID = sourceDeviceID
	if err := s.repo.Update(connection); err != nil {
		return nil, nil, fmt.Errorf("update server connection: %w", err)
	}

	event, err := s.recordServerEvent(userID, sourceDeviceID, connection, SyncOperationUpdated, req.ClientEventID, false)
	if err != nil {
		return nil, nil, err
	}
	return connection, event, nil
}

func (s *ServerConnectionsService) RemoveServer(userID uuid.UUID, sourceDeviceID string, connectionID uuid.UUID, clientEventID string) (*model.SyncEvent, error) {
	connection, err := s.repo.GetByUserAndID(userID, connectionID)
	if err != nil {
		return nil, fmt.Errorf("server connection not found")
	}
	connection.UpdatedByDeviceID = sourceDeviceID
	connection.UpdatedAt = time.Now()
	event, err := s.recordServerEvent(userID, sourceDeviceID, connection, SyncOperationRemoved, clientEventID, true)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Delete(connection); err != nil {
		return nil, fmt.Errorf("delete server connection: %w", err)
	}
	return event, nil
}

func (s *ServerConnectionsService) recordServerEvent(userID uuid.UUID, sourceDeviceID string, connection *model.UserServerConnection, operation, clientEventID string, removed bool) (*model.SyncEvent, error) {
	if s.syncSvc == nil {
		return nil, nil
	}
	payload := datatypes.JSONMap{
		"object_id":            connection.ID.String(),
		"connection_id":        connection.ID.String(),
		"server_id":            connection.ServerID,
		"name":                 connection.Name,
		"base_url":             connection.BaseURL,
		"status":               connection.Status,
		"config":               connection.Config,
		"updated_by_device_id": connection.UpdatedByDeviceID,
		"updated_at":           connection.UpdatedAt,
	}
	if removed {
		payload["removed"] = true
	}
	return s.syncSvc.RecordEnvelope(SyncEnvelope{
		UserID:         userID,
		SourceDeviceID: sourceDeviceID,
		ObjectType:     SyncObjectServer,
		ObjectID:       connection.ID.String(),
		Operation:      operation,
		ClientEventID:  clientEventID,
		Payload:        payload,
	})
}
