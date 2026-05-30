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
