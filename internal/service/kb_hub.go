package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

var (
	ErrKBCollectionNotFound = errors.New("kb collection not found")
	ErrKBSnapshotNotFound   = errors.New("kb snapshot not found")
	ErrKBSubscriptionNeeded = errors.New("active kb subscription required")
	ErrKBInvalid            = errors.New("kb request invalid")
	ErrKBNoEntries          = errors.New("kb snapshot requires at least one active knowledge entry")
)

type KBHubService struct {
	kbRepo        *repository.KBHubRepo
	knowledgeRepo *repository.KnowledgeEntriesRepo
	objectSvc     *ObjectService
}

type CreateKBCollectionInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsFree      *bool  `json:"is_free"`
}

type PublishKBSnapshotInput struct {
	EntryIDs []string `json:"entry_ids"`
}

type InstallKBCollectionInput struct {
	TrackMode     string `json:"track_mode"`
	PinnedVersion *int   `json:"pinned_version"`
}

type KBSnapshotDetail struct {
	Snapshot *model.KBSnapshot       `json:"snapshot"`
	Entries  []model.KBSnapshotEntry `json:"entries"`
}

type KBPublicCollectionDetail struct {
	Collection     *model.KBCollection `json:"collection"`
	LatestSnapshot *model.KBSnapshot   `json:"latest_snapshot"`
}

type kbManifest struct {
	SchemaVersion int               `json:"schema_version"`
	CollectionID  string            `json:"collection_id"`
	SnapshotID    string            `json:"snapshot_id"`
	Version       int               `json:"version"`
	PublishedAt   time.Time         `json:"published_at"`
	EntryCount    int               `json:"entry_count"`
	TotalTokens   int               `json:"total_tokens"`
	Entries       []kbManifestEntry `json:"entries"`
}

type kbManifestEntry struct {
	EntryID          string         `json:"entry_id"`
	Title            string         `json:"title"`
	Summary          string         `json:"summary"`
	Tags             []string       `json:"tags"`
	Metadata         map[string]any `json:"metadata"`
	Version          uint64         `json:"version"`
	ContentHash      string         `json:"content_hash"`
	ContentObjectURI string         `json:"content_object_uri"`
	Tokens           int            `json:"tokens"`
	SourceURI        string         `json:"source_uri"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func NewKBHubService(kbRepo *repository.KBHubRepo, knowledgeRepo *repository.KnowledgeEntriesRepo, objectSvc *ObjectService) *KBHubService {
	return &KBHubService{kbRepo: kbRepo, knowledgeRepo: knowledgeRepo, objectSvc: objectSvc}
}

func (s *KBHubService) CreateCollection(ownerID uuid.UUID, input CreateKBCollectionInput) (*model.KBCollection, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrKBInvalid)
	}
	isFree := true
	if input.IsFree != nil {
		isFree = *input.IsFree
	}
	collection := &model.KBCollection{
		OwnerID:     ownerID,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Status:      repository.KBCollectionStatusDraft,
		IsFree:      isFree,
	}
	if err := s.kbRepo.CreateCollection(collection); err != nil {
		return nil, err
	}
	return collection, nil
}

func (s *KBHubService) ListCollections(ownerID uuid.UUID) ([]model.KBCollection, error) {
	return s.kbRepo.ListCollectionsByOwner(ownerID)
}

func (s *KBHubService) ListPublicCollections() ([]model.KBCollection, error) {
	return s.kbRepo.ListPublishedCollections()
}

func (s *KBHubService) GetPublicCollection(collectionID uuid.UUID) (*KBPublicCollectionDetail, error) {
	collection, err := s.kbRepo.GetPublishedCollection(collectionID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBCollectionNotFound
		}
		return nil, err
	}
	latest, err := s.kbRepo.GetLatestSnapshot(collectionID)
	if err != nil {
		if repository.IsNotFound(err) {
			latest = nil
		} else {
			return nil, err
		}
	}
	return &KBPublicCollectionDetail{Collection: collection, LatestSnapshot: latest}, nil
}

func (s *KBHubService) GetCollection(ownerID, collectionID uuid.UUID) (*model.KBCollection, error) {
	collection, err := s.kbRepo.GetCollectionForOwner(ownerID, collectionID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBCollectionNotFound
		}
		return nil, err
	}
	return collection, nil
}

func (s *KBHubService) PublishSnapshot(ctx context.Context, ownerID, collectionID uuid.UUID, input PublishKBSnapshotInput) (*KBSnapshotDetail, error) {
	collection, err := s.GetCollection(ownerID, collectionID)
	if err != nil {
		return nil, err
	}
	entries, err := s.knowledgeRepo.ListByUser(ownerID, false)
	if err != nil {
		return nil, err
	}
	entries = filterKnowledgeEntries(entries, input.EntryIDs)
	if len(entries) == 0 {
		return nil, ErrKBNoEntries
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].EntryID < entries[j].EntryID })

	version, err := s.kbRepo.NextSnapshotVersion(collectionID)
	if err != nil {
		return nil, err
	}
	publishedAt := time.Now().UTC()
	snapshotID := uuid.New()
	manifestEntries := make([]kbManifestEntry, 0, len(entries))
	snapshotEntries := make([]model.KBSnapshotEntry, 0, len(entries))
	totalTokens := 0
	contentSize := int64(0)

	for _, entry := range entries {
		content := []byte(entry.ContentMarkdown)
		contentObj, err := s.objectSvc.StoreObject(ctx, ownerID, StoreObjectInput{
			Scope:       fmt.Sprintf("kb/snapshots/%s/v%d/entries", collectionID.String(), version),
			Filename:    safeEntryFilename(entry.EntryID),
			ContentType: "text/markdown; charset=utf-8",
			Content:     content,
		})
		if err != nil {
			return nil, err
		}
		tokens := estimateTokens(entry.ContentMarkdown)
		totalTokens += tokens
		contentSize += int64(len(content))
		manifestEntry := kbManifestEntry{
			EntryID:          entry.EntryID,
			Title:            entry.Title,
			Summary:          entry.Summary,
			Tags:             []string(entry.Tags),
			Metadata:         jsonMapToMap(entry.Metadata),
			Version:          entry.Version,
			ContentHash:      entry.ContentHash,
			ContentObjectURI: contentObj.ObjectURI,
			Tokens:           tokens,
			SourceURI:        entry.SourceURI,
			UpdatedAt:        entry.UpdatedAt,
		}
		manifestEntries = append(manifestEntries, manifestEntry)
		snapshotEntries = append(snapshotEntries, model.KBSnapshotEntry{
			EntryID:          entry.EntryID,
			Title:            entry.Title,
			Summary:          entry.Summary,
			Tags:             entry.Tags,
			Metadata:         entry.Metadata,
			ContentObjectURI: contentObj.ObjectURI,
			Tokens:           tokens,
		})
	}

	manifest := kbManifest{
		SchemaVersion: 1,
		CollectionID:  collection.ID.String(),
		SnapshotID:    snapshotID.String(),
		Version:       version,
		PublishedAt:   publishedAt,
		EntryCount:    len(entries),
		TotalTokens:   totalTokens,
		Entries:       manifestEntries,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	manifestObj, err := s.objectSvc.StoreObject(ctx, ownerID, StoreObjectInput{
		Scope:       fmt.Sprintf("kb/snapshots/%s/v%d", collectionID.String(), version),
		Filename:    "manifest.json",
		ContentType: "application/json",
		Content:     manifestBytes,
	})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(manifestBytes)
	checksum := hex.EncodeToString(sum[:])
	snapshot := &model.KBSnapshot{
		Base:              model.Base{ID: snapshotID},
		CollectionID:      collectionID,
		Version:           version,
		EntryCount:        len(entries),
		TotalTokens:       totalTokens,
		PublishedAt:       publishedAt,
		Checksum:          checksum,
		ManifestObjectURI: manifestObj.ObjectURI,
		ContentHash:       checksum,
		ContentSize:       contentSize + int64(len(manifestBytes)),
	}
	if err := s.kbRepo.CreateSnapshot(snapshot, snapshotEntries); err != nil {
		return nil, err
	}
	if err := s.kbRepo.MarkCollectionPublished(collectionID); err != nil {
		return nil, err
	}
	createdEntries, err := s.kbRepo.ListSnapshotEntries(snapshot.ID)
	if err != nil {
		return nil, err
	}
	return &KBSnapshotDetail{Snapshot: snapshot, Entries: createdEntries}, nil
}

func (s *KBHubService) ListSnapshots(ownerID, collectionID uuid.UUID) ([]model.KBSnapshot, error) {
	if _, err := s.GetCollection(ownerID, collectionID); err != nil {
		return nil, err
	}
	return s.kbRepo.ListSnapshots(collectionID)
}

func (s *KBHubService) GetSnapshot(ownerID, collectionID, snapshotID uuid.UUID) (*KBSnapshotDetail, error) {
	if _, err := s.GetCollection(ownerID, collectionID); err != nil {
		return nil, err
	}
	return s.getSnapshotDetail(collectionID, snapshotID)
}

func (s *KBHubService) GetPublicSnapshot(collectionID, snapshotID uuid.UUID) (*KBSnapshotDetail, error) {
	if _, err := s.GetPublicCollection(collectionID); err != nil {
		return nil, err
	}
	return s.getSnapshotDetail(collectionID, snapshotID)
}

func (s *KBHubService) CreateSnapshotManifestDownloadURL(ctx context.Context, collectionID, snapshotID uuid.UUID) (*DownloadURLResponse, error) {
	if _, err := s.GetPublicCollection(collectionID); err != nil {
		return nil, err
	}
	snapshot, err := s.kbRepo.GetSnapshot(collectionID, snapshotID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	return s.objectSvc.CreateDownloadURLByObjectURI(ctx, snapshot.ManifestObjectURI, CreateDownloadURLInput{Disposition: "attachment"})
}

func (s *KBHubService) InstallCollection(userID, collectionID uuid.UUID, input InstallKBCollectionInput) (*model.KBSubscription, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user is required", ErrKBInvalid)
	}
	if _, err := s.GetPublicCollection(collectionID); err != nil {
		return nil, err
	}
	trackMode := strings.TrimSpace(input.TrackMode)
	if trackMode == "" {
		trackMode = "latest"
	}
	if trackMode != "latest" && trackMode != "pinned" {
		return nil, fmt.Errorf("%w: track_mode must be latest or pinned", ErrKBInvalid)
	}
	var snapshot *model.KBSnapshot
	var err error
	var pinnedVersion *int
	if trackMode == "pinned" {
		if input.PinnedVersion == nil || *input.PinnedVersion <= 0 {
			return nil, fmt.Errorf("%w: pinned_version is required for pinned track mode", ErrKBInvalid)
		}
		version := *input.PinnedVersion
		pinnedVersion = &version
		snapshot, err = s.kbRepo.GetSnapshotByVersion(collectionID, version)
	} else {
		snapshot, err = s.kbRepo.GetLatestSnapshot(collectionID)
	}
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	now := time.Now().UTC()
	subscription := &model.KBSubscription{
		UserID:        userID,
		CollectionID:  collectionID,
		SnapshotID:    snapshot.ID,
		TrackMode:     trackMode,
		PinnedVersion: pinnedVersion,
		Status:        "active",
		StartedAt:     now,
	}
	if err := s.kbRepo.UpsertSubscription(subscription); err != nil {
		return nil, err
	}
	return subscription, nil
}

func (s *KBHubService) ListSubscriptions(userID uuid.UUID) ([]model.KBSubscription, error) {
	return s.kbRepo.ListSubscriptionsByUser(userID)
}

func (s *KBHubService) CancelSubscription(userID, collectionID uuid.UUID) (*model.KBSubscription, error) {
	if err := s.kbRepo.CancelSubscription(userID, collectionID); err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSubscriptionNeeded
		}
		return nil, err
	}
	subs, err := s.kbRepo.ListSubscriptionsByUser(userID)
	if err != nil {
		return nil, err
	}
	for i := range subs {
		if subs[i].CollectionID == collectionID {
			return &subs[i], nil
		}
	}
	return nil, ErrKBSubscriptionNeeded
}

func (s *KBHubService) CreateInstalledManifestDownloadURL(ctx context.Context, userID, collectionID, snapshotID uuid.UUID) (*DownloadURLResponse, error) {
	if _, err := s.authorizeSnapshotAccess(userID, collectionID, snapshotID); err != nil {
		return nil, err
	}
	snapshot, err := s.kbRepo.GetSnapshot(collectionID, snapshotID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	return s.objectSvc.CreateDownloadURLByObjectURI(ctx, snapshot.ManifestObjectURI, CreateDownloadURLInput{Disposition: "attachment"})
}

func (s *KBHubService) CreateInstalledEntryContentDownloadURL(ctx context.Context, userID, collectionID, snapshotID, entryRecordID uuid.UUID) (*DownloadURLResponse, error) {
	if _, err := s.authorizeSnapshotAccess(userID, collectionID, snapshotID); err != nil {
		return nil, err
	}
	entry, err := s.kbRepo.GetSnapshotEntry(snapshotID, entryRecordID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	return s.objectSvc.CreateDownloadURLByObjectURI(ctx, entry.ContentObjectURI, CreateDownloadURLInput{Disposition: "attachment"})
}

func (s *KBHubService) authorizeSnapshotAccess(userID, collectionID, snapshotID uuid.UUID) (*model.KBSubscription, error) {
	if userID == uuid.Nil {
		return nil, ErrKBSubscriptionNeeded
	}
	snapshot, err := s.kbRepo.GetSnapshot(collectionID, snapshotID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	subscription, err := s.kbRepo.GetActiveSubscription(userID, collectionID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSubscriptionNeeded
		}
		return nil, err
	}
	if subscription.TrackMode == "pinned" {
		if subscription.PinnedVersion == nil || *subscription.PinnedVersion != snapshot.Version {
			return nil, ErrKBSubscriptionNeeded
		}
		return subscription, nil
	}
	if subscription.TrackMode == "latest" {
		latest, err := s.kbRepo.GetLatestSnapshot(collectionID)
		if err != nil {
			if repository.IsNotFound(err) {
				return nil, ErrKBSnapshotNotFound
			}
			return nil, err
		}
		if latest.ID != snapshotID {
			return nil, ErrKBSubscriptionNeeded
		}
		return subscription, nil
	}
	return nil, ErrKBSubscriptionNeeded
}

func (s *KBHubService) getSnapshotDetail(collectionID, snapshotID uuid.UUID) (*KBSnapshotDetail, error) {
	snapshot, err := s.kbRepo.GetSnapshot(collectionID, snapshotID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	entries, err := s.kbRepo.ListSnapshotEntries(snapshot.ID)
	if err != nil {
		return nil, err
	}
	return &KBSnapshotDetail{Snapshot: snapshot, Entries: entries}, nil
}

func filterKnowledgeEntries(entries []model.UserKnowledgeEntry, entryIDs []string) []model.UserKnowledgeEntry {
	if len(entryIDs) == 0 {
		return entries
	}
	wanted := make(map[string]bool, len(entryIDs))
	for _, entryID := range entryIDs {
		entryID = strings.TrimSpace(entryID)
		if entryID != "" {
			wanted[entryID] = true
		}
	}
	out := make([]model.UserKnowledgeEntry, 0, len(entries))
	for _, entry := range entries {
		if wanted[entry.EntryID] {
			out = append(out, entry)
		}
	}
	return out
}

func estimateTokens(content string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}
	runes := utf8.RuneCountInString(content)
	return (runes + 3) / 4
}

func safeEntryFilename(entryID string) string {
	entryID = strings.TrimSpace(entryID)
	entryID = strings.Trim(entryID, "/")
	entryID = strings.ReplaceAll(entryID, "\\", "/")
	entryID = strings.ReplaceAll(entryID, "/", "__")
	if entryID == "" || entryID == "." || entryID == ".." {
		entryID = "entry"
	}
	if !strings.HasSuffix(entryID, ".md") {
		entryID += ".md"
	}
	return entryID
}

func jsonMapToMap(in datatypes.JSONMap) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
