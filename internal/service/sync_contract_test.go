package service

import "testing"

func TestBuildSyncEventTypeBuildsStableName(t *testing.T) {
	eventType, err := BuildSyncEventType(SyncObjectProfile, SyncOperationUpdated)
	if err != nil {
		t.Fatalf("build sync event type: %v", err)
	}
	if eventType != "profile.updated" {
		t.Fatalf("expected profile.updated, got %q", eventType)
	}
}

func TestValidateSyncEventRejectsUnsupportedObjectType(t *testing.T) {
	if err := ValidateSyncEvent("unknown", SyncOperationUpdated); err == nil {
		t.Fatal("expected unsupported object type to be rejected")
	}
}

func TestValidateSyncEventRejectsUnsupportedOperationForObject(t *testing.T) {
	if err := ValidateSyncEvent(SyncObjectProfile, SyncOperationCreated); err == nil {
		t.Fatal("expected unsupported profile operation to be rejected")
	}
}

func TestValidateSyncEventAllowsMessageCreated(t *testing.T) {
	if err := ValidateSyncEvent(SyncObjectMessage, SyncOperationCreated); err != nil {
		t.Fatalf("expected message.created to be allowed: %v", err)
	}
}

func TestValidateSyncEventAllowsContactLifecycle(t *testing.T) {
	for _, operation := range []string{SyncOperationCreated, SyncOperationUpdated, SyncOperationDeleted} {
		if err := ValidateSyncEvent(SyncObjectContact, operation); err != nil {
			t.Fatalf("expected contact.%s to be allowed: %v", operation, err)
		}
	}
	eventType, err := BuildSyncEventType(SyncObjectContact, SyncOperationCreated)
	if err != nil {
		t.Fatalf("build contact event type: %v", err)
	}
	if eventType != "contact.created" {
		t.Fatalf("expected contact.created, got %q", eventType)
	}
	families := SyncSupportedObjectFamilies()
	if got := families[SyncObjectContact]; len(got) != 3 || got[0] != SyncOperationCreated || got[1] != SyncOperationUpdated || got[2] != SyncOperationDeleted {
		t.Fatalf("unexpected contact capabilities: %+v", got)
	}
}
