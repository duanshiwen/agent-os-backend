package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

func TestKBHubStage2GovernanceModerationAndTakedown(t *testing.T) {
	svc, knowledgeRepo, _, ownerID, _ := newKBHubServiceTestEnv(t)
	adminID := uuid.New()
	reporterID := uuid.New()
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "stage2/governance", Title: "Governance", ContentMarkdown: "# Governance", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("f", 64)}); err != nil {
		t.Fatalf("create knowledge entry: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Governed KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	updated, err := svc.UpdateCollectionDeclarations(ownerID, collection.ID, UpdateKBCollectionDeclarationsInput{SourceDeclaration: "Original notes", CopyrightDeclaration: "Owned by author", ModerationMetadata: map[string]any{"risk": "low"}})
	if err != nil {
		t.Fatalf("update declarations: %v", err)
	}
	if updated.SourceDeclaration == "" || updated.CopyrightDeclaration == "" || updated.ReviewStatus != repository.KBReviewStatusPending {
		t.Fatalf("unexpected declarations: %+v", updated)
	}
	if _, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{}); err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	if _, err := svc.GetPublicCollection(collection.ID); err != nil {
		t.Fatalf("expected published collection to be public after publish approval compatibility: %v", err)
	}
	report, err := svc.ReportCollection(reporterID, collection.ID, ReportKBCollectionInput{Reason: "copyright", Detail: "Needs review"})
	if err != nil {
		t.Fatalf("report collection: %v", err)
	}
	if report.Status != "open" || report.Reason != "copyright" {
		t.Fatalf("unexpected report: %+v", report)
	}
	reports, err := svc.ListModerationReports("open", 10, 0)
	if err != nil || len(reports) != 1 {
		t.Fatalf("expected open report, got reports=%+v err=%v", reports, err)
	}
	resolved, err := svc.ResolveModerationReport(adminID, report.ID, ResolveKBModerationReportInput{Status: "resolved", Resolution: "Handled"})
	if err != nil {
		t.Fatalf("resolve report: %v", err)
	}
	if resolved.ResolvedBy == nil || *resolved.ResolvedBy != adminID || resolved.Status != "resolved" {
		t.Fatalf("unexpected resolved report: %+v", resolved)
	}
	takenDown, err := svc.ReviewCollection(adminID, collection.ID, ReviewKBCollectionInput{ReviewStatus: repository.KBReviewStatusTakedown, Reason: "copyright violation"})
	if err != nil {
		t.Fatalf("takedown collection: %v", err)
	}
	if takenDown.Status != repository.KBCollectionStatusArchived || takenDown.ReviewStatus != repository.KBReviewStatusTakedown || takenDown.TakedownAt == nil {
		t.Fatalf("unexpected takedown state: %+v", takenDown)
	}
	if _, err := svc.GetPublicCollection(collection.ID); err == nil || !strings.Contains(err.Error(), ErrKBCollectionNotFound.Error()) {
		t.Fatalf("expected public access denied after takedown, got %v", err)
	}
}

func TestKBHubStage2SnapshotLifecycleDiffAndSubscriptionExpiry(t *testing.T) {
	svc, knowledgeRepo, _, ownerID, _ := newKBHubServiceTestEnv(t)
	consumerID := uuid.New()
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "stage2/alpha", Title: "Alpha", ContentMarkdown: "# Alpha v1", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("1", 64)}); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Lifecycle KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	first, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish first: %v", err)
	}
	alpha, err := knowledgeRepo.GetByEntryID(ownerID, "stage2/alpha", false)
	if err != nil {
		t.Fatalf("get alpha: %v", err)
	}
	alpha.Title = "Alpha v2"
	alpha.ContentMarkdown = "# Alpha v2"
	alpha.ContentHash = strings.Repeat("2", 64)
	alpha.Version = 2
	if err := knowledgeRepo.UpdateActive(alpha); err != nil {
		t.Fatalf("update alpha: %v", err)
	}
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "stage2/beta", Title: "Beta", ContentMarkdown: "# Beta", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("3", 64)}); err != nil {
		t.Fatalf("create beta: %v", err)
	}
	second, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{})
	if err != nil {
		t.Fatalf("publish second: %v", err)
	}
	diff, err := svc.DiffSnapshots(ownerID, collection.ID, first.Snapshot.ID, second.Snapshot.ID)
	if err != nil {
		t.Fatalf("diff snapshots: %v", err)
	}
	if len(diff.Added) != 1 || len(diff.Changed) != 1 || diff.UnchangedCount != 0 {
		t.Fatalf("unexpected diff: %+v", diff)
	}
	archived, err := svc.ArchiveSnapshot(ownerID, collection.ID, second.Snapshot.ID)
	if err != nil {
		t.Fatalf("archive snapshot: %v", err)
	}
	if archived.Status != repository.KBSnapshotStatusArchived || archived.ArchivedAt == nil {
		t.Fatalf("unexpected archived snapshot: %+v", archived)
	}
	latest, err := svc.GetPublicCollection(collection.ID)
	if err != nil {
		t.Fatalf("public collection after archive second: %v", err)
	}
	if latest.LatestSnapshot == nil || latest.LatestSnapshot.ID != first.Snapshot.ID {
		t.Fatalf("expected latest active snapshot to fall back to first, got %+v", latest)
	}
	restored, err := svc.RestoreSnapshot(ownerID, collection.ID, second.Snapshot.ID)
	if err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	if restored.Status != repository.KBSnapshotStatusActive || restored.ArchivedAt != nil {
		t.Fatalf("unexpected restored snapshot: %+v", restored)
	}
	expiresAt := time.Now().UTC().Add(-time.Minute)
	if _, err := svc.InstallCollection(consumerID, collection.ID, InstallKBCollectionInput{ExpiresAt: &expiresAt}); err != nil {
		t.Fatalf("install expired subscription: %v", err)
	}
	expired, err := svc.ExpireSubscriptions(time.Now().UTC())
	if err != nil {
		t.Fatalf("expire subscriptions: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expected one expired subscription, got %d", expired)
	}
	if _, err := svc.CreateInstalledManifestDownloadURL(context.Background(), consumerID, collection.ID, restored.ID); err == nil || !strings.Contains(err.Error(), ErrKBSubscriptionNeeded.Error()) {
		t.Fatalf("expected access denial after expiry, got %v", err)
	}
}

func TestKBHubGovernanceObserveRecordsReviewAndPricingWithoutBlocking(t *testing.T) {
	svc, _, _, ownerID, db := newKBHubServiceTestEnv(t)
	governance := NewGovernanceService(repository.NewGovernanceRepo(db))
	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{Name: "Observe deny KB takedown", CapabilityKey: "kb.collection.review.takedown", SubjectType: GovernanceSubjectKBOperation, Effect: GovernanceDecisionDeny, Priority: 1, Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create takedown rule: %v", err)
	}
	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{Name: "Observe deny KB pricing", CapabilityKey: "kb.collection.pricing.update", SubjectType: GovernanceSubjectKBOperation, Effect: GovernanceDecisionDeny, Priority: 1, Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create pricing rule: %v", err)
	}
	svc.SetGovernanceEnforcer(NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{}))

	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Governed KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	priced, err := svc.UpdateCollectionPricing(ownerID, collection.ID, UpdateKBCollectionPricingInput{IsFree: boolPtr(false), PricingModel: "monthly", MonthlyPrice: 9900})
	if err != nil {
		t.Fatalf("observe mode should not block pricing: %v", err)
	}
	if priced.MonthlyPrice != 9900 {
		t.Fatalf("expected pricing update to proceed, got %#v", priced)
	}
	reviewed, err := svc.ReviewCollection(uuid.New(), collection.ID, ReviewKBCollectionInput{ReviewStatus: repository.KBReviewStatusTakedown, Reason: "observe"})
	if err != nil {
		t.Fatalf("observe mode should not block takedown: %v", err)
	}
	if reviewed.ReviewStatus != repository.KBReviewStatusTakedown {
		t.Fatalf("expected review update to proceed, got %#v", reviewed)
	}
}
