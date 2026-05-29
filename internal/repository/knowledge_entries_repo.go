package repository

import (
	"errors"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	KnowledgeEntryStatusActive  = "active"
	KnowledgeEntryStatusDeleted = "deleted"
)

type KnowledgeEntriesRepo struct {
	db *gorm.DB
}

func NewKnowledgeEntriesRepo(db *gorm.DB) *KnowledgeEntriesRepo {
	return &KnowledgeEntriesRepo{db: db}
}

func (r *KnowledgeEntriesRepo) Transaction(fn func(tx *gorm.DB, repo *KnowledgeEntriesRepo) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(tx, &KnowledgeEntriesRepo{db: tx})
	})
}

func (r *KnowledgeEntriesRepo) ListByUser(userID uuid.UUID, includeDeleted bool) ([]model.UserKnowledgeEntry, error) {
	var entries []model.UserKnowledgeEntry
	q := r.db.Where("user_id = ?", userID)
	if !includeDeleted {
		q = q.Where("status = ?", KnowledgeEntryStatusActive)
	}
	err := q.Order("entry_id ASC").Find(&entries).Error
	return entries, err
}

func (r *KnowledgeEntriesRepo) GetByEntryID(userID uuid.UUID, entryID string, includeDeleted bool) (*model.UserKnowledgeEntry, error) {
	var entry model.UserKnowledgeEntry
	q := r.db.Where("user_id = ? AND entry_id = ?", userID, entryID)
	if !includeDeleted {
		q = q.Where("status = ?", KnowledgeEntryStatusActive)
	}
	err := q.First(&entry).Error
	return &entry, err
}

func (r *KnowledgeEntriesRepo) Create(entry *model.UserKnowledgeEntry) error {
	return r.db.Create(entry).Error
}

func (r *KnowledgeEntriesRepo) UpdateActive(entry *model.UserKnowledgeEntry) error {
	res := r.db.Model(&model.UserKnowledgeEntry{}).
		Where("user_id = ? AND entry_id = ? AND status = ?", entry.UserID, entry.EntryID, KnowledgeEntryStatusActive).
		Updates(map[string]any{
			"title":                entry.Title,
			"content_markdown":     entry.ContentMarkdown,
			"summary":              entry.Summary,
			"tags":                 entry.Tags,
			"metadata":             entry.Metadata,
			"source_uri":           entry.SourceURI,
			"version":              gorm.Expr("version + 1"),
			"content_hash":         entry.ContentHash,
			"updated_by_device_id": entry.UpdatedByDeviceID,
			"updated_at":           time.Now(),
		}).Error
	if res != nil {
		return res
	}
	return nil
}

func (r *KnowledgeEntriesRepo) Tombstone(userID uuid.UUID, entryID, sourceDeviceID, contentHash string, deletedAt time.Time) error {
	res := r.db.Model(&model.UserKnowledgeEntry{}).
		Where("user_id = ? AND entry_id = ? AND status = ?", userID, entryID, KnowledgeEntryStatusActive).
		Updates(map[string]any{
			"status":               KnowledgeEntryStatusDeleted,
			"version":              gorm.Expr("version + 1"),
			"content_hash":         contentHash,
			"deleted_at":           deletedAt,
			"updated_by_device_id": sourceDeviceID,
			"updated_at":           deletedAt,
		}).RowsAffected
	if res == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
