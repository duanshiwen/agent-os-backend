package service

import (
	"crypto/sha256"
	"encoding/hex"
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

var (
	ErrKnowledgeEntryNotFound = errors.New("knowledge entry not found")
	ErrKnowledgeEntryDeleted  = errors.New("knowledge entry is deleted")
	ErrKnowledgeEntryConflict = errors.New("knowledge entry conflict")
)

type KnowledgeEntriesService struct {
	repo    *repository.KnowledgeEntriesRepo
	syncSvc *SyncService
}

type KnowledgeEntryInput struct {
	EntryID         string                      `json:"entry_id"`
	Title           string                      `json:"title"`
	ContentMarkdown string                      `json:"content_markdown"`
	Summary         string                      `json:"summary"`
	Tags            datatypes.JSONSlice[string] `json:"tags"`
	Metadata        datatypes.JSONMap           `json:"metadata"`
	SourceURI       string                      `json:"source_uri"`
	ClientEventID   string                      `json:"client_event_id"`
}

func NewKnowledgeEntriesService(repo *repository.KnowledgeEntriesRepo, syncSvc *SyncService) *KnowledgeEntriesService {
	return &KnowledgeEntriesService{repo: repo, syncSvc: syncSvc}
}

func (s *KnowledgeEntriesService) ListEntries(userID uuid.UUID, includeDeleted bool) ([]model.UserKnowledgeEntry, error) {
	return s.repo.ListByUser(userID, includeDeleted)
}

func (s *KnowledgeEntriesService) GetEntry(userID uuid.UUID, entryID string, includeDeleted bool) (*model.UserKnowledgeEntry, error) {
	entry, err := s.repo.GetByEntryID(userID, strings.TrimSpace(entryID), includeDeleted)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKnowledgeEntryNotFound
		}
		return nil, err
	}
	return entry, nil
}

func (s *KnowledgeEntriesService) CreateEntry(userID uuid.UUID, sourceDeviceID string, input KnowledgeEntryInput) (*model.UserKnowledgeEntry, *model.SyncEvent, error) {
	if err := validateKnowledgeInput(input, true); err != nil {
		return nil, nil, err
	}
	entryID := strings.TrimSpace(input.EntryID)
	if existingEvent, entry, err := s.idempotentKnowledgeReplay(userID, sourceDeviceID, entryID, SyncOperationCreated, input.ClientEventID); existingEvent != nil || err != nil {
		return entry, existingEvent, err
	}
	metadata := input.Metadata
	if metadata == nil {
		metadata = datatypes.JSONMap{}
	}
	entry := &model.UserKnowledgeEntry{
		UserID:            userID,
		EntryID:           entryID,
		Title:             strings.TrimSpace(input.Title),
		ContentMarkdown:   input.ContentMarkdown,
		Summary:           input.Summary,
		Tags:              input.Tags,
		Metadata:          metadata,
		SourceURI:         input.SourceURI,
		Status:            repository.KnowledgeEntryStatusActive,
		Version:           1,
		ContentHash:       knowledgeContentHash(input.ContentMarkdown),
		UpdatedByDeviceID: sourceDeviceID,
	}
	if err := s.repo.Create(entry); err != nil {
		return nil, nil, fmt.Errorf("create knowledge entry: %w", err)
	}
	persisted, err := s.repo.GetByEntryID(userID, entryID, true)
	if err != nil {
		return nil, nil, fmt.Errorf("get created knowledge entry: %w", err)
	}
	event, err := s.recordKnowledgeEvent(userID, sourceDeviceID, persisted, SyncOperationCreated, input.ClientEventID)
	if err != nil {
		return nil, nil, err
	}
	return persisted, event, nil
}

func (s *KnowledgeEntriesService) UpdateEntry(userID uuid.UUID, sourceDeviceID, entryID string, input KnowledgeEntryInput) (*model.UserKnowledgeEntry, *model.SyncEvent, error) {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return nil, nil, fmt.Errorf("entry_id is required")
	}
	if err := validateKnowledgeInput(input, false); err != nil {
		return nil, nil, err
	}
	existing, err := s.repo.GetByEntryID(userID, entryID, true)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, nil, ErrKnowledgeEntryNotFound
		}
		return nil, nil, err
	}
	if existing.Status == repository.KnowledgeEntryStatusDeleted {
		return nil, nil, ErrKnowledgeEntryDeleted
	}
	metadata := input.Metadata
	if metadata == nil {
		metadata = datatypes.JSONMap{}
	}
	updated := &model.UserKnowledgeEntry{
		UserID:            userID,
		EntryID:           entryID,
		Title:             strings.TrimSpace(input.Title),
		ContentMarkdown:   input.ContentMarkdown,
		Summary:           input.Summary,
		Tags:              input.Tags,
		Metadata:          metadata,
		SourceURI:         input.SourceURI,
		ContentHash:       knowledgeContentHash(input.ContentMarkdown),
		UpdatedByDeviceID: sourceDeviceID,
	}
	if err := s.repo.UpdateActive(updated); err != nil {
		return nil, nil, fmt.Errorf("update knowledge entry: %w", err)
	}
	persisted, err := s.repo.GetByEntryID(userID, entryID, true)
	if err != nil {
		return nil, nil, err
	}
	event, err := s.recordKnowledgeEvent(userID, sourceDeviceID, persisted, SyncOperationUpdated, input.ClientEventID)
	if err != nil {
		return nil, nil, err
	}
	return persisted, event, nil
}

func (s *KnowledgeEntriesService) DeleteEntry(userID uuid.UUID, sourceDeviceID, entryID, clientEventID string) (*model.UserKnowledgeEntry, *model.SyncEvent, error) {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return nil, nil, fmt.Errorf("entry_id is required")
	}
	existing, err := s.repo.GetByEntryID(userID, entryID, true)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, nil, ErrKnowledgeEntryNotFound
		}
		return nil, nil, err
	}
	if existing.Status == repository.KnowledgeEntryStatusDeleted {
		return nil, nil, ErrKnowledgeEntryDeleted
	}
	deletedAt := time.Now()
	if err := s.repo.Tombstone(userID, entryID, sourceDeviceID, knowledgeContentHash(existing.ContentMarkdown), deletedAt); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrKnowledgeEntryNotFound
		}
		return nil, nil, err
	}
	persisted, err := s.repo.GetByEntryID(userID, entryID, true)
	if err != nil {
		return nil, nil, err
	}
	event, err := s.recordKnowledgeEvent(userID, sourceDeviceID, persisted, SyncOperationDeleted, clientEventID)
	if err != nil {
		return nil, nil, err
	}
	return persisted, event, nil
}

func (s *KnowledgeEntriesService) recordKnowledgeEvent(userID uuid.UUID, sourceDeviceID string, entry *model.UserKnowledgeEntry, operation, clientEventID string) (*model.SyncEvent, error) {
	if s.syncSvc == nil {
		return nil, nil
	}
	payload := knowledgeEntryPayload(entry)
	return s.syncSvc.RecordEnvelope(SyncEnvelope{
		UserID:         userID,
		SourceDeviceID: sourceDeviceID,
		ObjectType:     SyncObjectKnowledge,
		ObjectID:       entry.EntryID,
		Operation:      operation,
		ClientEventID:  clientEventID,
		Payload:        payload,
	})
}

func (s *KnowledgeEntriesService) idempotentKnowledgeReplay(userID uuid.UUID, sourceDeviceID, entryID, operation, clientEventID string) (*model.SyncEvent, *model.UserKnowledgeEntry, error) {
	if s.syncSvc == nil || clientEventID == "" {
		return nil, nil, nil
	}
	existing, err := s.syncSvc.syncRepo.GetEventByClientEventID(userID, clientEventID)
	if err != nil {
		return nil, nil, nil
	}
	if existing.ObjectType != SyncObjectKnowledge || existing.ObjectID != entryID || existing.Operation != operation || existing.SourceDeviceID != sourceDeviceID {
		return nil, nil, ErrSyncIdempotencyConflict
	}
	entry, err := s.repo.GetByEntryID(userID, entryID, true)
	if err != nil {
		if repository.IsNotFound(err) {
			return existing, nil, ErrKnowledgeEntryNotFound
		}
		return existing, nil, err
	}
	return existing, entry, nil
}

func knowledgeEntryPayload(entry *model.UserKnowledgeEntry) datatypes.JSONMap {
	payload := datatypes.JSONMap{
		"object_id":            entry.EntryID,
		"entry_id":             entry.EntryID,
		"title":                entry.Title,
		"content_markdown":     entry.ContentMarkdown,
		"summary":              entry.Summary,
		"tags":                 entry.Tags,
		"metadata":             entry.Metadata,
		"source_uri":           entry.SourceURI,
		"status":               entry.Status,
		"version":              entry.Version,
		"content_hash":         entry.ContentHash,
		"updated_by_device_id": entry.UpdatedByDeviceID,
		"updated_at":           entry.UpdatedAt,
	}
	if entry.DeletedAt != nil {
		payload["deleted_at"] = entry.DeletedAt
	}
	return payload
}

func validateKnowledgeInput(input KnowledgeEntryInput, requireEntryID bool) error {
	if requireEntryID && strings.TrimSpace(input.EntryID) == "" {
		return fmt.Errorf("entry_id is required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.ContentMarkdown) == "" {
		return fmt.Errorf("content_markdown is required")
	}
	return nil
}

func knowledgeContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
