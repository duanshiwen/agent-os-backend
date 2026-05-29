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
