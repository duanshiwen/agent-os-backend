package repository

import (
	"strings"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type KBSearchRepo struct {
	db *gorm.DB
}

type KBSearchDocumentsQuery struct {
	Q            string
	CollectionID *uuid.UUID
	SnapshotID   *uuid.UUID
	Limit        int
	Offset       int
}

func NewKBSearchRepo(db *gorm.DB) *KBSearchRepo {
	return &KBSearchRepo{db: db}
}

func (r *KBSearchRepo) ReplaceSnapshotDocuments(snapshotID uuid.UUID, docs []model.KBSearchDocument) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("snapshot_id = ?", snapshotID).Delete(&model.KBSearchDocument{}).Error; err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}
		return tx.Create(&docs).Error
	})
}

func (r *KBSearchRepo) SearchDocuments(query KBSearchDocumentsQuery) ([]model.KBSearchDocument, error) {
	var docs []model.KBSearchDocument
	db := r.applySearchFilters(r.db.Model(&model.KBSearchDocument{}), query)
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	err := db.Order("indexed_at DESC").Limit(limit).Offset(query.Offset).Find(&docs).Error
	return docs, err
}

func (r *KBSearchRepo) CountDocuments(query KBSearchDocumentsQuery) (int64, error) {
	var count int64
	err := r.applySearchFilters(r.db.Model(&model.KBSearchDocument{}), query).Count(&count).Error
	return count, err
}

func (r *KBSearchRepo) applySearchFilters(db *gorm.DB, query KBSearchDocumentsQuery) *gorm.DB {
	db = db.Where("status = ?", "active")
	if query.CollectionID != nil && *query.CollectionID != uuid.Nil {
		db = db.Where("collection_id = ?", *query.CollectionID)
	}
	if query.SnapshotID != nil && *query.SnapshotID != uuid.Nil {
		db = db.Where("snapshot_id = ?", *query.SnapshotID)
	}
	q := strings.TrimSpace(query.Q)
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		db = db.Where("LOWER(title) LIKE ? OR LOWER(summary) LIKE ? OR LOWER(content_text) LIKE ? OR LOWER(entry_id) LIKE ?", like, like, like, like)
	}
	return db
}
