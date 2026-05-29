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

func TestServerConnectionsAddEmitsServerAddedSyncEvent(t *testing.T) {
	svc, syncRepo, user, _ := newServerConnectionsTestService(t)

	conn, event, err := svc.AddServer(user.ID, "device-a", AddServerConnectionRequest{ServerID: "primary", Name: "Primary", BaseURL: "https://agent.example", Status: "active", Config: datatypes.JSONMap{"region": "cn"}, ClientEventID: "server-add-1"})
	if err != nil {
		t.Fatalf("add server: %v", err)
	}
	if conn.ServerID != "primary" || conn.Name != "Primary" || conn.BaseURL != "https://agent.example" || conn.Status != "active" || conn.UpdatedByDeviceID != "device-a" {
		t.Fatalf("unexpected connection: %+v", conn)
	}
	assertServerSyncEvent(t, event, user.ID, "device-a", conn.ID.String(), "primary", SyncOperationAdded, "server-add-1")

	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "server.added" {
		t.Fatalf("expected one server.added event, got %+v", events)
	}
}

func TestServerConnectionsUpdateEmitsServerUpdatedSyncEvent(t *testing.T) {
	svc, _, user, _ := newServerConnectionsTestService(t)
	conn, _, err := svc.AddServer(user.ID, "device-a", AddServerConnectionRequest{ServerID: "primary", Name: "Primary", BaseURL: "https://agent.example", Status: "active"})
	if err != nil {
		t.Fatalf("add server: %v", err)
	}

	updated, event, err := svc.UpdateServer(user.ID, "device-b", conn.ID, UpdateServerConnectionRequest{Name: stringPtr("Primary Updated"), Status: stringPtr("disabled"), Config: datatypes.JSONMap{"region": "us"}, ClientEventID: "server-update-1"})
	if err != nil {
		t.Fatalf("update server: %v", err)
	}
	if updated.Name != "Primary Updated" || updated.Status != "disabled" || updated.Config["region"] != "us" || updated.UpdatedByDeviceID != "device-b" {
		t.Fatalf("unexpected updated connection: %+v", updated)
	}
	assertServerSyncEvent(t, event, user.ID, "device-b", conn.ID.String(), "primary", SyncOperationUpdated, "server-update-1")
}

func TestServerConnectionsRemoveEmitsServerRemovedSyncEvent(t *testing.T) {
	svc, _, user, _ := newServerConnectionsTestService(t)
	conn, _, err := svc.AddServer(user.ID, "device-a", AddServerConnectionRequest{ServerID: "primary", Name: "Primary", BaseURL: "https://agent.example", Status: "active"})
	if err != nil {
		t.Fatalf("add server: %v", err)
	}

	event, err := svc.RemoveServer(user.ID, "device-b", conn.ID, "server-remove-1")
	if err != nil {
		t.Fatalf("remove server: %v", err)
	}
	assertServerSyncEvent(t, event, user.ID, "device-b", conn.ID.String(), "primary", SyncOperationRemoved, "server-remove-1")
	if event.Payload["removed"] != true {
		t.Fatalf("expected tombstone removed=true, got %+v", event.Payload)
	}
	listed, err := svc.ListServers(user.ID)
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected connection deleted, got %+v", listed)
	}
}

func TestServerConnectionsClientEventIDIsIdempotent(t *testing.T) {
	svc, syncRepo, user, _ := newServerConnectionsTestService(t)

	_, first, err := svc.AddServer(user.ID, "device-a", AddServerConnectionRequest{ServerID: "primary", Name: "Primary", BaseURL: "https://agent.example", ClientEventID: "server-idempotent-1"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, second, err := svc.AddServer(user.ID, "device-a", AddServerConnectionRequest{ServerID: "primary", Name: "Primary", BaseURL: "https://agent.example", ClientEventID: "server-idempotent-1"})
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if first.ID != second.ID || first.Sequence != second.Sequence {
		t.Fatalf("expected same event for idempotent replay, first=%+v second=%+v", first, second)
	}
	events, err := syncRepo.GetEventsSince(user.ID, "", 0, 100)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one persisted event, got %+v", events)
	}
}

func TestServerConnectionsClientEventIDConflictIsRejected(t *testing.T) {
	svc, _, user, _ := newServerConnectionsTestService(t)

	_, _, err := svc.AddServer(user.ID, "device-a", AddServerConnectionRequest{ServerID: "primary", Name: "Primary", BaseURL: "https://agent.example", ClientEventID: "server-conflict-1"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, _, err = svc.AddServer(user.ID, "device-a", AddServerConnectionRequest{ServerID: "secondary", Name: "Secondary", BaseURL: "https://agent2.example", ClientEventID: "server-conflict-1"})
	if !errors.Is(err, ErrSyncIdempotencyConflict) {
		t.Fatalf("expected ErrSyncIdempotencyConflict, got %v", err)
	}
}

func TestServerConnectionsRejectsCrossUserMutation(t *testing.T) {
	svc, _, user, other := newServerConnectionsTestService(t)
	conn, _, err := svc.AddServer(user.ID, "device-a", AddServerConnectionRequest{ServerID: "primary", Name: "Primary", BaseURL: "https://agent.example"})
	if err != nil {
		t.Fatalf("add server: %v", err)
	}

	_, _, err = svc.UpdateServer(other.ID, "device-b", conn.ID, UpdateServerConnectionRequest{Name: stringPtr("Stolen")})
	if err == nil {
		t.Fatal("expected cross-user update to fail")
	}
	if _, err := svc.RemoveServer(other.ID, "device-b", conn.ID, ""); err == nil {
		t.Fatal("expected cross-user remove to fail")
	}
}

func assertServerSyncEvent(t *testing.T, event *model.SyncEvent, userID uuid.UUID, sourceDeviceID, connectionID, serverID, operation, clientEventID string) {
	t.Helper()
	if event == nil {
		t.Fatal("expected sync event")
	}
	if event.UserID != userID || event.SourceDeviceID != sourceDeviceID || event.ObjectType != SyncObjectServer || event.ObjectID != connectionID || event.Operation != operation || event.ClientEventID != clientEventID {
		t.Fatalf("unexpected sync event: %+v", event)
	}
	if event.EventType != "server."+operation || event.Payload["connection_id"] != connectionID || event.Payload["server_id"] != serverID || event.Payload["object_id"] != connectionID || event.Payload["updated_by_device_id"] != sourceDeviceID {
		t.Fatalf("unexpected server sync payload/event: %+v payload=%+v", event, event.Payload)
	}
}

func newServerConnectionsTestService(t *testing.T) (*ServerConnectionsService, *repository.SyncRepo, *model.User, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserServerConnection{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &model.User{PubKeyEd25519: "server-connections-pubkey", DisplayName: "Server User", Status: "active"}
	other := &model.User{PubKeyEd25519: "server-connections-other-pubkey", DisplayName: "Other User", Status: "active"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	syncRepo := repository.NewSyncRepo(db)
	syncSvc := NewSyncService(syncRepo, &captureHub{})
	svc := NewServerConnectionsService(repository.NewServerConnectionsRepo(db), syncSvc)
	return svc, syncRepo, user, other
}
