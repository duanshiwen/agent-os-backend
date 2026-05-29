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

func TestKnowledgeEntriesCreateEmitsKnowledgeCreatedSyncEvent(t *testing.T) {
	svc, syncRepo, user := newKnowledgeEntriesTestService(t)

	entry, event, err := svc.CreateEntry(user.ID, "device-a", KnowledgeEntryInput{
		EntryID:         "notes/alpha",
		Title:           "Alpha",
		ContentMarkdown: "# Alpha\n\nBody",
		Summary:         "summary",
		Tags:            datatypes.JSONSlice[string]{"agentos"},
		Metadata:        datatypes.JSONMap{"category": "notes"},
		SourceURI:       "file://alpha.md",
		ClientEventID:   "knowledge-create-1",
	})
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if entry.EntryID != "notes/alpha" || entry.Status != repository.KnowledgeEntryStatusActive || entry.Version != 1 || entry.ContentHash == "" || entry.UpdatedByDeviceID != "device-a" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	assertKnowledgeSyncEvent(t, event, user.ID, "device-a", "notes/alpha", SyncOperationCreated, "knowledge-create-1")
	if event.Payload["title"] != "Alpha" || event.Payload["status"] != repository.KnowledgeEntryStatusActive || event.Payload["content_hash"] == "" {
		t.Fatalf("unexpected created payload: %+v", event.Payload)
	}

	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "knowledge.created" {
		t.Fatalf("expected one knowledge.created event, got %+v", events)
	}
}

func TestKnowledgeEntriesUpdateEmitsKnowledgeUpdatedSyncEvent(t *testing.T) {
	svc, _, user := newKnowledgeEntriesTestService(t)
	_, _, err := svc.CreateEntry(user.ID, "device-a", KnowledgeEntryInput{EntryID: "notes/alpha", Title: "Alpha", ContentMarkdown: "v1", ClientEventID: "create-before-update"})
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}

	entry, event, err := svc.UpdateEntry(user.ID, "device-b", "notes/alpha", KnowledgeEntryInput{
		Title:           "Alpha v2",
		ContentMarkdown: "v2",
		Summary:         "summary v2",
		Tags:            datatypes.JSONSlice[string]{"sync"},
		Metadata:        datatypes.JSONMap{"stage": "m2.2"},
		ClientEventID:   "knowledge-update-1",
	})
	if err != nil {
		t.Fatalf("update entry: %v", err)
	}
	if entry.Title != "Alpha v2" || entry.Version != 2 || entry.UpdatedByDeviceID != "device-b" {
		t.Fatalf("unexpected updated entry: %+v", entry)
	}
	assertKnowledgeSyncEvent(t, event, user.ID, "device-b", "notes/alpha", SyncOperationUpdated, "knowledge-update-1")
	if event.Payload["title"] != "Alpha v2" || event.Payload["version"] != uint64(2) {
		t.Fatalf("unexpected updated payload: %+v", event.Payload)
	}
}

func TestKnowledgeEntriesDeleteCreatesTombstoneAndEmitsKnowledgeDeletedSyncEvent(t *testing.T) {
	svc, _, user := newKnowledgeEntriesTestService(t)
	_, _, err := svc.CreateEntry(user.ID, "device-a", KnowledgeEntryInput{EntryID: "notes/alpha", Title: "Alpha", ContentMarkdown: "v1", ClientEventID: "create-before-delete"})
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}

	entry, event, err := svc.DeleteEntry(user.ID, "device-a", "notes/alpha", "knowledge-delete-1")
	if err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	if entry.Status != repository.KnowledgeEntryStatusDeleted || entry.DeletedAt == nil || entry.Version != 2 {
		t.Fatalf("expected tombstone, got %+v", entry)
	}
	assertKnowledgeSyncEvent(t, event, user.ID, "device-a", "notes/alpha", SyncOperationDeleted, "knowledge-delete-1")
	if event.Payload["status"] != repository.KnowledgeEntryStatusDeleted || event.Payload["deleted_at"] == nil {
		t.Fatalf("unexpected delete payload: %+v", event.Payload)
	}

	active, err := svc.ListEntries(user.ID, false)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active entries after tombstone, got %+v", active)
	}
	all, err := svc.ListEntries(user.ID, true)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 1 || all[0].Status != repository.KnowledgeEntryStatusDeleted {
		t.Fatalf("expected tombstone in include_deleted list, got %+v", all)
	}
}

func TestKnowledgeEntriesDeletedEntryCannotBeUpdated(t *testing.T) {
	svc, _, user := newKnowledgeEntriesTestService(t)
	_, _, err := svc.CreateEntry(user.ID, "device-a", KnowledgeEntryInput{EntryID: "notes/alpha", Title: "Alpha", ContentMarkdown: "v1", ClientEventID: "create-before-deleted-update"})
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	_, _, err = svc.DeleteEntry(user.ID, "device-a", "notes/alpha", "delete-before-update")
	if err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	_, _, err = svc.UpdateEntry(user.ID, "device-b", "notes/alpha", KnowledgeEntryInput{Title: "Revive", ContentMarkdown: "revive", ClientEventID: "update-deleted"})
	if !errors.Is(err, ErrKnowledgeEntryDeleted) {
		t.Fatalf("expected ErrKnowledgeEntryDeleted, got %v", err)
	}
}

func TestKnowledgeEntriesClientEventIDIsIdempotent(t *testing.T) {
	svc, syncRepo, user := newKnowledgeEntriesTestService(t)
	_, first, err := svc.CreateEntry(user.ID, "device-a", KnowledgeEntryInput{EntryID: "notes/alpha", Title: "Alpha", ContentMarkdown: "v1", ClientEventID: "knowledge-idempotent-1"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, second, err := svc.CreateEntry(user.ID, "device-a", KnowledgeEntryInput{EntryID: "notes/alpha", Title: "Alpha", ContentMarkdown: "v1", ClientEventID: "knowledge-idempotent-1"})
	if err != nil {
		t.Fatalf("second create idempotent replay: %v", err)
	}
	if first.ID != second.ID || first.Sequence != second.Sequence {
		t.Fatalf("expected same event on idempotent replay, first=%+v second=%+v", first, second)
	}
	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one persisted event after idempotent replay, got %+v", events)
	}
}

func assertKnowledgeSyncEvent(t *testing.T, event *model.SyncEvent, userID uuid.UUID, sourceDeviceID, entryID, operation, clientEventID string) {
	t.Helper()
	if event == nil {
		t.Fatal("expected sync event")
	}
	if event.UserID != userID || event.SourceDeviceID != sourceDeviceID || event.ObjectType != SyncObjectKnowledge || event.ObjectID != entryID || event.Operation != operation || event.ClientEventID != clientEventID {
		t.Fatalf("unexpected sync event: %+v", event)
	}
	if event.EventType != "knowledge."+operation || event.Payload["entry_id"] != entryID || event.Payload["object_id"] != entryID || event.Payload["updated_by_device_id"] != sourceDeviceID {
		t.Fatalf("unexpected knowledge sync payload/event: %+v payload=%+v", event, event.Payload)
	}
}

func newKnowledgeEntriesTestService(t *testing.T) (*KnowledgeEntriesService, *repository.SyncRepo, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserKnowledgeEntry{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &model.User{PubKeyEd25519: "knowledge-pubkey", DisplayName: "Knowledge User", Status: "active"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	syncRepo := repository.NewSyncRepo(db)
	syncSvc := NewSyncService(syncRepo, &captureHub{})
	svc := NewKnowledgeEntriesService(repository.NewKnowledgeEntriesRepo(db), syncSvc)
	return svc, syncRepo, user
}
