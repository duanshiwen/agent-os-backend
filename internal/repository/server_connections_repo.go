package repository

import (
	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServerConnectionsRepo struct {
	db *gorm.DB
}

func NewServerConnectionsRepo(db *gorm.DB) *ServerConnectionsRepo {
	return &ServerConnectionsRepo{db: db}
}

func (r *ServerConnectionsRepo) ListByUser(userID uuid.UUID) ([]model.UserServerConnection, error) {
	var connections []model.UserServerConnection
	err := r.db.Where("user_id = ?", userID).Order("created_at ASC").Find(&connections).Error
	return connections, err
}

func (r *ServerConnectionsRepo) GetByUserAndID(userID, id uuid.UUID) (*model.UserServerConnection, error) {
	var connection model.UserServerConnection
	err := r.db.Where("user_id = ? AND id = ?", userID, id).First(&connection).Error
	return &connection, err
}

func (r *ServerConnectionsRepo) GetByUserAndServerID(userID uuid.UUID, serverID string) (*model.UserServerConnection, error) {
	var connection model.UserServerConnection
	err := r.db.Where("user_id = ? AND server_id = ?", userID, serverID).First(&connection).Error
	return &connection, err
}

func (r *ServerConnectionsRepo) Create(connection *model.UserServerConnection) error {
	return r.db.Create(connection).Error
}

func (r *ServerConnectionsRepo) Update(connection *model.UserServerConnection) error {
	return r.db.Save(connection).Error
}

func (r *ServerConnectionsRepo) Delete(connection *model.UserServerConnection) error {
	return r.db.Delete(connection).Error
}
