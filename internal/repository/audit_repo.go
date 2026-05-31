package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

func (r *AuditRepo) AppendHashChained(event *model.AuditEvent, prepare func(previous *model.AuditEvent, nextSequence int64) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var previous model.AuditEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("sequence DESC").First(&previous).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		var previousPtr *model.AuditEvent
		nextSequence := int64(1)
		if err == nil {
			previousPtr = &previous
			nextSequence = previous.Sequence + 1
		}
		if err := prepare(previousPtr, nextSequence); err != nil {
			return err
		}
		return tx.Create(event).Error
	})
}

func (r *AuditRepo) ListHashChain(limit int) ([]model.AuditEvent, error) {
	db := r.db.Model(&model.AuditEvent{}).Order("sequence ASC")
	if limit > 0 {
		db = db.Limit(limit)
	}
	var events []model.AuditEvent
	err := db.Find(&events).Error
	return events, err
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
