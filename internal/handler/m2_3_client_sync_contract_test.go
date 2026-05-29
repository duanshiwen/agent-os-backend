package handler_test

import (
	"net/http"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type m23KnowledgeProjection struct {
	entries map[string]m23KnowledgeEntry
	cursor  uint64
}

type m23KnowledgeEntry struct {
	EntryID     string
	Title       string
	Status      string
	Version     uint64
	ContentHash string
}

func newM23KnowledgeProjection() *m23KnowledgeProjection {
	return &m23KnowledgeProjection{entries: map[string]m23KnowledgeEntry{}}
}

func (p *m23KnowledgeProjection) apply(t *testing.T, events []model.SyncEvent) {
	t.Helper()
	for _, event := range events {
		if event.Sequence <= p.cursor {
			continue
		}
		if event.ObjectType != service.SyncObjectKnowledge {
			t.Fatalf("unexpected object type in M2.3 knowledge projection: %+v", event)
		}
		entryID, _ := event.Payload["entry_id"].(string)
		status, _ := event.Payload["status"].(string)
		title, _ := event.Payload["title"].(string)
		contentHash, _ := event.Payload["content_hash"].(string)
		versionFloat, ok := event.Payload["version"].(float64)
		if !ok || entryID == "" || status == "" || contentHash == "" {
			t.Fatalf("invalid knowledge payload for client projection: %+v", event.Payload)
		}
		incoming := m23KnowledgeEntry{EntryID: entryID, Title: title, Status: status, Version: uint64(versionFloat), ContentHash: contentHash}
		local, exists := p.entries[entryID]
		switch {
		case !exists:
			p.entries[entryID] = incoming
		case incoming.Version > local.Version:
			p.entries[entryID] = incoming
		case incoming.Version == local.Version:
			if incoming.Status != local.Status || incoming.ContentHash != local.ContentHash {
				t.Fatalf("same-version knowledge mismatch: local=%+v incoming=%+v", local, incoming)
			}
		case incoming.Version < local.Version:
			// stale event; ignore
		}
		p.cursor = event.Sequence
	}
}

func TestM23ClientKnowledgeSyncConsumerFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	aliceA := env.verifyNewUser(t, "m23-device-a", "m23-pubkey")
	start := env.startPairing(t, aliceA.AccessToken)
	env.claimPairing(t, start.QRPayload, "m23-device-b", "m23-device-b-pubkey")
	aliceB := env.verifyExistingUser(t, "m23-device-b", "m23-pubkey")
	deviceBProjection := newM23KnowledgeProjection()

	var created model.UserKnowledgeEntry
	env.doJSON(t, http.MethodPost, "/api/v1/knowledge/entries", aliceA.AccessToken, map[string]any{
		"entry_id":         "notes/m23",
		"title":            "M2.3",
		"content_markdown": "# M2.3",
		"summary":          "client sync contract",
		"client_event_id":  "m23-create-1",
	}, http.StatusCreated, &created)
	if created.Version != 1 || created.EntryID != "notes/m23" {
		t.Fatalf("unexpected created knowledge entry: %+v", created)
	}

	createPull := env.getSyncPull(t, "/api/v1/sync/events?limit=100", aliceB.AccessToken)
	if len(createPull.Events) != 1 || createPull.Events[0].EventType != "knowledge.created" || createPull.NextAfterSequence != createPull.Events[0].Sequence {
		t.Fatalf("expected device B cursor pull to receive knowledge.created, got %+v", createPull)
	}
	deviceBProjection.apply(t, createPull.Events)
	env.ackSyncEvents(t, aliceB.AccessToken, deviceBProjection.cursor)
	if got := deviceBProjection.entries["notes/m23"]; got.Version != 1 || got.Status != "active" || got.Title != "M2.3" {
		t.Fatalf("unexpected device B projection after create: %+v", got)
	}

	var replay model.UserKnowledgeEntry
	env.doJSON(t, http.MethodPost, "/api/v1/knowledge/entries", aliceA.AccessToken, map[string]any{
		"entry_id":         "notes/m23",
		"title":            "M2.3",
		"content_markdown": "# M2.3",
		"summary":          "client sync contract",
		"client_event_id":  "m23-create-1",
	}, http.StatusCreated, &replay)
	if replay.ID != created.ID || replay.Version != created.Version {
		t.Fatalf("expected idempotent create replay to return original entry, got replay=%+v created=%+v", replay, created)
	}
	if events := env.getSyncEvents(t, aliceB.AccessToken, 100); len(events) != 0 {
		t.Fatalf("expected idempotent retry to allocate no new sync event after ack, got %+v", events)
	}

	var updated model.UserKnowledgeEntry
	env.doJSON(t, http.MethodPut, "/api/v1/knowledge/entries/notes/m23", aliceA.AccessToken, map[string]any{
		"title":            "M2.3 v2",
		"content_markdown": "# M2.3 v2",
		"summary":          "client sync contract v2",
		"client_event_id":  "m23-update-1",
		"base_version":     created.Version,
	}, http.StatusOK, &updated)
	if updated.Version != 2 || updated.Title != "M2.3 v2" {
		t.Fatalf("unexpected updated knowledge entry: %+v", updated)
	}
	updatePull := env.getSyncPull(t, "/api/v1/sync/events?limit=100", aliceB.AccessToken)
	if len(updatePull.Events) != 1 || updatePull.Events[0].EventType != "knowledge.updated" {
		t.Fatalf("expected device B cursor pull to receive knowledge.updated, got %+v", updatePull)
	}
	deviceBProjection.apply(t, updatePull.Events)
	env.ackSyncEvents(t, aliceB.AccessToken, deviceBProjection.cursor)
	if got := deviceBProjection.entries["notes/m23"]; got.Version != 2 || got.Title != "M2.3 v2" || got.Status != "active" {
		t.Fatalf("unexpected device B projection after update: %+v", got)
	}

	conflict := env.doRawJSON(t, http.MethodPut, "/api/v1/knowledge/entries/notes/m23", aliceB.AccessToken, map[string]any{
		"title":            "M2.3 stale",
		"content_markdown": "# stale",
		"summary":          "stale",
		"client_event_id":  "m23-stale-update-1",
		"base_version":     created.Version,
	}, http.StatusConflict)
	if conflict.Message == "" {
		t.Fatalf("expected stale base_version conflict message, got %+v", conflict)
	}
	if events := env.getSyncEventsAfter(t, aliceA.AccessToken, updatePull.Events[0].Sequence, 100); len(events) != 0 {
		t.Fatalf("expected stale update to create no sync event after latest known sequence, got %+v", events)
	}

	var deleted model.UserKnowledgeEntry
	env.doJSON(t, http.MethodDelete, "/api/v1/knowledge/entries/notes/m23", aliceA.AccessToken, map[string]any{
		"client_event_id": "m23-delete-1",
		"base_version":    updated.Version,
	}, http.StatusOK, &deleted)
	if deleted.Version != 3 || deleted.Status != "deleted" || deleted.DeletedAt == nil {
		t.Fatalf("unexpected deleted knowledge entry: %+v", deleted)
	}
	deletePull := env.getSyncPull(t, "/api/v1/sync/events?limit=100", aliceB.AccessToken)
	if len(deletePull.Events) != 1 || deletePull.Events[0].EventType != "knowledge.deleted" {
		t.Fatalf("expected device B cursor pull to receive knowledge.deleted, got %+v", deletePull)
	}
	deviceBProjection.apply(t, deletePull.Events)
	env.ackSyncEvents(t, aliceB.AccessToken, deviceBProjection.cursor)
	if got := deviceBProjection.entries["notes/m23"]; got.Version != 3 || got.Status != "deleted" {
		t.Fatalf("expected device B projection tombstone after delete, got %+v", got)
	}
	if events := env.getSyncEvents(t, aliceB.AccessToken, 100); len(events) != 0 {
		t.Fatalf("expected no events after final ack, got %+v", events)
	}
}
