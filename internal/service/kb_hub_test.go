package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestKBHubServicePublishesImmutableSnapshot(t *testing.T) {
	svc, knowledgeRepo, fake, ownerID, _ := newKBHubServiceTestEnv(t)
	ctx := context.Background()

	entry := &model.UserKnowledgeEntry{
		UserID:          ownerID,
		EntryID:         "notes/alpha",
		Title:           "Alpha",
		ContentMarkdown: "# Alpha\n\nOriginal content.",
		Summary:         "Alpha summary",
		Tags:            datatypes.NewJSONSlice([]string{"alpha"}),
		Metadata:        datatypes.JSONMap{"source": "test"},
		Status:          repository.KnowledgeEntryStatusActive,
		Version:         1,
		ContentHash:     strings.Repeat("a", 64),
	}
	if err := knowledgeRepo.Create(entry); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}

	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "My KB", Description: "snapshot test"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	detail, err := svc.PublishSnapshot(ctx, ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	if detail.Snapshot.Version != 1 || detail.Snapshot.EntryCount != 1 || detail.Snapshot.ManifestObjectURI == "" || detail.Snapshot.Checksum == "" {
		t.Fatalf("unexpected snapshot: %+v", detail.Snapshot)
	}
	if len(detail.Entries) != 1 || detail.Entries[0].ContentObjectURI == "" || detail.Entries[0].Title != "Alpha" {
		t.Fatalf("unexpected snapshot entries: %+v", detail.Entries)
	}
	if len(fake.puts) != 2 {
		t.Fatalf("expected one content object and one manifest object, got %d", len(fake.puts))
	}

	entry.ContentMarkdown = "# Alpha\n\nChanged after snapshot."
	entry.Title = "Alpha changed"
	entry.ContentHash = strings.Repeat("b", 64)
	if err := knowledgeRepo.UpdateActive(entry); err != nil {
		t.Fatalf("update source entry: %v", err)
	}

	loaded, err := svc.GetSnapshot(ownerID, collection.ID, detail.Snapshot.ID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if loaded.Entries[0].Title != "Alpha" {
		t.Fatalf("snapshot entry mutated with source entry, got title %q", loaded.Entries[0].Title)
	}

	detail2, err := svc.PublishSnapshot(ctx, ownerID, collection.ID, PublishKBSnapshotInput{EntryIDs: []string{"notes/alpha"}})
	if err != nil {
		t.Fatalf("publish second snapshot: %v", err)
	}
	if detail2.Snapshot.Version != 2 || detail2.Entries[0].Title != "Alpha changed" {
		t.Fatalf("expected second snapshot to capture new source state, got snapshot=%+v entries=%+v", detail2.Snapshot, detail2.Entries)
	}
}

func TestKBHubServiceRejectsEmptySnapshot(t *testing.T) {
	svc, _, _, ownerID, _ := newKBHubServiceTestEnv(t)
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Empty KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	_, err = svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err == nil || !strings.Contains(err.Error(), ErrKBNoEntries.Error()) {
		t.Fatalf("expected no entries error, got %v", err)
	}
}

func TestKBHubServicePublicReadManifestAndInstall(t *testing.T) {
	svc, knowledgeRepo, _, ownerID, _ := newKBHubServiceTestEnv(t)
	consumerID := uuid.New()
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "notes/public", Title: "Public", ContentMarkdown: "# Public", Summary: "public", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("c", 64)}); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Public KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if public, err := svc.ListPublicCollections(); err != nil || len(public) != 0 {
		t.Fatalf("draft collection should not be public, got public=%+v err=%v", public, err)
	}
	detail, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	public, err := svc.ListPublicCollections()
	if err != nil || len(public) != 1 || public[0].ID != collection.ID {
		t.Fatalf("expected published collection in public list, got public=%+v err=%v", public, err)
	}
	publicDetail, err := svc.GetPublicCollection(collection.ID)
	if err != nil {
		t.Fatalf("get public collection: %v", err)
	}
	if publicDetail.LatestSnapshot == nil || publicDetail.LatestSnapshot.ID != detail.Snapshot.ID {
		t.Fatalf("unexpected public detail: %+v", publicDetail)
	}
	manifestURL, err := svc.CreateSnapshotManifestDownloadURL(context.Background(), collection.ID, detail.Snapshot.ID)
	if err != nil {
		t.Fatalf("manifest download url: %v", err)
	}
	if manifestURL.DownloadURL == "" || manifestURL.Object.ObjectURI != detail.Snapshot.ManifestObjectURI {
		t.Fatalf("unexpected manifest download response: %+v", manifestURL)
	}
	sub, err := svc.InstallCollection(consumerID, collection.ID, InstallKBCollectionInput{})
	if err != nil {
		t.Fatalf("install latest: %v", err)
	}
	if sub.TrackMode != "latest" || sub.SnapshotID != detail.Snapshot.ID || sub.Status != "active" {
		t.Fatalf("unexpected subscription: %+v", sub)
	}
	pinned := 1
	sub, err = svc.InstallCollection(consumerID, collection.ID, InstallKBCollectionInput{TrackMode: "pinned", PinnedVersion: &pinned})
	if err != nil {
		t.Fatalf("install pinned: %v", err)
	}
	if sub.TrackMode != "pinned" || sub.PinnedVersion == nil || *sub.PinnedVersion != 1 {
		t.Fatalf("unexpected pinned subscription: %+v", sub)
	}
	subs, err := svc.ListSubscriptions(consumerID)
	if err != nil || len(subs) != 1 {
		t.Fatalf("expected one upserted subscription, got subs=%+v err=%v", subs, err)
	}
}

func TestKBHubServiceInstalledAccessAndCancel(t *testing.T) {
	svc, knowledgeRepo, _, ownerID, _ := newKBHubServiceTestEnv(t)
	consumerID := uuid.New()
	strangerID := uuid.New()
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "notes/access", Title: "Access", ContentMarkdown: "# Access", Summary: "access", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("d", 64)}); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Access KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	detail, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	entryRecordID := detail.Entries[0].ID
	if _, err := svc.CreateInstalledManifestDownloadURL(context.Background(), strangerID, collection.ID, detail.Snapshot.ID); err == nil || !strings.Contains(err.Error(), ErrKBSubscriptionNeeded.Error()) {
		t.Fatalf("expected stranger manifest access denial, got %v", err)
	}
	if _, err := svc.InstallCollection(consumerID, collection.ID, InstallKBCollectionInput{}); err != nil {
		t.Fatalf("install latest: %v", err)
	}
	manifestURL, err := svc.CreateInstalledManifestDownloadURL(context.Background(), consumerID, collection.ID, detail.Snapshot.ID)
	if err != nil {
		t.Fatalf("installed manifest url: %v", err)
	}
	if manifestURL.DownloadURL == "" {
		t.Fatal("expected manifest download URL")
	}
	contentURL, err := svc.CreateInstalledEntryContentDownloadURL(context.Background(), consumerID, collection.ID, detail.Snapshot.ID, entryRecordID)
	if err != nil {
		t.Fatalf("installed content url: %v", err)
	}
	if contentURL.DownloadURL == "" || contentURL.Object.ObjectURI != detail.Entries[0].ContentObjectURI {
		t.Fatalf("unexpected content url response: %+v", contentURL)
	}
	fullText, err := svc.FetchInstalledEntryFullText(context.Background(), consumerID, collection.ID, detail.Snapshot.ID, entryRecordID)
	if err != nil {
		t.Fatalf("fulltext fetch: %v", err)
	}
	if fullText.Entry.ID != entryRecordID || fullText.Usage == nil || fullText.Usage.OperationType != KBOperationEntryFullTextFetch || !strings.Contains(fullText.Content, "# Access") {
		t.Fatalf("unexpected fulltext response: %+v", fullText)
	}
	stats, err := svc.GetCollectionStats(ownerID, collection.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.ActiveInstallCount != 1 || stats.ManifestDownloadCount != 1 || stats.ContentDownloadCount != 1 {
		t.Fatalf("expected usage stats to reflect access, got %+v", stats)
	}
	cancelled, err := svc.CancelSubscription(consumerID, collection.ID)
	if err != nil {
		t.Fatalf("cancel subscription: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled subscription, got %+v", cancelled)
	}
	if _, err := svc.CreateInstalledEntryContentDownloadURL(context.Background(), consumerID, collection.ID, detail.Snapshot.ID, entryRecordID); err == nil || !strings.Contains(err.Error(), ErrKBSubscriptionNeeded.Error()) {
		t.Fatalf("expected access denial after cancel, got %v", err)
	}
}

func TestKBHubServiceFullM3MarketplacePricingAndSearch(t *testing.T) {
	svc, knowledgeRepo, _, ownerID, _ := newKBHubServiceTestEnv(t)
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "strategy/blue-ocean", Title: "Blue Ocean Strategy", ContentMarkdown: "# Blue Ocean\n\nValue innovation", Summary: "Create uncontested market space", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("e", 64)}); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Strategy Library", Description: "Business strategy frameworks"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	paid := false
	priced, err := svc.UpdateCollectionPricing(ownerID, collection.ID, UpdateKBCollectionPricingInput{IsFree: &paid, PricingModel: "monthly", MonthlyPrice: 9900, PlatformMinPrice: 100, PlatformMaxPrice: 20000})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if priced.IsFree || priced.PricingModel != "monthly" || priced.MonthlyPrice != 9900 {
		t.Fatalf("unexpected pricing: %+v", priced)
	}
	detail, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	market, err := svc.SearchPublicCollections(SearchKBCollectionsInput{Q: "strategy", Limit: 10})
	if err != nil {
		t.Fatalf("search public collections: %v", err)
	}
	if market.Total != 1 || len(market.Items) != 1 || market.Items[0].LatestSnapshotID == nil || *market.Items[0].LatestSnapshotID != detail.Snapshot.ID || market.Items[0].MonthlyPrice != 9900 {
		t.Fatalf("unexpected marketplace result: %+v", market)
	}
	search, err := svc.searchSvc.Search(context.Background(), KBSearchInput{Q: "Blue Ocean", Mode: "lexical", CollectionID: &collection.ID, Limit: 10})
	if err != nil {
		t.Fatalf("lexical search: %v", err)
	}
	if search.Total != 1 || len(search.Items) != 1 || search.Items[0].EntryID != "strategy/blue-ocean" {
		t.Fatalf("unexpected lexical search result: %+v", search)
	}
	worker := NewKBEmbeddingWorker(svc.searchSvc.embeddingRepo, svc.searchSvc.embeddingProvider, KBEmbeddingWorkerConfig{BatchSize: 8})
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("process embedding jobs: %v", err)
	}
	semantic, err := svc.searchSvc.Search(context.Background(), KBSearchInput{Q: "Blue Ocean", Mode: "semantic", SnapshotID: &detail.Snapshot.ID, Limit: 10})
	if err != nil {
		t.Fatalf("semantic search: %v", err)
	}
	if len(semantic.Items) != 1 || !semantic.SemanticAvailable {
		t.Fatalf("expected deterministic semantic result, got %+v", semantic)
	}
	status, err := svc.searchSvc.EmbeddingStatus(context.Background(), detail.Snapshot.ID)
	if err != nil {
		t.Fatalf("embedding status: %v", err)
	}
	if status.Coverage == nil || status.Coverage.ReadyEmbeddings != 1 || status.Coverage.Coverage != 1 {
		t.Fatalf("unexpected embedding status: %+v", status)
	}
}

func TestKBHubServiceIndexesSnapshotBodyContent(t *testing.T) {
	svc, knowledgeRepo, _, ownerID, _ := newKBHubServiceTestEnv(t)
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{
		UserID:          ownerID,
		EntryID:         "notes/body-only",
		Title:           "Ordinary Note",
		ContentMarkdown: "# Ordinary Note\n\nThe hidden retrieval phrase is quokka-vector-signal and appears only in the markdown body.",
		Summary:         "A generic note without the special phrase",
		Status:          repository.KnowledgeEntryStatusActive,
		Version:         1,
		ContentHash:     strings.Repeat("f", 64),
	}); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Body KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	detail, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	search, err := svc.searchSvc.Search(context.Background(), KBSearchInput{Q: "quokka-vector-signal", Mode: "lexical", SnapshotID: &detail.Snapshot.ID, Limit: 10})
	if err != nil {
		t.Fatalf("body lexical search: %v", err)
	}
	if search.Total != 1 || len(search.Items) != 1 || search.Items[0].EntryID != "notes/body-only" {
		t.Fatalf("expected body-only term to be indexed, got %+v", search)
	}
}

func TestKBSearchHybridSortsByCombinedScore(t *testing.T) {
	_, _, _, _, db := newKBHubServiceTestEnv(t)
	collectionID := uuid.New()
	snapshotID := uuid.New()
	now := strings.Repeat("1", 64)
	docs := []model.KBSearchDocument{
		{CollectionID: collectionID, SnapshotID: snapshotID, SnapshotEntryID: uuid.New(), EntryID: "weak-lexical", Title: "needle", Summary: "weak", ContentText: "needle", ContentHash: now, Status: "active"},
		{CollectionID: collectionID, SnapshotID: snapshotID, SnapshotEntryID: uuid.New(), EntryID: "strong-semantic", Title: "semantic", Summary: "semantic", ContentText: "semantic", ContentHash: strings.Repeat("2", 64), Status: "active"},
	}
	if err := db.Create(&docs).Error; err != nil {
		t.Fatalf("create docs: %v", err)
	}
	provider := NewDeterministicEmbeddingProvider("BAAI/bge-m3", 16)
	searchSvc := NewKBSearchService(repository.NewKBSearchRepo(db), repository.NewKBHubRepo(db), nil)
	embeddingRepo := repository.NewKBEmbeddingRepo(db)
	searchSvc.SetEmbedding(embeddingRepo, provider)
	queryVec, err := provider.EmbedTexts(context.Background(), []string{"needle"})
	if err != nil {
		t.Fatalf("query embedding: %v", err)
	}
	if err := embeddingRepo.CompleteJob(model.KBEmbeddingJob{SearchDocumentID: docs[1].ID, CollectionID: collectionID, SnapshotID: snapshotID, SnapshotEntryID: docs[1].SnapshotEntryID, Provider: provider.Name(), Model: provider.Model(), Dimensions: 16, ContentHash: docs[1].ContentHash}, queryVec[0].Vector); err != nil {
		t.Fatalf("complete semantic embedding: %v", err)
	}
	result, err := searchSvc.Search(context.Background(), KBSearchInput{Q: "needle", Mode: "hybrid", SnapshotID: &snapshotID, Limit: 10})
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(result.Items) < 2 {
		t.Fatalf("expected merged lexical and semantic candidates, got %+v", result)
	}
	for i := 1; i < len(result.Items); i++ {
		if result.Items[i-1].Score < result.Items[i].Score {
			t.Fatalf("hybrid results not sorted by score: %+v", result.Items)
		}
	}
}

func newKBHubServiceTestEnv(t *testing.T) (*KBHubService, *repository.KnowledgeEntriesRepo, *recordingObjectStorageBackend, uuid.UUID, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.UserKnowledgeEntry{}, &model.ObjectRecord{}, &model.KBCollection{}, &model.KBSnapshot{}, &model.KBSnapshotEntry{}, &model.KBSubscription{}, &model.KBModerationReport{}, &model.KBUsageRecord{}, &model.KBSearchDocument{}, &model.KBEmbeddingJob{}, &model.KBSearchEmbedding{}, &model.KBBillingPlan{}, &model.KBInvoice{}, &model.KBInvoiceItem{}, &model.ContributorPayoutPeriod{}, &model.KBRefund{}, &model.KBBillingDispute{}, &model.BillingAccount{}, &model.BillingTransaction{}, &model.ContributorEarning{}, &model.CapabilityDefinition{}, &model.PolicyRule{}, &model.PolicyDecision{}, &model.ApprovalReceipt{}, &model.KillSwitch{}, &model.GovernanceScanResult{}); err != nil {
		t.Fatalf("migrate models: %v", err)
	}
	fake := &recordingObjectStorageBackend{fakeObjectStorageBackend: fakeObjectStorageBackend{head: ObjectHead{ContentHash: strings.Repeat("a", 64), ContentSize: 1}}}
	objectSvc := NewObjectService(repository.NewObjectRecordsRepo(db), fake, config.ObjectStorageConfig{Bucket: "agentos-test", UploadTTLSecs: 900, DownloadTTLSecs: 900})
	kbRepo := repository.NewKBHubRepo(db)
	billingSvc := NewKBBillingService(repository.NewBillingRepo(db))
	embeddingRepo := repository.NewKBEmbeddingRepo(db)
	searchSvc := NewKBSearchService(repository.NewKBSearchRepo(db), kbRepo, billingSvc)
	searchSvc.SetEmbedding(embeddingRepo, NewDeterministicEmbeddingProvider("BAAI/bge-m3", 1024))
	svc := NewKBHubService(kbRepo, repository.NewKnowledgeEntriesRepo(db), objectSvc)
	svc.SetBillingService(billingSvc)
	svc.SetSearchService(searchSvc)
	return svc, repository.NewKnowledgeEntriesRepo(db), fake, uuid.New(), db
}

type recordedPut struct {
	Bucket      string
	Key         string
	Size        int64
	ContentType string
	Content     string
}

type recordingObjectStorageBackend struct {
	fakeObjectStorageBackend
	puts    []recordedPut
	objects map[string]string
}

func (f *recordingObjectStorageBackend) PutObject(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string, metadata map[string]string) error {
	_ = ctx
	_ = metadata
	buf, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if f.objects == nil {
		f.objects = map[string]string{}
	}
	f.objects[bucket+"/"+key] = string(buf)
	f.puts = append(f.puts, recordedPut{Bucket: bucket, Key: key, Size: size, ContentType: contentType, Content: string(buf)})
	return nil
}

func (f *recordingObjectStorageBackend) ReadObject(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, error) {
	_ = ctx
	_ = maxBytes
	return []byte(f.objects[bucket+"/"+key]), nil
}
