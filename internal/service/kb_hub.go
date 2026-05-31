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

const maxKBFullTextBytes int64 = 2 << 20

type KBHubService struct {
	kbRepo             *repository.KBHubRepo
	knowledgeRepo      *repository.KnowledgeEntriesRepo
	objectSvc          *ObjectService
	billingSvc         *KBBillingService
	searchSvc          *KBSearchService
	governanceEnforcer *GovernanceEnforcer
}

type CreateKBCollectionInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsFree      *bool  `json:"is_free"`
}

type UpdateKBCollectionPricingInput struct {
	PricingModel     string         `json:"pricing_model"`
	MonthlyPrice     int64          `json:"monthly_price"`
	IsFree           *bool          `json:"is_free"`
	PlatformMinPrice int64          `json:"platform_min_price"`
	PlatformMaxPrice int64          `json:"platform_max_price"`
	EntitlementMode  string         `json:"entitlement_mode"`
	BillingInterval  string         `json:"billing_interval"`
	TrialDays        int            `json:"trial_days"`
	Currency         string         `json:"currency"`
	Metadata         map[string]any `json:"metadata"`
}

type SearchKBCollectionsInput struct {
	Q       string
	OwnerID *uuid.UUID
	IsFree  *bool
	Limit   int
	Offset  int
}

type PublishKBSnapshotInput struct {
	EntryIDs []string `json:"entry_ids"`
}

type InstallKBCollectionInput struct {
	TrackMode       string     `json:"track_mode"`
	PinnedVersion   *int       `json:"pinned_version"`
	ExpiresAt       *time.Time `json:"expires_at"`
	EntitlementType string     `json:"entitlement_type"`
	GrantReason     string     `json:"grant_reason"`
}

type UpdateKBCollectionDeclarationsInput struct {
	SourceDeclaration    string         `json:"source_declaration"`
	CopyrightDeclaration string         `json:"copyright_declaration"`
	ModerationMetadata   map[string]any `json:"moderation_metadata"`
}

type ReviewKBCollectionInput struct {
	ReviewStatus string `json:"review_status"`
	Reason       string `json:"reason"`
}

type ReportKBCollectionInput struct {
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

type ResolveKBModerationReportInput struct {
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
}

type KBSnapshotDiff struct {
	FromSnapshotID uuid.UUID               `json:"from_snapshot_id"`
	ToSnapshotID   uuid.UUID               `json:"to_snapshot_id"`
	Added          []model.KBSnapshotEntry `json:"added"`
	Removed        []model.KBSnapshotEntry `json:"removed"`
	Changed        []KBSnapshotEntryChange `json:"changed"`
	UnchangedCount int                     `json:"unchanged_count"`
}

type KBSnapshotEntryChange struct {
	EntryID string                `json:"entry_id"`
	From    model.KBSnapshotEntry `json:"from"`
	To      model.KBSnapshotEntry `json:"to"`
}

type KBSnapshotDetail struct {
	Snapshot *model.KBSnapshot       `json:"snapshot"`
	Entries  []model.KBSnapshotEntry `json:"entries"`
}

type KBPublicCollectionDetail struct {
	Collection     *model.KBCollection `json:"collection"`
	LatestSnapshot *model.KBSnapshot   `json:"latest_snapshot"`
}

type KBMarketplaceSearchResult struct {
	Items  []KBMarketplaceCollectionCard `json:"items"`
	Limit  int                           `json:"limit"`
	Offset int                           `json:"offset"`
	Total  int64                         `json:"total"`
}

type KBMarketplaceCollectionCard struct {
	CollectionID          uuid.UUID  `json:"collection_id"`
	OwnerID               uuid.UUID  `json:"owner_id"`
	Name                  string     `json:"name"`
	Description           string     `json:"description"`
	Status                string     `json:"status"`
	IsFree                bool       `json:"is_free"`
	PricingModel          string     `json:"pricing_model"`
	MonthlyPrice          int64      `json:"monthly_price"`
	LatestSnapshotID      *uuid.UUID `json:"latest_snapshot_id"`
	LatestVersion         int        `json:"latest_version"`
	EntryCount            int        `json:"entry_count"`
	TotalTokens           int        `json:"total_tokens"`
	PublishedAt           *time.Time `json:"published_at"`
	ActiveInstallCount    int64      `json:"active_install_count"`
	ManifestDownloadCount int64      `json:"manifest_download_count"`
	ContentDownloadCount  int64      `json:"content_download_count"`
}

type KBOwnerCollectionStats struct {
	CollectionID          uuid.UUID `json:"collection_id"`
	ActiveInstallCount    int64     `json:"active_install_count"`
	CancelledInstallCount int64     `json:"cancelled_install_count"`
	ManifestDownloadCount int64     `json:"manifest_download_count"`
	ContentDownloadCount  int64     `json:"content_download_count"`
	LatestSnapshotVersion int       `json:"latest_snapshot_version"`
	LatestSnapshotEntries int       `json:"latest_snapshot_entries"`
	LatestSnapshotTokens  int       `json:"latest_snapshot_tokens"`
}

type KBFullTextResponse struct {
	Entry   model.KBSnapshotEntry `json:"entry"`
	Content string                `json:"content"`
	Usage   *model.KBUsageRecord  `json:"usage"`
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

func (s *KBHubService) SetBillingService(billingSvc *KBBillingService) {
	s.billingSvc = billingSvc
}

func (s *KBHubService) SetSearchService(searchSvc *KBSearchService) {
	s.searchSvc = searchSvc
}

func (s *KBHubService) SetGovernanceEnforcer(enforcer *GovernanceEnforcer) {
	s.governanceEnforcer = enforcer
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

func (s *KBHubService) SearchPublicCollections(input SearchKBCollectionsInput) (*KBMarketplaceSearchResult, error) {
	query := repository.ListPublishedCollectionsQuery{Q: input.Q, OwnerID: input.OwnerID, IsFree: input.IsFree, Limit: input.Limit, Offset: input.Offset}
	collections, err := s.kbRepo.SearchPublishedCollections(query)
	if err != nil {
		return nil, err
	}
	total, err := s.kbRepo.CountPublishedCollections(query)
	if err != nil {
		return nil, err
	}
	cards := make([]KBMarketplaceCollectionCard, 0, len(collections))
	for _, collection := range collections {
		card, err := s.buildMarketplaceCard(collection)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return &KBMarketplaceSearchResult{Items: cards, Limit: normalizeAPILimit(input.Limit), Offset: normalizeOffset(input.Offset), Total: total}, nil
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

func (s *KBHubService) UpdateCollectionDeclarations(ownerID, collectionID uuid.UUID, input UpdateKBCollectionDeclarationsInput) (*model.KBCollection, error) {
	collection, err := s.GetCollection(ownerID, collectionID)
	if err != nil {
		return nil, err
	}
	collection.SourceDeclaration = strings.TrimSpace(input.SourceDeclaration)
	collection.CopyrightDeclaration = strings.TrimSpace(input.CopyrightDeclaration)
	if input.ModerationMetadata != nil {
		collection.ModerationMetadata = datatypes.JSONMap(input.ModerationMetadata)
	}
	collection.ReviewStatus = repository.KBReviewStatusPending
	collection.ReviewReason = ""
	collection.ReviewedBy = nil
	collection.ReviewedAt = nil
	if err := s.kbRepo.UpdateCollection(collection); err != nil {
		return nil, err
	}
	return collection, nil
}

func (s *KBHubService) ListCollectionsForReview(status string, limit, offset int) ([]model.KBCollection, error) {
	return s.kbRepo.ListCollectionsForReview(status, limit, offset)
}

func (s *KBHubService) ReviewCollection(adminID, collectionID uuid.UUID, input ReviewKBCollectionInput) (*model.KBCollection, error) {
	status := strings.TrimSpace(input.ReviewStatus)
	switch status {
	case repository.KBReviewStatusApproved, repository.KBReviewStatusRejected, repository.KBReviewStatusTakedown:
	default:
		return nil, fmt.Errorf("%w: review_status must be approved, rejected, or takedown", ErrKBInvalid)
	}
	if err := s.enforceKBGovernance(adminID, collectionID.String(), "kb.collection.review."+status, GovernanceRiskHigh, map[string]any{"collection_id": collectionID.String(), "review_status": status, "reason": strings.TrimSpace(input.Reason)}); err != nil {
		return nil, err
	}
	collection, err := s.kbRepo.UpdateCollectionReview(collectionID, adminID, status, strings.TrimSpace(input.Reason))
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBCollectionNotFound
		}
		return nil, err
	}
	return collection, nil
}

func (s *KBHubService) ReportCollection(reporterID, collectionID uuid.UUID, input ReportKBCollectionInput) (*model.KBModerationReport, error) {
	if reporterID == uuid.Nil {
		return nil, fmt.Errorf("%w: reporter is required", ErrKBInvalid)
	}
	if _, err := s.kbRepo.GetCollection(collectionID); err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBCollectionNotFound
		}
		return nil, err
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrKBInvalid)
	}
	report := &model.KBModerationReport{CollectionID: collectionID, ReporterID: reporterID, Reason: reason, Detail: strings.TrimSpace(input.Detail), Status: "open"}
	if err := s.kbRepo.CreateModerationReport(report); err != nil {
		return nil, err
	}
	return report, nil
}

func (s *KBHubService) ListModerationReports(status string, limit, offset int) ([]model.KBModerationReport, error) {
	return s.kbRepo.ListModerationReports(status, limit, offset)
}

func (s *KBHubService) ResolveModerationReport(adminID, reportID uuid.UUID, input ResolveKBModerationReportInput) (*model.KBModerationReport, error) {
	status := strings.TrimSpace(input.Status)
	if status != "resolved" && status != "dismissed" {
		return nil, fmt.Errorf("%w: status must be resolved or dismissed", ErrKBInvalid)
	}
	report, err := s.kbRepo.ResolveModerationReport(reportID, adminID, status, strings.TrimSpace(input.Resolution))
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *KBHubService) UpdateCollectionPricing(ownerID, collectionID uuid.UUID, input UpdateKBCollectionPricingInput) (*model.KBCollection, error) {
	collection, err := s.GetCollection(ownerID, collectionID)
	if err != nil {
		return nil, err
	}
	isFree := collection.IsFree
	if input.IsFree != nil {
		isFree = *input.IsFree
	}
	pricingModel := strings.TrimSpace(input.PricingModel)
	monthlyPrice := input.MonthlyPrice
	if isFree {
		pricingModel = "free"
		monthlyPrice = 0
	}
	if !isFree {
		if pricingModel == "" {
			pricingModel = "monthly"
		}
		if pricingModel != "monthly" && pricingModel != "token" {
			return nil, fmt.Errorf("%w: pricing_model must be monthly or token", ErrKBInvalid)
		}
		if monthlyPrice <= 0 {
			return nil, fmt.Errorf("%w: monthly_price must be positive for paid collections", ErrKBInvalid)
		}
		if input.PlatformMinPrice > 0 && monthlyPrice < input.PlatformMinPrice {
			return nil, fmt.Errorf("%w: monthly_price below platform minimum", ErrKBInvalid)
		}
		if input.PlatformMaxPrice > 0 && monthlyPrice > input.PlatformMaxPrice {
			return nil, fmt.Errorf("%w: monthly_price above platform maximum", ErrKBInvalid)
		}
	}
	entitlement := ""
	if strings.TrimSpace(input.EntitlementMode) != "" {
		entitlement = normalizeEntitlement(input.EntitlementMode)
		if entitlement == "" {
			return nil, fmt.Errorf("%w: invalid entitlement_mode", ErrKBInvalid)
		}
	}
	if entitlement == "" {
		if isFree {
			entitlement = KBEntitlementFree
		} else {
			entitlement = KBEntitlementPaid
		}
	}
	billingInterval := normalizeBillingInterval(input.BillingInterval, entitlement)
	if billingInterval == "" {
		return nil, fmt.Errorf("%w: invalid billing_interval", ErrKBInvalid)
	}
	currency := strings.TrimSpace(input.Currency)
	if currency == "" {
		currency = "CNY"
	}
	if entitlement == KBEntitlementFree || entitlement == KBEntitlementGranted {
		isFree = true
		pricingModel = "free"
		monthlyPrice = 0
	}
	if err := s.enforceKBGovernance(ownerID, collectionID.String(), "kb.collection.pricing.update", GovernanceRiskHigh, map[string]any{"collection_id": collectionID.String(), "pricing_model": pricingModel, "monthly_price": monthlyPrice, "entitlement_mode": entitlement, "billing_interval": billingInterval, "currency": currency}); err != nil {
		return nil, err
	}
	collection.IsFree = isFree
	collection.PricingModel = pricingModel
	collection.MonthlyPrice = monthlyPrice
	collection.PlatformMinPrice = input.PlatformMinPrice
	collection.PlatformMaxPrice = input.PlatformMaxPrice
	collection.EntitlementMode = entitlement
	collection.BillingInterval = billingInterval
	collection.TrialDays = input.TrialDays
	collection.Currency = currency
	if err := s.kbRepo.UpdateCollection(collection); err != nil {
		return nil, err
	}
	if s.billingSvc != nil {
		if _, err := s.billingSvc.UpsertBillingPlan(UpsertKBBillingPlanInput{CollectionID: collectionID, EntitlementType: entitlement, BillingInterval: billingInterval, Price: monthlyPrice, Currency: currency, TrialDays: input.TrialDays, Metadata: input.Metadata}); err != nil {
			return nil, err
		}
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
	if s.searchSvc != nil {
		if err := s.searchSvc.IndexSnapshot(ctx, collection, snapshot, createdEntries); err != nil {
			return nil, err
		}
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

func (s *KBHubService) ArchiveSnapshot(ownerID, collectionID, snapshotID uuid.UUID) (*model.KBSnapshot, error) {
	if _, err := s.GetCollection(ownerID, collectionID); err != nil {
		return nil, err
	}
	if err := s.enforceKBGovernance(ownerID, snapshotID.String(), "kb.snapshot.archive", GovernanceRiskMedium, map[string]any{"collection_id": collectionID.String(), "snapshot_id": snapshotID.String()}); err != nil {
		return nil, err
	}
	snapshot, err := s.kbRepo.ArchiveSnapshot(collectionID, snapshotID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	return snapshot, nil
}

func (s *KBHubService) RestoreSnapshot(ownerID, collectionID, snapshotID uuid.UUID) (*model.KBSnapshot, error) {
	if _, err := s.GetCollection(ownerID, collectionID); err != nil {
		return nil, err
	}
	if err := s.enforceKBGovernance(ownerID, snapshotID.String(), "kb.snapshot.restore", GovernanceRiskMedium, map[string]any{"collection_id": collectionID.String(), "snapshot_id": snapshotID.String()}); err != nil {
		return nil, err
	}
	snapshot, err := s.kbRepo.RestoreSnapshot(collectionID, snapshotID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	return snapshot, nil
}

func (s *KBHubService) enforceKBGovernance(actorID uuid.UUID, subjectID, capabilityKey, risk string, context map[string]any) error {
	if s.governanceEnforcer == nil {
		return nil
	}
	actor := actorID
	_, err := s.governanceEnforcer.Enforce(GovernanceEnforcementInput{ActorUserID: &actor, SubjectType: GovernanceSubjectKBOperation, SubjectID: subjectID, CapabilityKey: capabilityKey, RiskLevel: risk, Context: context})
	return err
}

func (s *KBHubService) DiffSnapshots(ownerID, collectionID, fromSnapshotID, toSnapshotID uuid.UUID) (*KBSnapshotDiff, error) {
	if _, err := s.GetCollection(ownerID, collectionID); err != nil {
		return nil, err
	}
	fromEntries, err := s.kbRepo.ListSnapshotEntries(fromSnapshotID)
	if err != nil {
		return nil, err
	}
	toEntries, err := s.kbRepo.ListSnapshotEntries(toSnapshotID)
	if err != nil {
		return nil, err
	}
	fromByID := map[string]model.KBSnapshotEntry{}
	toByID := map[string]model.KBSnapshotEntry{}
	for _, entry := range fromEntries {
		fromByID[entry.EntryID] = entry
	}
	for _, entry := range toEntries {
		toByID[entry.EntryID] = entry
	}
	diff := &KBSnapshotDiff{FromSnapshotID: fromSnapshotID, ToSnapshotID: toSnapshotID}
	for entryID, to := range toByID {
		from, ok := fromByID[entryID]
		if !ok {
			diff.Added = append(diff.Added, to)
			continue
		}
		if snapshotEntryChanged(from, to) {
			diff.Changed = append(diff.Changed, KBSnapshotEntryChange{EntryID: entryID, From: from, To: to})
		} else {
			diff.UnchangedCount++
		}
	}
	for entryID, from := range fromByID {
		if _, ok := toByID[entryID]; !ok {
			diff.Removed = append(diff.Removed, from)
		}
	}
	return diff, nil
}

func (s *KBHubService) GetPublicSnapshot(collectionID, snapshotID uuid.UUID) (*KBSnapshotDetail, error) {
	if _, err := s.GetPublicCollection(collectionID); err != nil {
		return nil, err
	}
	return s.getSnapshotDetail(collectionID, snapshotID)
}

func (s *KBHubService) CreateSnapshotManifestDownloadURL(ctx context.Context, collectionID, snapshotID uuid.UUID) (*DownloadURLResponse, error) {
	public, err := s.GetPublicCollection(collectionID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.kbRepo.GetSnapshot(collectionID, snapshotID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrKBSnapshotNotFound
		}
		return nil, err
	}
	result, err := s.objectSvc.CreateDownloadURLByObjectURI(ctx, snapshot.ManifestObjectURI, CreateDownloadURLInput{Disposition: "attachment"})
	if err != nil {
		return nil, err
	}
	_, _ = s.recordUsage(RecordKBUsageInput{OwnerID: public.Collection.OwnerID, CollectionID: collectionID, SnapshotID: snapshotID, OperationType: KBOperationManifestDownloadURLPublic, IsFree: true})
	return result, nil
}

func (s *KBHubService) InstallCollection(userID, collectionID uuid.UUID, input InstallKBCollectionInput) (*model.KBSubscription, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user is required", ErrKBInvalid)
	}
	public, err := s.GetPublicCollection(collectionID)
	if err != nil {
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
	entitlement := normalizeEntitlement(input.EntitlementType)
	if entitlement == "" {
		entitlement = public.Collection.EntitlementMode
	}
	if entitlement == "" {
		if public.Collection.IsFree {
			entitlement = KBEntitlementFree
		} else {
			entitlement = KBEntitlementPaid
		}
	}
	expiresAt := input.ExpiresAt
	periodStart := now
	periodEnd := expiresAt
	renewalStatus := "none"
	if entitlement == KBEntitlementTrial {
		if periodEnd == nil {
			trialDays := public.Collection.TrialDays
			if trialDays <= 0 {
				trialDays = 14
			}
			end := now.AddDate(0, 0, trialDays)
			periodEnd = &end
			expiresAt = &end
		}
		renewalStatus = "active"
	} else if entitlement == KBEntitlementPaid {
		if periodEnd == nil {
			end := addBillingPeriod(now, public.Collection.BillingInterval)
			periodEnd = &end
			expiresAt = &end
		}
		renewalStatus = "active"
	} else if entitlement == KBEntitlementGranted {
		if strings.TrimSpace(input.GrantReason) == "" {
			return nil, fmt.Errorf("%w: grant_reason is required for granted entitlement", ErrKBInvalid)
		}
	}
	subscription := &model.KBSubscription{
		UserID:             userID,
		CollectionID:       collectionID,
		SnapshotID:         snapshot.ID,
		TrackMode:          trackMode,
		PinnedVersion:      pinnedVersion,
		Status:             "active",
		StartedAt:          now,
		ExpiresAt:          expiresAt,
		EntitlementType:    entitlement,
		RenewalStatus:      renewalStatus,
		CurrentPeriodStart: &periodStart,
		CurrentPeriodEnd:   periodEnd,
		GrantReason:        strings.TrimSpace(input.GrantReason),
	}
	if err := s.kbRepo.UpsertSubscription(subscription); err != nil {
		return nil, err
	}
	return subscription, nil
}

func (s *KBHubService) ListSubscriptions(userID uuid.UUID) ([]model.KBSubscription, error) {
	return s.kbRepo.ListSubscriptionsByUser(userID)
}

func (s *KBHubService) ExpireSubscriptions(now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return s.kbRepo.ExpireSubscriptions(now)
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
	result, err := s.objectSvc.CreateDownloadURLByObjectURI(ctx, snapshot.ManifestObjectURI, CreateDownloadURLInput{Disposition: "attachment"})
	if err != nil {
		return nil, err
	}
	collection, _ := s.kbRepo.GetPublishedCollection(collectionID)
	_, _ = s.recordUsage(RecordKBUsageInput{UserID: userID, OwnerID: ownerIDFromCollection(collection), CollectionID: collectionID, SnapshotID: snapshotID, OperationType: KBOperationManifestDownloadURLInstalled, IsFree: collection == nil || collection.IsFree})
	return result, nil
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
	result, err := s.objectSvc.CreateDownloadURLByObjectURI(ctx, entry.ContentObjectURI, CreateDownloadURLInput{Disposition: "attachment"})
	if err != nil {
		return nil, err
	}
	collection, _ := s.kbRepo.GetPublishedCollection(collectionID)
	entryID := entry.ID
	_, _ = s.recordUsage(RecordKBUsageInput{UserID: userID, OwnerID: ownerIDFromCollection(collection), CollectionID: collectionID, SnapshotID: snapshotID, SnapshotEntryID: &entryID, OperationType: KBOperationEntryContentDownloadURL, TokensUsed: entry.Tokens, IsFree: collection == nil || collection.IsFree})
	return result, nil
}

func (s *KBHubService) FetchInstalledEntryFullText(ctx context.Context, userID, collectionID, snapshotID, entryRecordID uuid.UUID) (*KBFullTextResponse, error) {
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
	_, content, err := s.objectSvc.ReadObjectByObjectURI(ctx, entry.ContentObjectURI, maxKBFullTextBytes)
	if err != nil {
		return nil, err
	}
	collection, _ := s.kbRepo.GetPublishedCollection(collectionID)
	entryID := entry.ID
	usage, _ := s.recordUsage(RecordKBUsageInput{UserID: userID, OwnerID: ownerIDFromCollection(collection), CollectionID: collectionID, SnapshotID: snapshotID, SnapshotEntryID: &entryID, OperationType: KBOperationEntryFullTextFetch, TokensUsed: entry.Tokens, IsFree: collection == nil || collection.IsFree})
	return &KBFullTextResponse{Entry: *entry, Content: string(content), Usage: usage}, nil
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

func (s *KBHubService) GetCollectionStats(ownerID, collectionID uuid.UUID) (*KBOwnerCollectionStats, error) {
	if _, err := s.GetCollection(ownerID, collectionID); err != nil {
		return nil, err
	}
	active, err := s.kbRepo.CountSubscriptionsByStatus(collectionID, "active")
	if err != nil {
		return nil, err
	}
	cancelled, err := s.kbRepo.CountSubscriptionsByStatus(collectionID, "cancelled")
	if err != nil {
		return nil, err
	}
	usage, err := s.kbRepo.UsageCounts(collectionID)
	if err != nil {
		return nil, err
	}
	stats := &KBOwnerCollectionStats{CollectionID: collectionID, ActiveInstallCount: active, CancelledInstallCount: cancelled, ManifestDownloadCount: usage.ManifestDownloadCount, ContentDownloadCount: usage.ContentDownloadCount}
	latest, err := s.kbRepo.GetLatestSnapshot(collectionID)
	if err == nil {
		stats.LatestSnapshotVersion = latest.Version
		stats.LatestSnapshotEntries = latest.EntryCount
		stats.LatestSnapshotTokens = latest.TotalTokens
	} else if !repository.IsNotFound(err) {
		return nil, err
	}
	return stats, nil
}

func (s *KBHubService) buildMarketplaceCard(collection model.KBCollection) (KBMarketplaceCollectionCard, error) {
	card := KBMarketplaceCollectionCard{CollectionID: collection.ID, OwnerID: collection.OwnerID, Name: collection.Name, Description: collection.Description, Status: collection.Status, IsFree: collection.IsFree, PricingModel: collection.PricingModel, MonthlyPrice: collection.MonthlyPrice}
	latest, err := s.kbRepo.GetLatestSnapshot(collection.ID)
	if err == nil {
		card.LatestSnapshotID = &latest.ID
		card.LatestVersion = latest.Version
		card.EntryCount = latest.EntryCount
		card.TotalTokens = latest.TotalTokens
		card.PublishedAt = &latest.PublishedAt
	} else if !repository.IsNotFound(err) {
		return card, err
	}
	active, err := s.kbRepo.CountSubscriptionsByStatus(collection.ID, "active")
	if err != nil {
		return card, err
	}
	card.ActiveInstallCount = active
	usage, err := s.kbRepo.UsageCounts(collection.ID)
	if err != nil {
		return card, err
	}
	card.ManifestDownloadCount = usage.ManifestDownloadCount
	card.ContentDownloadCount = usage.ContentDownloadCount
	return card, nil
}

func (s *KBHubService) recordUsage(input RecordKBUsageInput) (*model.KBUsageRecord, error) {
	if s.billingSvc == nil {
		return nil, nil
	}
	record, _, _, err := s.billingSvc.RecordUsage(input)
	return record, err
}

func ownerIDFromCollection(collection *model.KBCollection) uuid.UUID {
	if collection == nil {
		return uuid.Nil
	}
	return collection.OwnerID
}

func addBillingPeriod(start time.Time, interval string) time.Time {
	switch strings.TrimSpace(interval) {
	case KBBillingIntervalYear:
		return start.AddDate(1, 0, 0)
	case KBBillingIntervalMonth, "":
		return start.AddDate(0, 1, 0)
	default:
		return start
	}
}

func snapshotEntryChanged(from, to model.KBSnapshotEntry) bool {
	return from.Title != to.Title || from.Summary != to.Summary || from.ContentObjectURI != to.ContentObjectURI || from.Tokens != to.Tokens || fmt.Sprint(from.Tags) != fmt.Sprint(to.Tags) || fmt.Sprint(from.Metadata) != fmt.Sprint(to.Metadata)
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
