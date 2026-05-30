package repository

import (
	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	KBCollectionStatusDraft     = "draft"
	KBCollectionStatusPublished = "published"
	KBCollectionStatusArchived  = "archived"
)

type KBHubRepo struct {
	db *gorm.DB
}

func NewKBHubRepo(db *gorm.DB) *KBHubRepo {
	return &KBHubRepo{db: db}
}

func (r *KBHubRepo) Transaction(fn func(txRepo *KBHubRepo) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(&KBHubRepo{db: tx})
	})
}

func (r *KBHubRepo) CreateCollection(collection *model.KBCollection) error {
	return r.db.Create(collection).Error
}

func (r *KBHubRepo) ListCollectionsByOwner(ownerID uuid.UUID) ([]model.KBCollection, error) {
	var collections []model.KBCollection
	err := r.db.Where("owner_id = ?", ownerID).Order("created_at DESC").Find(&collections).Error
	return collections, err
}

func (r *KBHubRepo) GetCollectionForOwner(ownerID, collectionID uuid.UUID) (*model.KBCollection, error) {
	var collection model.KBCollection
	err := r.db.First(&collection, "id = ? AND owner_id = ?", collectionID, ownerID).Error
	return &collection, err
}

func (r *KBHubRepo) NextSnapshotVersion(collectionID uuid.UUID) (int, error) {
	var maxVersion int
	err := r.db.Model(&model.KBSnapshot{}).Where("collection_id = ?", collectionID).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error
	return maxVersion + 1, err
}

func (r *KBHubRepo) CreateSnapshot(snapshot *model.KBSnapshot, entries []model.KBSnapshotEntry) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(snapshot).Error; err != nil {
			return err
		}
		for i := range entries {
			entries[i].SnapshotID = snapshot.ID
		}
		if len(entries) > 0 {
			if err := tx.Create(&entries).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *KBHubRepo) ListSnapshots(collectionID uuid.UUID) ([]model.KBSnapshot, error) {
	var snapshots []model.KBSnapshot
	err := r.db.Where("collection_id = ?", collectionID).Order("version DESC").Find(&snapshots).Error
	return snapshots, err
}

func (r *KBHubRepo) GetSnapshot(collectionID, snapshotID uuid.UUID) (*model.KBSnapshot, error) {
	var snapshot model.KBSnapshot
	err := r.db.First(&snapshot, "id = ? AND collection_id = ?", snapshotID, collectionID).Error
	return &snapshot, err
}

func (r *KBHubRepo) ListSnapshotEntries(snapshotID uuid.UUID) ([]model.KBSnapshotEntry, error) {
	var entries []model.KBSnapshotEntry
	err := r.db.Where("snapshot_id = ?", snapshotID).Order("entry_id ASC").Find(&entries).Error
	return entries, err
}

func (r *KBHubRepo) MarkCollectionPublished(collectionID uuid.UUID) error {
	return r.db.Model(&model.KBCollection{}).Where("id = ?", collectionID).Update("status", KBCollectionStatusPublished).Error
}
