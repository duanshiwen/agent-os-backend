package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuditRepo struct {
	db *gorm.DB
}

func NewAuditRepo(db *gorm.DB) *AuditRepo {
	return &AuditRepo{db: db}
}

type AuditEventsQuery struct {
	ActorUserID  *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      string
	Limit        int
	Offset       int
}

func (r *AuditRepo) Create(event *model.AuditEvent) error {
	return r.db.Create(event).Error
}

func (r *AuditRepo) List(query AuditEventsQuery) ([]model.AuditEvent, error) {
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	db := r.db.Model(&model.AuditEvent{})
	if query.ActorUserID != nil && *query.ActorUserID != uuid.Nil {
		db = db.Where("actor_user_id = ?", *query.ActorUserID)
	}
	if query.Action != "" {
		db = db.Where("action = ?", query.Action)
	}
	if query.ResourceType != "" {
		db = db.Where("resource_type = ?", query.ResourceType)
	}
	if query.ResourceID != "" {
		db = db.Where("resource_id = ?", query.ResourceID)
	}
	if query.Outcome != "" {
		db = db.Where("outcome = ?", query.Outcome)
	}

	var events []model.AuditEvent
	err := db.Order("occurred_at DESC").Limit(limit).Offset(query.Offset).Find(&events).Error
	return events, err
}

func (r *AuditRepo) Count(query AuditEventsQuery) (int64, error) {
	db := r.db.Model(&model.AuditEvent{})
	if query.ActorUserID != nil && *query.ActorUserID != uuid.Nil {
		db = db.Where("actor_user_id = ?", *query.ActorUserID)
	}
	if query.Action != "" {
		db = db.Where("action = ?", query.Action)
	}
	if query.ResourceType != "" {
		db = db.Where("resource_type = ?", query.ResourceType)
	}
	if query.ResourceID != "" {
		db = db.Where("resource_id = ?", query.ResourceID)
	}
	if query.Outcome != "" {
		db = db.Where("outcome = ?", query.Outcome)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func NewAuditEvent(action, resourceType, resourceID, outcome string, actorUserID *uuid.UUID, actorDeviceID string) *model.AuditEvent {
	if outcome == "" {
		outcome = "success"
	}
	return &model.AuditEvent{Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, ActorUserID: actorUserID, ActorDeviceID: actorDeviceID, OccurredAt: time.Now().UTC()}
}
