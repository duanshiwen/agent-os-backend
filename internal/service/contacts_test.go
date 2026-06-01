package service

import (
	"errors"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestContactsCreateUpdateDeleteEmitSyncEvents(t *testing.T) {
	svc, syncRepo, user := newContactsTestService(t)

	created, event, err := svc.CreateContact(user.ID, "device-a", ContactInput{ContactID: "alice", DisplayName: "Alice", Emails: datatypes.JSONSlice[string]{"alice@example.com"}, Labels: datatypes.JSONSlice[string]{"friend"}, ClientEventID: "contact-create-1"})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	if created.ContactID != "alice" || created.Status != repository.ContactStatusActive || created.Version != 1 || created.UpdatedByDeviceID != "device-a" {
		t.Fatalf("unexpected created contact: %+v", created)
	}
	assertContactSyncEvent(t, event, user.ID, "device-a", "alice", SyncOperationCreated, "contact-create-1")
	if event.Payload["display_name"] != "Alice" || event.Payload["status"] != repository.ContactStatusActive {
		t.Fatalf("unexpected create payload: %+v", event.Payload)
	}

	updated, event, err := svc.UpdateContact(user.ID, "device-b", "alice", ContactInput{DisplayName: "Alice Zhang", Phones: datatypes.JSONSlice[string]{"+8613800000000"}, ClientEventID: "contact-update-1"})
	if err != nil {
		t.Fatalf("update contact: %v", err)
	}
	if updated.DisplayName != "Alice Zhang" || updated.Version != 2 || updated.UpdatedByDeviceID != "device-b" {
		t.Fatalf("unexpected updated contact: %+v", updated)
	}
	assertContactSyncEvent(t, event, user.ID, "device-b", "alice", SyncOperationUpdated, "contact-update-1")

	deleted, event, err := svc.DeleteContact(user.ID, "device-c", "alice", "contact-delete-1")
	if err != nil {
		t.Fatalf("delete contact: %v", err)
	}
	if deleted.Status != repository.ContactStatusDeleted || deleted.DeletedAt == nil || deleted.Version != 3 {
		t.Fatalf("expected tombstone, got %+v", deleted)
	}
	assertContactSyncEvent(t, event, user.ID, "device-c", "alice", SyncOperationDeleted, "contact-delete-1")

	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get sync events: %v", err)
	}
	if len(events) != 3 || events[0].EventType != "contact.created" || events[1].EventType != "contact.updated" || events[2].EventType != "contact.deleted" {
		t.Fatalf("unexpected contact events: %+v", events)
	}
}

func TestContactsRejectStaleBaseVersionAndDeletedUpdate(t *testing.T) {
	svc, _, user := newContactsTestService(t)
	created, _, err := svc.CreateContact(user.ID, "device-a", ContactInput{ContactID: "alice", DisplayName: "Alice", ClientEventID: "contact-create-version"})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	stale := created.Version - 1
	_, _, err = svc.UpdateContact(user.ID, "device-b", "alice", ContactInput{DisplayName: "Stale", BaseVersion: &stale, ClientEventID: "contact-stale-update"})
	if !errors.Is(err, ErrContactConflict) {
		t.Fatalf("expected ErrContactConflict, got %v", err)
	}
	_, _, err = svc.DeleteContact(user.ID, "device-a", "alice", "contact-delete-version")
	if err != nil {
		t.Fatalf("delete contact: %v", err)
	}
	_, _, err = svc.UpdateContact(user.ID, "device-b", "alice", ContactInput{DisplayName: "After delete", ClientEventID: "contact-update-deleted"})
	if !errors.Is(err, ErrContactDeleted) {
		t.Fatalf("expected ErrContactDeleted, got %v", err)
	}
}

func TestContactsClientEventIDIsIdempotent(t *testing.T) {
	svc, syncRepo, user := newContactsTestService(t)
	_, first, err := svc.CreateContact(user.ID, "device-a", ContactInput{ContactID: "alice", DisplayName: "Alice", ClientEventID: "contact-idempotent-1"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	contact, second, err := svc.CreateContact(user.ID, "device-a", ContactInput{ContactID: "alice", DisplayName: "Alice", ClientEventID: "contact-idempotent-1"})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if contact.Version != 1 || first.ID != second.ID || first.Sequence != second.Sequence {
		t.Fatalf("expected idempotent replay, contact=%+v first=%+v second=%+v", contact, first, second)
	}
	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %+v", events)
	}
	_, _, err = svc.UpdateContact(user.ID, "device-a", "alice", ContactInput{DisplayName: "Conflict", ClientEventID: "contact-idempotent-1"})
	if !errors.Is(err, ErrSyncIdempotencyConflict) {
		t.Fatalf("expected ErrSyncIdempotencyConflict, got %v", err)
	}
}

func assertContactSyncEvent(t *testing.T, event *model.SyncEvent, userID uuid.UUID, sourceDeviceID, contactID, operation, clientEventID string) {
	t.Helper()
	if event == nil {
		t.Fatal("expected sync event")
	}
	if event.UserID != userID || event.SourceDeviceID != sourceDeviceID || event.ObjectType != SyncObjectContact || event.ObjectID != contactID || event.Operation != operation || event.ClientEventID != clientEventID {
		t.Fatalf("unexpected sync event: %+v", event)
	}
	if event.EventType != "contact."+operation || event.Payload["contact_id"] != contactID || event.Payload["object_id"] != contactID || event.Payload["updated_by_device_id"] != sourceDeviceID {
		t.Fatalf("unexpected contact sync payload/event: %+v payload=%+v", event, event.Payload)
	}
}

func newContactsTestService(t *testing.T) (*ContactsService, *repository.SyncRepo, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Contact{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &model.User{PubKeyEd25519: "contact-pubkey", DisplayName: "Contact User", Status: "active"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	syncRepo := repository.NewSyncRepo(db)
	syncSvc := NewSyncService(syncRepo, &captureHub{})
	contactRepo := repository.NewContactsRepo(db)
	return NewContactsService(contactRepo, syncSvc), syncRepo, user
}
