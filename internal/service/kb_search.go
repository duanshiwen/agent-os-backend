package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

var ErrKBSemanticSearchUnavailable = errors.New("kb semantic search unavailable")

type KBSearchMode string

const (
	KBSearchModeMetadata KBSearchMode = "metadata"
	KBSearchModeLexical  KBSearchMode = "lexical"
	KBSearchModeSemantic KBSearchMode = "semantic"
	KBSearchModeHybrid   KBSearchMode = "hybrid"
)

type KBSearchService struct {
	searchRepo *repository.KBSearchRepo
	kbRepo     *repository.KBHubRepo
	billingSvc *KBBillingService
}

type KBSearchInput struct {
	UserID       uuid.UUID
	Q            string
	Mode         string
	CollectionID *uuid.UUID
	SnapshotID   *uuid.UUID
	Limit        int
	Offset       int
}

type KBSearchResult struct {
	Mode                      string               `json:"mode"`
	SemanticAvailable         bool                 `json:"semantic_available"`
	SemanticUnavailableReason string               `json:"semantic_unavailable_reason,omitempty"`
	Items                     []KBSearchResultItem `json:"items"`
	Limit                     int                  `json:"limit"`
	Offset                    int                  `json:"offset"`
	Total                     int64                `json:"total"`
}

type KBSearchResultItem struct {
	CollectionID    uuid.UUID `json:"collection_id"`
	SnapshotID      uuid.UUID `json:"snapshot_id"`
	SnapshotEntryID uuid.UUID `json:"snapshot_entry_id"`
	EntryID         string    `json:"entry_id"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	Tags            []string  `json:"tags"`
	Tokens          int       `json:"tokens"`
	Score           float64   `json:"score"`
}

func NewKBSearchService(searchRepo *repository.KBSearchRepo, kbRepo *repository.KBHubRepo, billingSvc *KBBillingService) *KBSearchService {
	return &KBSearchService{searchRepo: searchRepo, kbRepo: kbRepo, billingSvc: billingSvc}
}

func (s *KBSearchService) IndexSnapshot(ctx context.Context, collection *model.KBCollection, snapshot *model.KBSnapshot, entries []model.KBSnapshotEntry) error {
	_ = ctx
	if collection == nil || snapshot == nil {
		return fmt.Errorf("%w: collection and snapshot are required", ErrKBInvalid)
	}
	now := time.Now().UTC()
	docs := make([]model.KBSearchDocument, 0, len(entries))
	for _, entry := range entries {
		docs = append(docs, model.KBSearchDocument{
			CollectionID:    collection.ID,
			SnapshotID:      snapshot.ID,
			SnapshotEntryID: entry.ID,
			EntryID:         entry.EntryID,
			Title:           entry.Title,
			Summary:         entry.Summary,
			Tags:            entry.Tags,
			Metadata:        entry.Metadata,
			ContentText:     strings.Join([]string{entry.Title, entry.Summary, entry.EntryID}, "\n"),
			ContentHash:     snapshot.ContentHash,
			Tokens:          entry.Tokens,
			Status:          "active",
			IndexedAt:       now,
		})
	}
	return s.searchRepo.ReplaceSnapshotDocuments(snapshot.ID, docs)
}

func (s *KBSearchService) Search(ctx context.Context, input KBSearchInput) (*KBSearchResult, error) {
	_ = ctx
	mode := normalizeSearchMode(input.Mode)
	if mode == KBSearchModeSemantic {
		return nil, ErrKBSemanticSearchUnavailable
	}
	if mode == KBSearchModeMetadata {
		return s.searchMetadata(input)
	}
	result, err := s.searchLexical(input, mode == KBSearchModeHybrid)
	if err != nil {
		return nil, err
	}
	if s.billingSvc != nil && input.UserID != uuid.Nil && len(result.Items) > 0 {
		_, _, _, _ = s.billingSvc.RecordUsage(RecordKBUsageInput{
			UserID:        input.UserID,
			CollectionID:  firstCollectionID(result.Items),
			SnapshotID:    firstSnapshotID(result.Items),
			OperationType: KBOperationSearchLexical,
			TokensUsed:    0,
			IsFree:        true,
		})
	}
	return result, nil
}

func (s *KBSearchService) searchMetadata(input KBSearchInput) (*KBSearchResult, error) {
	query := repository.ListPublishedCollectionsQuery{Q: input.Q, Limit: input.Limit, Offset: input.Offset}
	collections, err := s.kbRepo.SearchPublishedCollections(query)
	if err != nil {
		return nil, err
	}
	total, err := s.kbRepo.CountPublishedCollections(query)
	if err != nil {
		return nil, err
	}
	items := make([]KBSearchResultItem, 0, len(collections))
	for _, collection := range collections {
		latest, err := s.kbRepo.GetLatestSnapshot(collection.ID)
		if err != nil {
			if repository.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		items = append(items, KBSearchResultItem{CollectionID: collection.ID, SnapshotID: latest.ID, Title: collection.Name, Summary: collection.Description, Tokens: latest.TotalTokens, Score: 1})
	}
	limit := normalizeAPILimit(input.Limit)
	return &KBSearchResult{Mode: string(KBSearchModeMetadata), SemanticAvailable: false, Items: items, Limit: limit, Offset: normalizeOffset(input.Offset), Total: total}, nil
}

func (s *KBSearchService) searchLexical(input KBSearchInput, hybrid bool) (*KBSearchResult, error) {
	query := repository.KBSearchDocumentsQuery{Q: input.Q, CollectionID: input.CollectionID, SnapshotID: input.SnapshotID, Limit: input.Limit, Offset: input.Offset}
	docs, err := s.searchRepo.SearchDocuments(query)
	if err != nil {
		return nil, err
	}
	total, err := s.searchRepo.CountDocuments(query)
	if err != nil {
		return nil, err
	}
	items := make([]KBSearchResultItem, 0, len(docs))
	for _, doc := range docs {
		items = append(items, KBSearchResultItem{CollectionID: doc.CollectionID, SnapshotID: doc.SnapshotID, SnapshotEntryID: doc.SnapshotEntryID, EntryID: doc.EntryID, Title: doc.Title, Summary: doc.Summary, Tags: []string(doc.Tags), Tokens: doc.Tokens, Score: lexicalScore(input.Q, doc)})
	}
	mode := KBSearchModeLexical
	semanticReason := ""
	if hybrid {
		mode = KBSearchModeHybrid
		semanticReason = ErrKBSemanticSearchUnavailable.Error()
	}
	return &KBSearchResult{Mode: string(mode), SemanticAvailable: false, SemanticUnavailableReason: semanticReason, Items: items, Limit: normalizeAPILimit(input.Limit), Offset: normalizeOffset(input.Offset), Total: total}, nil
}

func normalizeSearchMode(mode string) KBSearchMode {
	switch KBSearchMode(strings.ToLower(strings.TrimSpace(mode))) {
	case KBSearchModeMetadata:
		return KBSearchModeMetadata
	case KBSearchModeSemantic:
		return KBSearchModeSemantic
	case KBSearchModeHybrid:
		return KBSearchModeHybrid
	case KBSearchModeLexical:
		return KBSearchModeLexical
	default:
		return KBSearchModeLexical
	}
}

func normalizeAPILimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func lexicalScore(q string, doc model.KBSearchDocument) float64 {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return 1
	}
	score := 0.0
	if strings.Contains(strings.ToLower(doc.Title), q) {
		score += 3
	}
	if strings.Contains(strings.ToLower(doc.Summary), q) {
		score += 2
	}
	if strings.Contains(strings.ToLower(doc.ContentText), q) {
		score += 1
	}
	return score
}

func firstCollectionID(items []KBSearchResultItem) uuid.UUID {
	if len(items) == 0 {
		return uuid.Nil
	}
	return items[0].CollectionID
}

func firstSnapshotID(items []KBSearchResultItem) uuid.UUID {
	if len(items) == 0 {
		return uuid.Nil
	}
	return items[0].SnapshotID
}
