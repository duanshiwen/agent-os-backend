package repository

import (
	"strings"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	KBCollectionStatusDraft     = "draft"
	KBCollectionStatusPublished = "published"
	KBCollectionStatusArchived  = "archived"
)

type ListPublishedCollectionsQuery struct {
	Q       string
	OwnerID *uuid.UUID
	IsFree  *bool
	Limit   int
	Offset  int
}

type KBUsageCounts struct {
	ManifestDownloadCount int64
	ContentDownloadCount  int64
}

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

func (r *KBHubRepo) UpdateCollection(collection *model.KBCollection) error {
	return r.db.Save(collection).Error
}

func (r *KBHubRepo) ListPublishedCollections() ([]model.KBCollection, error) {
	return r.SearchPublishedCollections(ListPublishedCollectionsQuery{})
}

func (r *KBHubRepo) SearchPublishedCollections(query ListPublishedCollectionsQuery) ([]model.KBCollection, error) {
	var collections []model.KBCollection
	db := r.applyPublishedCollectionFilters(r.db.Model(&model.KBCollection{}), query)
	limit := normalizeLimit(query.Limit)
	if query.Offset < 0 {
		query.Offset = 0
	}
	err := db.Order("updated_at DESC").Limit(limit).Offset(query.Offset).Find(&collections).Error
	return collections, err
}

func (r *KBHubRepo) CountPublishedCollections(query ListPublishedCollectionsQuery) (int64, error) {
	var count int64
	err := r.applyPublishedCollectionFilters(r.db.Model(&model.KBCollection{}), query).Count(&count).Error
	return count, err
}

func (r *KBHubRepo) applyPublishedCollectionFilters(db *gorm.DB, query ListPublishedCollectionsQuery) *gorm.DB {
	db = db.Where("status = ?", KBCollectionStatusPublished)
	if query.OwnerID != nil && *query.OwnerID != uuid.Nil {
		db = db.Where("owner_id = ?", *query.OwnerID)
	}
	if query.IsFree != nil {
		db = db.Where("is_free = ?", *query.IsFree)
	}
	q := strings.TrimSpace(query.Q)
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		db = db.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}
	return db
}

func normalizeLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}

func (r *KBHubRepo) GetPublishedCollection(collectionID uuid.UUID) (*model.KBCollection, error) {
	var collection model.KBCollection
	err := r.db.First(&collection, "id = ? AND status = ?", collectionID, KBCollectionStatusPublished).Error
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

func (r *KBHubRepo) GetLatestSnapshot(collectionID uuid.UUID) (*model.KBSnapshot, error) {
	var snapshot model.KBSnapshot
	err := r.db.Where("collection_id = ?", collectionID).Order("version DESC").First(&snapshot).Error
	return &snapshot, err
}

func (r *KBHubRepo) GetSnapshotByVersion(collectionID uuid.UUID, version int) (*model.KBSnapshot, error) {
	var snapshot model.KBSnapshot
	err := r.db.First(&snapshot, "collection_id = ? AND version = ?", collectionID, version).Error
	return &snapshot, err
}

func (r *KBHubRepo) ListSnapshotEntries(snapshotID uuid.UUID) ([]model.KBSnapshotEntry, error) {
	var entries []model.KBSnapshotEntry
	err := r.db.Where("snapshot_id = ?", snapshotID).Order("entry_id ASC").Find(&entries).Error
	return entries, err
}

func (r *KBHubRepo) GetSnapshotEntry(snapshotID, entryRecordID uuid.UUID) (*model.KBSnapshotEntry, error) {
	var entry model.KBSnapshotEntry
	err := r.db.First(&entry, "id = ? AND snapshot_id = ?", entryRecordID, snapshotID).Error
	return &entry, err
}

func (r *KBHubRepo) MarkCollectionPublished(collectionID uuid.UUID) error {
	return r.db.Model(&model.KBCollection{}).Where("id = ?", collectionID).Update("status", KBCollectionStatusPublished).Error
}

func (r *KBHubRepo) UpsertSubscription(subscription *model.KBSubscription) error {
	var existing model.KBSubscription
	err := r.db.First(&existing, "user_id = ? AND collection_id = ?", subscription.UserID, subscription.CollectionID).Error
	if err != nil {
		if IsNotFound(err) {
			return r.db.Create(subscription).Error
		}
		return err
	}
	return r.db.Model(&existing).Updates(map[string]any{
		"snapshot_id":    subscription.SnapshotID,
		"track_mode":     subscription.TrackMode,
		"pinned_version": subscription.PinnedVersion,
		"status":         subscription.Status,
		"started_at":     subscription.StartedAt,
		"expires_at":     subscription.ExpiresAt,
	}).Error
}

func (r *KBHubRepo) GetActiveSubscription(userID, collectionID uuid.UUID) (*model.KBSubscription, error) {
	var subscription model.KBSubscription
	err := r.db.First(&subscription, "user_id = ? AND collection_id = ? AND status = ?", userID, collectionID, "active").Error
	return &subscription, err
}

func (r *KBHubRepo) ListSubscriptionsByUser(userID uuid.UUID) ([]model.KBSubscription, error) {
	var subscriptions []model.KBSubscription
	err := r.db.Where("user_id = ?", userID).Order("updated_at DESC").Find(&subscriptions).Error
	return subscriptions, err
}

func (r *KBHubRepo) CountSubscriptionsByStatus(collectionID uuid.UUID, status string) (int64, error) {
	var count int64
	err := r.db.Model(&model.KBSubscription{}).Where("collection_id = ? AND status = ?", collectionID, status).Count(&count).Error
	return count, err
}

func (r *KBHubRepo) CreateUsageRecord(record *model.KBUsageRecord) error {
	return r.db.Create(record).Error
}

func (r *KBHubRepo) CountUsageByOperations(collectionID uuid.UUID, operations []string) (int64, error) {
	var count int64
	if len(operations) == 0 {
		return 0, nil
	}
	err := r.db.Model(&model.KBUsageRecord{}).Where("collection_id = ? AND operation_type IN ?", collectionID, operations).Count(&count).Error
	return count, err
}

func (r *KBHubRepo) UsageCounts(collectionID uuid.UUID) (KBUsageCounts, error) {
	manifest, err := r.CountUsageByOperations(collectionID, KBManifestDownloadOperations())
	if err != nil {
		return KBUsageCounts{}, err
	}
	content, err := r.CountUsageByOperations(collectionID, KBContentDownloadOperations())
	if err != nil {
		return KBUsageCounts{}, err
	}
	return KBUsageCounts{ManifestDownloadCount: manifest, ContentDownloadCount: content}, nil
}

func KBManifestDownloadOperations() []string {
	return []string{"manifest_download_url.created.public", "manifest_download_url.created.installed"}
}

func KBContentDownloadOperations() []string {
	return []string{"entry_content_download_url.created.installed"}
}

func (r *KBHubRepo) CancelSubscription(userID, collectionID uuid.UUID) error {
	res := r.db.Model(&model.KBSubscription{}).
		Where("user_id = ? AND collection_id = ? AND status = ?", userID, collectionID, "active").
		Update("status", "cancelled")
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
