package service

import (
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuditServiceRecordAndList(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewAuditService(repository.NewAuditRepo(db))
	actorID := uuid.New()

	if _, err := svc.Record(RecordAuditEventInput{ActorUserID: &actorID, ActorDeviceID: "device-1", Action: AuditActionAdmissionPolicyUpdated, ResourceType: "server_admission", ResourceID: "default", Metadata: map[string]any{"policy_type": "approval"}}); err != nil {
		t.Fatalf("record audit: %v", err)
	}
	if _, err := svc.Record(RecordAuditEventInput{Action: AuditActionDevicePairingStarted, ResourceType: "device_pairing_session", ResourceID: uuid.NewString(), Outcome: AuditOutcomeFailure}); err != nil {
		t.Fatalf("record second audit: %v", err)
	}

	page, err := svc.List(ListAuditEventsInput{ActorUserID: &actorID, Limit: 10})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one actor event, got total=%d len=%d", page.Total, len(page.Items))
	}
	item := page.Items[0]
	if item.Action != AuditActionAdmissionPolicyUpdated || item.ResourceType != "server_admission" || item.ResourceID != "default" || item.Outcome != AuditOutcomeSuccess {
		t.Fatalf("unexpected audit event: %+v", item)
	}
	if item.Metadata["policy_type"] != "approval" {
		t.Fatalf("expected metadata to be persisted, got %+v", item.Metadata)
	}

	failures, err := svc.List(ListAuditEventsInput{Outcome: AuditOutcomeFailure, Limit: 10})
	if err != nil {
		t.Fatalf("list failures: %v", err)
	}
	if failures.Total != 1 || failures.Items[0].Action != AuditActionDevicePairingStarted {
		t.Fatalf("unexpected failures page: %+v", failures)
	}
}

func TestAuditServiceRecordsTamperEvidentHashChain(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewAuditService(repository.NewAuditRepo(db))

	first, err := svc.Record(RecordAuditEventInput{Action: AuditActionSAGEPluginCreated, ResourceType: "sage_plugin", ResourceID: "plugin-1", Metadata: map[string]any{"version": "1.0.0"}})
	if err != nil {
		t.Fatalf("record first audit: %v", err)
	}
	second, err := svc.Record(RecordAuditEventInput{Action: AuditActionSAGEPluginReviewed, ResourceType: "sage_plugin", ResourceID: "plugin-1", Outcome: AuditOutcomeDenied, Metadata: map[string]any{"reason": "policy_violation"}})
	if err != nil {
		t.Fatalf("record second audit: %v", err)
	}

	if first.Sequence <= 0 || second.Sequence != first.Sequence+1 {
		t.Fatalf("expected monotonic sequence, first=%d second=%d", first.Sequence, second.Sequence)
	}
	if first.PreviousHash != AuditGenesisHash {
		t.Fatalf("expected genesis previous hash, got %q", first.PreviousHash)
	}
	if second.PreviousHash != first.EventHash {
		t.Fatalf("expected second event to link to first hash, previous=%q first=%q", second.PreviousHash, first.EventHash)
	}
	if len(first.EventHash) != 64 || len(second.EventHash) != 64 {
		t.Fatalf("expected sha256 event hashes, first=%q second=%q", first.EventHash, second.EventHash)
	}
	if first.HashAlgorithm != "sha256" || second.HashAlgorithm != "sha256" {
		t.Fatalf("expected sha256 hash algorithm, first=%q second=%q", first.HashAlgorithm, second.HashAlgorithm)
	}

	verification, err := svc.VerifyHashChain(VerifyAuditHashChainInput{})
	if err != nil {
		t.Fatalf("verify chain: %v", err)
	}
	if !verification.Valid || verification.Checked != 2 || len(verification.Breaks) != 0 {
		t.Fatalf("expected valid chain, got %+v", verification)
	}
}

func TestAuditServiceDetectsTamperedHashChain(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewAuditService(repository.NewAuditRepo(db))

	event, err := svc.Record(RecordAuditEventInput{Action: AuditActionSAGEPluginInstalled, ResourceType: "sage_plugin", ResourceID: "plugin-2", Metadata: map[string]any{"installed_by": "test"}})
	if err != nil {
		t.Fatalf("record audit: %v", err)
	}
	if err := db.Model(&model.AuditEvent{}).Where("id = ?", event.ID).Update("resource_id", "plugin-evil").Error; err != nil {
		t.Fatalf("tamper event: %v", err)
	}

	verification, err := svc.VerifyHashChain(VerifyAuditHashChainInput{})
	if err != nil {
		t.Fatalf("verify chain: %v", err)
	}
	if verification.Valid {
		t.Fatalf("expected invalid chain after tampering, got %+v", verification)
	}
	if len(verification.Breaks) != 1 || verification.Breaks[0].Reason != AuditChainBreakHashMismatch {
		t.Fatalf("expected hash mismatch break, got %+v", verification.Breaks)
	}
}
