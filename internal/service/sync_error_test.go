package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

func TestValidateSyncEventReturnsUnsupportedSyncEventError(t *testing.T) {
	if err := ValidateSyncEvent("unknown", SyncOperationUpdated); !errors.Is(err, ErrUnsupportedSyncEvent) {
		t.Fatalf("expected ErrUnsupportedSyncEvent for unsupported object type, got %v", err)
	}
	if err := ValidateSyncEvent(SyncObjectProfile, SyncOperationCreated); !errors.Is(err, ErrUnsupportedSyncEvent) {
		t.Fatalf("expected ErrUnsupportedSyncEvent for unsupported operation, got %v", err)
	}
}

func TestRecordEnvelopeConflictReturnsIdempotencyConflictError(t *testing.T) {
	svc, _ := newSyncTestService(t)
	userID := uuid.New()

	_, err := svc.RecordEnvelope(SyncEnvelope{
		UserID:         userID,
		SourceDeviceID: "device-a",
		ObjectType:     SyncObjectProfile,
		ObjectID:       "profile-a",
		Operation:      SyncOperationUpdated,
		ClientEventID:  "client-event-1",
		Payload:        datatypes.JSONMap{"display_name": "Alice"},
	})
	if err != nil {
		t.Fatalf("record first envelope: %v", err)
	}

	_, err = svc.RecordEnvelope(SyncEnvelope{
		UserID:         userID,
		SourceDeviceID: "device-a",
		ObjectType:     SyncObjectProfile,
		ObjectID:       "profile-b",
		Operation:      SyncOperationUpdated,
		ClientEventID:  "client-event-1",
		Payload:        datatypes.JSONMap{"display_name": "Alice"},
	})
	if !errors.Is(err, ErrSyncIdempotencyConflict) {
		t.Fatalf("expected ErrSyncIdempotencyConflict, got %v", err)
	}
}
