package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	searchRepo        *repository.KBSearchRepo
	embeddingRepo     *repository.KBEmbeddingRepo
	embeddingProvider EmbeddingProvider
	kbRepo            *repository.KBHubRepo
	billingSvc        *KBBillingService
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
	EmbeddingCoverage         *float64             `json:"embedding_coverage,omitempty"`
	Limit                     int                  `json:"limit"`
	Offset                    int                  `json:"offset"`
	Total                     int64                `json:"total"`
}

type KBEmbeddingStatus struct {
	SnapshotID                uuid.UUID                       `json:"snapshot_id"`
	Provider                  string                          `json:"provider"`
	Model                     string                          `json:"model"`
	Dimensions                int                             `json:"dimensions"`
	ProviderHealth            *EmbeddingProviderHealth        `json:"provider_health,omitempty"`
	ProviderUnavailableReason string                          `json:"provider_unavailable_reason,omitempty"`
	Coverage                  *repository.KBEmbeddingCoverage `json:"coverage,omitempty"`
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
	LexicalScore    *float64  `json:"lexical_score,omitempty"`
	SemanticScore   *float64  `json:"semantic_score,omitempty"`
}

func NewKBSearchService(searchRepo *repository.KBSearchRepo, kbRepo *repository.KBHubRepo, billingSvc *KBBillingService) *KBSearchService {
	return &KBSearchService{searchRepo: searchRepo, kbRepo: kbRepo, billingSvc: billingSvc, embeddingProvider: DisabledEmbeddingProvider{model: DefaultEmbeddingModel, dimensions: DefaultEmbeddingDimensions}}
}

func (s *KBSearchService) SetEmbedding(repo *repository.KBEmbeddingRepo, provider EmbeddingProvider) {
	s.embeddingRepo = repo
	if provider != nil {
		s.embeddingProvider = provider
	}
}

type RetryKBEmbeddingJobsResult struct {
	SnapshotID uuid.UUID `json:"snapshot_id"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	Retried    int64     `json:"retried"`
}

func (s *KBSearchService) RetryFailedEmbeddingJobs(snapshotID uuid.UUID) (*RetryKBEmbeddingJobsResult, error) {
	if snapshotID == uuid.Nil {
		return nil, fmt.Errorf("%w: snapshot_id is required", ErrKBInvalid)
	}
	if s.embeddingRepo == nil {
		return nil, ErrKBSemanticSearchUnavailable
	}
	provider := s.embeddingProvider
	if provider == nil {
		provider = DisabledEmbeddingProvider{model: DefaultEmbeddingModel, dimensions: DefaultEmbeddingDimensions}
	}
	count, err := s.embeddingRepo.RetryFailedJobs(snapshotID, provider.Name(), provider.Model())
	if err != nil {
		return nil, err
	}
	return &RetryKBEmbeddingJobsResult{SnapshotID: snapshotID, Provider: provider.Name(), Model: provider.Model(), Retried: count}, nil
}

func (s *KBSearchService) EmbeddingStatus(ctx context.Context, snapshotID uuid.UUID) (*KBEmbeddingStatus, error) {
	if snapshotID == uuid.Nil {
		return nil, fmt.Errorf("%w: snapshot_id is required", ErrKBInvalid)
	}
	provider := s.embeddingProvider
	if provider == nil {
		provider = DisabledEmbeddingProvider{model: DefaultEmbeddingModel, dimensions: DefaultEmbeddingDimensions}
	}
	status := &KBEmbeddingStatus{SnapshotID: snapshotID, Provider: provider.Name(), Model: provider.Model(), Dimensions: provider.Dimensions()}
	if health, err := provider.Health(ctx); err != nil {
		status.ProviderUnavailableReason = err.Error()
	} else {
		status.ProviderHealth = health
	}
	if s.embeddingRepo != nil {
		coverage, err := s.embeddingRepo.Coverage(snapshotID, provider.Name(), provider.Model())
		if err != nil {
			return nil, err
		}
		status.Coverage = coverage
	}
	return status, nil
}

func (s *KBSearchService) IndexSnapshot(ctx context.Context, collection *model.KBCollection, snapshot *model.KBSnapshot, entries []model.KBSnapshotEntry, contentByEntryID map[string]string) error {
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
			ContentText:     buildKBSearchContentText(entry, contentByEntryID[entry.EntryID]),
			ContentHash:     snapshotEntryContentHash(snapshot, entry),
			Tokens:          entry.Tokens,
			Status:          "active",
			IndexedAt:       now,
		})
	}
	if s.embeddingRepo != nil && s.embeddingProvider != nil && s.embeddingProvider.Name() != EmbeddingProviderDisabled {
		return s.searchRepo.ReplaceSnapshotDocumentsWithEmbeddingJobs(snapshot.ID, docs, s.embeddingRepo, s.embeddingProvider.Name(), s.embeddingProvider.Model(), s.embeddingProvider.Dimensions())
	}
	return s.searchRepo.ReplaceSnapshotDocuments(snapshot.ID, docs)
}

func (s *KBSearchService) Search(ctx context.Context, input KBSearchInput) (*KBSearchResult, error) {
	mode := normalizeSearchMode(input.Mode)
	if mode == KBSearchModeSemantic {
		return s.searchSemantic(ctx, input)
	}
	if mode == KBSearchModeMetadata {
		return s.searchMetadata(input)
	}
	if mode == KBSearchModeHybrid {
		return s.searchHybrid(ctx, input)
	}
	result, err := s.searchLexical(input, false)
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
		score := lexicalScore(input.Q, doc)
		items = append(items, KBSearchResultItem{CollectionID: doc.CollectionID, SnapshotID: doc.SnapshotID, SnapshotEntryID: doc.SnapshotEntryID, EntryID: doc.EntryID, Title: doc.Title, Summary: doc.Summary, Tags: []string(doc.Tags), Tokens: doc.Tokens, Score: score, LexicalScore: float64Ptr(score)})
	}
	mode := KBSearchModeLexical
	semanticReason := ""
	if hybrid {
		mode = KBSearchModeHybrid
		semanticReason = ErrKBSemanticSearchUnavailable.Error()
	}
	return &KBSearchResult{Mode: string(mode), SemanticAvailable: false, SemanticUnavailableReason: semanticReason, Items: items, Limit: normalizeAPILimit(input.Limit), Offset: normalizeOffset(input.Offset), Total: total}, nil
}

func (s *KBSearchService) searchSemantic(ctx context.Context, input KBSearchInput) (*KBSearchResult, error) {
	if s.embeddingRepo == nil || s.embeddingProvider == nil || s.embeddingProvider.Name() == EmbeddingProviderDisabled {
		return nil, ErrKBSemanticSearchUnavailable
	}
	queryVector, err := s.embeddingProvider.EmbedTexts(ctx, []string{input.Q})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKBSemanticSearchUnavailable, err)
	}
	if len(queryVector) != 1 {
		return nil, fmt.Errorf("%w: invalid query embedding response", ErrKBSemanticSearchUnavailable)
	}
	hits, err := s.searchRepo.SearchSemantic(repository.KBSearchSemanticQuery{Vector: queryVector[0].Vector, Provider: s.embeddingProvider.Name(), Model: s.embeddingProvider.Model(), CollectionID: input.CollectionID, SnapshotID: input.SnapshotID, Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return nil, err
	}
	items := make([]KBSearchResultItem, 0, len(hits))
	for _, hit := range hits {
		doc := hit.Document
		items = append(items, KBSearchResultItem{CollectionID: doc.CollectionID, SnapshotID: doc.SnapshotID, SnapshotEntryID: doc.SnapshotEntryID, EntryID: doc.EntryID, Title: doc.Title, Summary: doc.Summary, Tags: []string(doc.Tags), Tokens: doc.Tokens, Score: hit.Score, SemanticScore: float64Ptr(hit.Score)})
	}
	result := &KBSearchResult{Mode: string(KBSearchModeSemantic), SemanticAvailable: len(items) > 0, Items: items, Limit: normalizeAPILimit(input.Limit), Offset: normalizeOffset(input.Offset), Total: int64(len(items))}
	if input.SnapshotID != nil && *input.SnapshotID != uuid.Nil {
		if coverage, err := s.embeddingRepo.Coverage(*input.SnapshotID, s.embeddingProvider.Name(), s.embeddingProvider.Model()); err == nil {
			result.EmbeddingCoverage = &coverage.Coverage
			if len(items) == 0 && coverage.ReadyEmbeddings == 0 {
				result.SemanticAvailable = false
				result.SemanticUnavailableReason = "no ready embeddings"
			}
		}
	}
	return result, nil
}

func (s *KBSearchService) searchHybrid(ctx context.Context, input KBSearchInput) (*KBSearchResult, error) {
	lexical, err := s.searchLexical(input, true)
	if err != nil {
		return nil, err
	}
	lexical.Mode = string(KBSearchModeHybrid)
	if s.embeddingRepo == nil || s.embeddingProvider == nil || s.embeddingProvider.Name() == EmbeddingProviderDisabled {
		lexical.SemanticAvailable = false
		lexical.SemanticUnavailableReason = ErrKBSemanticSearchUnavailable.Error()
		return lexical, nil
	}
	semantic, err := s.searchSemantic(ctx, input)
	if err != nil {
		lexical.SemanticAvailable = false
		lexical.SemanticUnavailableReason = err.Error()
		return lexical, nil
	}
	byKey := map[string]int{}
	for i, item := range lexical.Items {
		byKey[item.SnapshotEntryID.String()] = i
	}
	for _, semItem := range semantic.Items {
		if idx, ok := byKey[semItem.SnapshotEntryID.String()]; ok {
			lex := lexical.Items[idx]
			lexScore := lex.Score
			semScore := semItem.Score
			lexical.Items[idx].SemanticScore = float64Ptr(semScore)
			lexical.Items[idx].Score = (0.3 * normalizeScore(lexScore)) + (0.7 * normalizeScore(semScore))
			continue
		}
		semScore := semItem.Score
		semItem.Score = 0.7 * normalizeScore(semScore)
		semItem.SemanticScore = float64Ptr(semScore)
		lexical.Items = append(lexical.Items, semItem)
	}
	lexical.SemanticAvailable = semantic.SemanticAvailable
	lexical.SemanticUnavailableReason = semantic.SemanticUnavailableReason
	lexical.EmbeddingCoverage = semantic.EmbeddingCoverage
	sort.SliceStable(lexical.Items, func(i, j int) bool {
		return lexical.Items[i].Score > lexical.Items[j].Score
	})
	lexical.Total = int64(len(lexical.Items))
	return lexical, nil
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

func buildKBSearchContentText(entry model.KBSnapshotEntry, body string) string {
	parts := []string{entry.Title, entry.Summary, entry.EntryID}
	if strings.TrimSpace(body) != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n")
}

func snapshotEntryContentHash(snapshot *model.KBSnapshot, entry model.KBSnapshotEntry) string {
	if hash, ok := entry.Metadata["content_hash"].(string); ok && strings.TrimSpace(hash) != "" {
		return hash
	}
	if strings.TrimSpace(snapshot.ContentHash) != "" {
		return snapshot.ContentHash + ":" + entry.ID.String()
	}
	return entry.ID.String()
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

func float64Ptr(v float64) *float64 { return &v }

func normalizeScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
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
