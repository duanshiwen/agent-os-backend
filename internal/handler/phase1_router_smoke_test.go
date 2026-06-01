package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/router"
	"github.com/agent-os/backend/internal/service"
	"github.com/agent-os/backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPhase1RouterSmokeAuthConversationSyncAndQRPairing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "alice-device-1", "alice-pubkey")
	bob := env.verifyNewUser(t, "bob-device-1", "bob-pubkey")

	conv := env.createPrivateConversation(t, alice.AccessToken, bob.User.ID)
	delivery, err := env.msgSvc.SendMessage(alice.User.ID, &service.SendMessageRequest{
		ConversationID: conv.ID,
		Content:        "hello router smoke",
	})
	if err != nil {
		t.Fatalf("send message via service setup: %v", err)
	}

	bobEvents := env.getSyncEvents(t, bob.AccessToken, 100)
	if len(bobEvents) != 2 || bobEvents[0].EventType != "conversation.created" || bobEvents[1].EventType != "message.created" || bobEvents[1].Payload["message_id"] != delivery.Message.ID.String() {
		t.Fatalf("expected bob conversation.created and message.created events, got %+v", bobEvents)
	}
	env.ackSyncEvents(t, bob.AccessToken, bobEvents[1].Sequence)
	bobEvents = env.getSyncEvents(t, bob.AccessToken, 100)
	if len(bobEvents) != 0 {
		t.Fatalf("expected no bob sync events after ack, got %+v", bobEvents)
	}

	aliceProfile := env.updateProfile(t, alice.AccessToken, "Alice Router Smoke")
	if aliceProfile.ID != alice.User.ID || aliceProfile.DisplayName != "Alice Router Smoke" {
		t.Fatalf("unexpected updated profile: %+v", aliceProfile)
	}
	aliceEvents := env.getSyncEventsAfter(t, alice.AccessToken, 2, 100)
	if len(aliceEvents) != 1 || aliceEvents[0].EventType != "profile.updated" || aliceEvents[0].ObjectType != service.SyncEventProfile || aliceEvents[0].ObjectID != alice.User.ID.String() || aliceEvents[0].Operation != service.SyncActionUpdated || aliceEvents[0].SourceDeviceID != alice.Device.DeviceID {
		t.Fatalf("expected alice profile.updated sync event, got %+v", aliceEvents)
	}
	if aliceEvents[0].Payload["display_name"] != "Alice Router Smoke" {
		t.Fatalf("unexpected profile sync payload: %+v", aliceEvents[0].Payload)
	}

	start := env.startPairing(t, alice.AccessToken)
	paired := env.claimPairing(t, start.QRPayload, "alice-device-2", "alice-device-2-pubkey")
	if paired.UserID != alice.User.ID || paired.DeviceID != "alice-device-2" {
		t.Fatalf("unexpected paired device: %+v", paired)
	}
}

func TestSkillSettingsEnableEmitsPullableSyncEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	aliceA := env.verifyNewUser(t, "skill-device-a", "skill-pubkey")
	start := env.startPairing(t, aliceA.AccessToken)
	env.claimPairing(t, start.QRPayload, "skill-device-b", "skill-device-b-pubkey")
	aliceB := env.verifyExistingUser(t, "skill-device-b", "skill-pubkey")

	var setting model.UserSkillSetting
	env.doJSON(t, http.MethodPost, "/api/v1/skills/settings/superpowers/enable", aliceA.AccessToken, map[string]any{"client_event_id": "skill-router-enable-1"}, http.StatusOK, &setting)
	if !setting.Enabled || setting.SkillID != "superpowers" || setting.UpdatedByDeviceID != "skill-device-a" {
		t.Fatalf("unexpected skill setting response: %+v", setting)
	}

	events := env.getSyncEvents(t, aliceB.AccessToken, 100)
	if len(events) != 1 || events[0].EventType != "skill.enabled" || events[0].ObjectType != service.SyncObjectSkill || events[0].ObjectID != "superpowers" || events[0].Operation != service.SyncOperationEnabled || events[0].SourceDeviceID != "skill-device-a" || events[0].ClientEventID != "skill-router-enable-1" {
		t.Fatalf("expected device B to pull skill.enabled event, got %+v", events)
	}
	if events[0].Payload["skill_id"] != "superpowers" || events[0].Payload["enabled"] != true {
		t.Fatalf("unexpected skill sync payload: %+v", events[0].Payload)
	}
}

func TestAgentSettingsUpdateEmitsPullableSyncEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	aliceA := env.verifyNewUser(t, "agent-device-a", "agent-pubkey")
	start := env.startPairing(t, aliceA.AccessToken)
	env.claimPairing(t, start.QRPayload, "agent-device-b", "agent-device-b-pubkey")
	aliceB := env.verifyExistingUser(t, "agent-device-b", "agent-pubkey")

	var setting model.UserAgentSetting
	env.doJSON(t, http.MethodPut, "/api/v1/agents/settings/default", aliceA.AccessToken, map[string]any{
		"display_name":    "Assistant",
		"config":          map[string]any{"model": "gpt-4.1"},
		"client_event_id": "agent-router-update-1",
	}, http.StatusOK, &setting)
	if setting.AgentID != "default" || setting.DisplayName != "Assistant" || setting.UpdatedByDeviceID != "agent-device-a" {
		t.Fatalf("unexpected agent setting response: %+v", setting)
	}

	events := env.getSyncEvents(t, aliceB.AccessToken, 100)
	if len(events) != 1 || events[0].EventType != "agent.updated" || events[0].ObjectType != service.SyncObjectAgent || events[0].ObjectID != "default" || events[0].Operation != service.SyncOperationUpdated || events[0].SourceDeviceID != "agent-device-a" || events[0].ClientEventID != "agent-router-update-1" {
		t.Fatalf("expected device B to pull agent.updated event, got %+v", events)
	}
	if events[0].Payload["agent_id"] != "default" || events[0].Payload["display_name"] != "Assistant" {
		t.Fatalf("unexpected agent sync payload: %+v", events[0].Payload)
	}
}

func TestServerConnectionsAddEmitsPullableSyncEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	aliceA := env.verifyNewUser(t, "server-device-a", "server-pubkey")
	start := env.startPairing(t, aliceA.AccessToken)
	env.claimPairing(t, start.QRPayload, "server-device-b", "server-device-b-pubkey")
	aliceB := env.verifyExistingUser(t, "server-device-b", "server-pubkey")

	var connection model.UserServerConnection
	env.doJSON(t, http.MethodPost, "/api/v1/servers", aliceA.AccessToken, map[string]any{
		"server_id":       "primary",
		"name":            "Primary",
		"base_url":        "https://agent.example",
		"status":          "active",
		"config":          map[string]any{"region": "cn"},
		"client_event_id": "server-router-add-1",
	}, http.StatusCreated, &connection)
	if connection.ID == uuid.Nil || connection.ServerID != "primary" || connection.UpdatedByDeviceID != "server-device-a" {
		t.Fatalf("unexpected server connection response: %+v", connection)
	}

	events := env.getSyncEvents(t, aliceB.AccessToken, 100)
	if len(events) != 1 || events[0].EventType != "server.added" || events[0].ObjectType != service.SyncObjectServer || events[0].ObjectID != connection.ID.String() || events[0].Operation != service.SyncOperationAdded || events[0].SourceDeviceID != "server-device-a" || events[0].ClientEventID != "server-router-add-1" {
		t.Fatalf("expected device B to pull server.added event, got %+v", events)
	}
	if events[0].Payload["server_id"] != "primary" || events[0].Payload["connection_id"] != connection.ID.String() {
		t.Fatalf("unexpected server sync payload: %+v", events[0].Payload)
	}
}

func TestContactsMutationsEmitPullableSyncEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	aliceA := env.verifyNewUser(t, "contact-device-a", "contact-pubkey")
	start := env.startPairing(t, aliceA.AccessToken)
	env.claimPairing(t, start.QRPayload, "contact-device-b", "contact-device-b-pubkey")
	aliceB := env.verifyExistingUser(t, "contact-device-b", "contact-pubkey")

	var contact model.Contact
	env.doJSON(t, http.MethodPost, "/api/v1/contacts", aliceA.AccessToken, map[string]any{"contact_id": "alice", "display_name": "Alice", "emails": []string{"alice@example.com"}, "client_event_id": "contact-router-create-1"}, http.StatusCreated, &contact)
	if contact.ContactID != "alice" || contact.DisplayName != "Alice" || contact.UpdatedByDeviceID != "contact-device-a" {
		t.Fatalf("unexpected contact response: %+v", contact)
	}

	events := env.getSyncEvents(t, aliceB.AccessToken, 100)
	if len(events) != 1 || events[0].EventType != "contact.created" || events[0].ObjectType != service.SyncObjectContact || events[0].ObjectID != "alice" || events[0].Operation != service.SyncOperationCreated || events[0].SourceDeviceID != "contact-device-a" || events[0].ClientEventID != "contact-router-create-1" {
		t.Fatalf("expected device B to pull contact.created event, got %+v", events)
	}
	if events[0].Payload["display_name"] != "Alice" || events[0].Payload["contact_id"] != "alice" {
		t.Fatalf("unexpected contact sync payload: %+v", events[0].Payload)
	}
}

func TestKnowledgeEntriesMutationsEmitPullableSyncEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	aliceA := env.verifyNewUser(t, "knowledge-device-a", "knowledge-pubkey")
	start := env.startPairing(t, aliceA.AccessToken)
	env.claimPairing(t, start.QRPayload, "knowledge-device-b", "knowledge-device-b-pubkey")
	aliceB := env.verifyExistingUser(t, "knowledge-device-b", "knowledge-pubkey")

	var created model.UserKnowledgeEntry
	env.doJSON(t, http.MethodPost, "/api/v1/knowledge/entries", aliceA.AccessToken, map[string]any{
		"entry_id":         "notes/alpha",
		"title":            "Alpha",
		"content_markdown": "# Alpha",
		"summary":          "first",
		"tags":             []string{"agentos"},
		"metadata":         map[string]any{"category": "notes"},
		"client_event_id":  "knowledge-router-create-1",
	}, http.StatusCreated, &created)
	if created.EntryID != "notes/alpha" || created.Status != "active" || created.Version != 1 || created.UpdatedByDeviceID != "knowledge-device-a" {
		t.Fatalf("unexpected created knowledge entry: %+v", created)
	}

	createEvents := env.getSyncEvents(t, aliceB.AccessToken, 100)
	if len(createEvents) != 1 || createEvents[0].EventType != "knowledge.created" || createEvents[0].ObjectType != service.SyncObjectKnowledge || createEvents[0].ObjectID != "notes/alpha" || createEvents[0].Operation != service.SyncOperationCreated || createEvents[0].SourceDeviceID != "knowledge-device-a" || createEvents[0].ClientEventID != "knowledge-router-create-1" {
		t.Fatalf("expected device B to pull knowledge.created event, got %+v", createEvents)
	}
	if createEvents[0].Payload["entry_id"] != "notes/alpha" || createEvents[0].Payload["status"] != "active" {
		t.Fatalf("unexpected knowledge created payload: %+v", createEvents[0].Payload)
	}
	env.ackSyncEvents(t, aliceB.AccessToken, createEvents[0].Sequence)

	var updated model.UserKnowledgeEntry
	env.doJSON(t, http.MethodPut, "/api/v1/knowledge/entries/notes/alpha", aliceA.AccessToken, map[string]any{
		"title":            "Alpha v2",
		"content_markdown": "# Alpha v2",
		"summary":          "second",
		"client_event_id":  "knowledge-router-update-1",
	}, http.StatusOK, &updated)
	if updated.Title != "Alpha v2" || updated.Version != 2 {
		t.Fatalf("unexpected updated knowledge entry: %+v", updated)
	}
	updateEvents := env.getSyncEvents(t, aliceB.AccessToken, 100)
	if len(updateEvents) != 1 || updateEvents[0].EventType != "knowledge.updated" || updateEvents[0].ObjectID != "notes/alpha" || updateEvents[0].Operation != service.SyncOperationUpdated {
		t.Fatalf("expected device B to pull knowledge.updated event, got %+v", updateEvents)
	}
	env.ackSyncEvents(t, aliceB.AccessToken, updateEvents[0].Sequence)

	var deleted model.UserKnowledgeEntry
	env.doJSON(t, http.MethodDelete, "/api/v1/knowledge/entries/notes/alpha", aliceA.AccessToken, map[string]any{
		"client_event_id": "knowledge-router-delete-1",
	}, http.StatusOK, &deleted)
	if deleted.Status != "deleted" || deleted.DeletedAt == nil || deleted.Version != 3 {
		t.Fatalf("unexpected deleted knowledge entry: %+v", deleted)
	}
	deleteEvents := env.getSyncEvents(t, aliceB.AccessToken, 100)
	if len(deleteEvents) != 1 || deleteEvents[0].EventType != "knowledge.deleted" || deleteEvents[0].ObjectID != "notes/alpha" || deleteEvents[0].Operation != service.SyncOperationDeleted {
		t.Fatalf("expected device B to pull knowledge.deleted event, got %+v", deleteEvents)
	}
	if deleteEvents[0].Payload["status"] != "deleted" || deleteEvents[0].Payload["deleted_at"] == nil {
		t.Fatalf("unexpected knowledge deleted payload: %+v", deleteEvents[0].Payload)
	}

	var activeEntries []model.UserKnowledgeEntry
	env.doJSON(t, http.MethodGet, "/api/v1/knowledge/entries", aliceA.AccessToken, nil, http.StatusOK, &activeEntries)
	if len(activeEntries) != 0 {
		t.Fatalf("expected no active knowledge entries after delete, got %+v", activeEntries)
	}
	var allEntries []model.UserKnowledgeEntry
	env.doJSON(t, http.MethodGet, "/api/v1/knowledge/entries?include_deleted=true", aliceA.AccessToken, nil, http.StatusOK, &allEntries)
	if len(allEntries) != 1 || allEntries[0].Status != "deleted" {
		t.Fatalf("expected tombstone in include_deleted list, got %+v", allEntries)
	}
}

func TestKnowledgeEntriesStaleBaseVersionReturnsConflictWithoutSyncEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	aliceA := env.verifyNewUser(t, "knowledge-conflict-device-a", "knowledge-conflict-pubkey")
	start := env.startPairing(t, aliceA.AccessToken)
	env.claimPairing(t, start.QRPayload, "knowledge-conflict-device-b", "knowledge-conflict-device-b-pubkey")
	aliceB := env.verifyExistingUser(t, "knowledge-conflict-device-b", "knowledge-conflict-pubkey")

	var created model.UserKnowledgeEntry
	env.doJSON(t, http.MethodPost, "/api/v1/knowledge/entries", aliceA.AccessToken, map[string]any{
		"entry_id":         "notes/conflict",
		"title":            "Conflict",
		"content_markdown": "v1",
		"client_event_id":  "knowledge-conflict-router-create-1",
	}, http.StatusCreated, &created)
	if created.Version != 1 {
		t.Fatalf("unexpected created version: %+v", created)
	}
	createEvents := env.getSyncEvents(t, aliceB.AccessToken, 100)
	if len(createEvents) != 1 || createEvents[0].EventType != "knowledge.created" {
		t.Fatalf("expected initial knowledge.created event, got %+v", createEvents)
	}
	env.ackSyncEvents(t, aliceB.AccessToken, createEvents[0].Sequence)

	conflict := env.doRawJSON(t, http.MethodPut, "/api/v1/knowledge/entries/notes/conflict", aliceA.AccessToken, map[string]any{
		"title":            "Conflict stale",
		"content_markdown": "stale",
		"client_event_id":  "knowledge-conflict-router-update-stale",
		"base_version":     0,
	}, http.StatusConflict)
	if conflict.Message == "" {
		t.Fatalf("expected conflict message, got %+v", conflict)
	}
	if events := env.getSyncEvents(t, aliceB.AccessToken, 100); len(events) != 0 {
		t.Fatalf("expected stale update to create no sync event, got %+v", events)
	}

	conflict = env.doRawJSON(t, http.MethodDelete, "/api/v1/knowledge/entries/notes/conflict", aliceA.AccessToken, map[string]any{
		"client_event_id": "knowledge-conflict-router-delete-stale",
		"base_version":    0,
	}, http.StatusConflict)
	if conflict.Message == "" {
		t.Fatalf("expected delete conflict message, got %+v", conflict)
	}
	if events := env.getSyncEvents(t, aliceB.AccessToken, 100); len(events) != 0 {
		t.Fatalf("expected stale delete to create no sync event, got %+v", events)
	}

	var current model.UserKnowledgeEntry
	env.doJSON(t, http.MethodGet, "/api/v1/knowledge/entries/notes/conflict", aliceA.AccessToken, nil, http.StatusOK, &current)
	if current.Status != "active" || current.Version != 1 || current.Title != "Conflict" {
		t.Fatalf("expected stale writes to leave entry unchanged, got %+v", current)
	}
}

func TestSyncEventsResponseIncludesStableEnvelopeFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "contract-device-1", "contract-pubkey")
	env.updateProfile(t, alice.AccessToken, "Contract Alice")

	res := env.doRawJSON(t, http.MethodGet, "/api/v1/sync/events?after_sequence=0&limit=100", alice.AccessToken, nil, http.StatusOK)
	var pull struct {
		Events            []map[string]any `json:"events"`
		NextAfterSequence float64          `json:"next_after_sequence"`
		HasMore           bool             `json:"has_more"`
		ServerTime        float64          `json:"server_time"`
		SchemaVersion     float64          `json:"schema_version"`
	}
	if err := json.Unmarshal(res.Data, &pull); err != nil {
		t.Fatalf("decode raw sync pull envelope: %v data=%s", err, string(res.Data))
	}
	if len(pull.Events) != 1 || pull.NextAfterSequence != 1 || pull.HasMore || pull.ServerTime <= 0 || pull.SchemaVersion != service.SyncSchemaVersion {
		t.Fatalf("unexpected sync pull envelope: %+v", pull)
	}
	event := pull.Events[0]
	for _, field := range []string{"id", "user_id", "device_id", "event_type", "schema_version", "object_type", "object_id", "operation", "source_device_id", "client_event_id", "payload", "timestamp", "sequence", "created_at", "updated_at"} {
		if _, ok := event[field]; !ok {
			t.Fatalf("expected sync event JSON field %q in %+v", field, event)
		}
	}
	if event["event_type"] != "profile.updated" || event["object_type"] != service.SyncEventProfile || event["operation"] != service.SyncActionUpdated || event["object_id"] != alice.User.ID.String() || event["source_device_id"] != alice.Device.DeviceID {
		t.Fatalf("unexpected sync event contract values: %+v", event)
	}
	payload, ok := event["payload"].(map[string]any)
	if !ok || payload["display_name"] != "Contract Alice" {
		t.Fatalf("unexpected sync payload contract: %+v", event["payload"])
	}
}

func TestSyncCapabilitiesEndpointReturnsContractMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "capabilities-device-1", "capabilities-pubkey")
	var capabilities struct {
		SchemaVersion             int                 `json:"schema_version"`
		MinSupportedSchemaVersion int                 `json:"min_supported_schema_version"`
		MaxSupportedSchemaVersion int                 `json:"max_supported_schema_version"`
		Pull                      map[string]any      `json:"pull"`
		Bridge                    map[string]any      `json:"bridge"`
		ObjectFamilies            map[string][]string `json:"object_families"`
		Retention                 map[string]any      `json:"retention"`
	}
	env.doJSON(t, http.MethodGet, "/api/v1/sync/capabilities", alice.AccessToken, nil, http.StatusOK, &capabilities)

	if capabilities.SchemaVersion != service.SyncSchemaVersion || capabilities.MinSupportedSchemaVersion != 1 || capabilities.MaxSupportedSchemaVersion != 1 {
		t.Fatalf("unexpected schema versions: %+v", capabilities)
	}
	if capabilities.Pull["endpoint"] != "/api/v1/sync/events" || capabilities.Pull["ack_endpoint"] != "/api/v1/sync/ack" || capabilities.Pull["max_limit"] != float64(500) {
		t.Fatalf("unexpected pull capabilities: %+v", capabilities.Pull)
	}
	if capabilities.Bridge["projection"] != "ClientReadySyncProjection" || capabilities.Bridge["ffi_symbol"] != "agentos_apply_sync_pull_response_json" {
		t.Fatalf("unexpected bridge capabilities: %+v", capabilities.Bridge)
	}
	assertContainsAll(t, capabilities.ObjectFamilies[service.SyncObjectConversation], []string{"created", "updated", "read"})
	assertContainsAll(t, capabilities.ObjectFamilies[service.SyncObjectParticipant], []string{"added", "updated", "removed"})
	assertContainsAll(t, capabilities.ObjectFamilies[service.SyncObjectMessage], []string{"created", "updated", "deleted", "reaction_added", "reaction_removed"})
	assertContainsAll(t, capabilities.ObjectFamilies[service.SyncObjectSkill], []string{"installed", "uninstalled", "enabled", "disabled", "updated"})
	if capabilities.Retention["repair_supported"] != false || capabilities.Retention["compaction_supported"] != false {
		t.Fatalf("expected repair/compaction unsupported foundation response, got %+v", capabilities.Retention)
	}
}

func assertContainsAll(t *testing.T, got []string, want []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, item := range got {
		seen[item] = true
	}
	for _, item := range want {
		if !seen[item] {
			t.Fatalf("expected %v to contain %q", got, item)
		}
	}
}

func TestSyncEventsRejectsInvalidAfterSequenceQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "bad-query-device-1", "bad-query-pubkey")
	res := env.doRawJSON(t, http.MethodGet, "/api/v1/sync/events?after_sequence=not-a-number", alice.AccessToken, nil, http.StatusBadRequest)
	if res.Code != http.StatusBadRequest || res.Message == "" {
		t.Fatalf("expected bad request response for invalid after_sequence, got %+v", res)
	}
}

func TestSyncEventsLimitQueryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "limit-device-1", "limit-pubkey")
	for _, name := range []string{"Limit Alice 1", "Limit Alice 2", "Limit Alice 3"} {
		env.updateProfile(t, alice.AccessToken, name)
	}

	limitedPull := env.getSyncPull(t, "/api/v1/sync/events?after_sequence=0&limit=2", alice.AccessToken)
	limited := limitedPull.Events
	if len(limited) != 2 {
		t.Fatalf("expected limit=2 to return 2 events, got %+v", limited)
	}
	if limited[0].Sequence != 1 || limited[1].Sequence != 2 || !limitedPull.HasMore || limitedPull.NextAfterSequence != 2 {
		t.Fatalf("expected first two events ordered by sequence with has_more, got events=%+v pull=%+v", limited, limitedPull)
	}

	fallbackZero := env.getSyncEventsAfter(t, alice.AccessToken, 0, 0)
	if len(fallbackZero) != 3 {
		t.Fatalf("expected limit=0 to fallback to default and return all 3 events, got %+v", fallbackZero)
	}
	fallbackTooLarge := env.getSyncEventsAfter(t, alice.AccessToken, 0, 501)
	if len(fallbackTooLarge) != 3 {
		t.Fatalf("expected limit>500 to fallback to default and return all 3 events, got %+v", fallbackTooLarge)
	}
}

func TestSyncAckRequiresLastSequence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "ack-device-1", "ack-pubkey")
	res := env.doRawJSON(t, http.MethodPost, "/api/v1/sync/ack", alice.AccessToken, map[string]any{}, http.StatusBadRequest)
	if res.Code != http.StatusBadRequest || res.Message == "" {
		t.Fatalf("expected bad request response for missing last_sequence, got %+v", res)
	}
}

type phase1RouterSmokeVerifier struct{}

func (v *phase1RouterSmokeVerifier) VerifyEd25519Challenge(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

type phase1RouterSmokeEnv struct {
	router *gin.Engine
	msgSvc *service.MessageService
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type authResult struct {
	AccessToken string       `json:"access_token"`
	User        model.User   `json:"user"`
	Device      model.Device `json:"device"`
	IsNewUser   bool         `json:"is_new_user"`
}

type syncPullResult struct {
	Events            []model.SyncEvent `json:"events"`
	NextAfterSequence uint64            `json:"next_after_sequence"`
	HasMore           bool              `json:"has_more"`
	ServerTime        int64             `json:"server_time"`
	SchemaVersion     int               `json:"schema_version"`
}

func newPhase1RouterSmokeEnv(t *testing.T) *phase1RouterSmokeEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.AuthChallenge{}, &model.AdmissionRequest{}, &model.ServerAdmission{}, &model.DevicePairingSession{}, &model.Conversation{}, &model.ConversationParticipant{}, &model.Message{}, &model.OfflineMessage{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}, &model.UserSkillSetting{}, &model.UserAgentSetting{}, &model.UserServerConnection{}, &model.UserKnowledgeEntry{}, &model.Contact{}, &model.SensitiveOperationConfirmation{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	hub := ws.NewHub()
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	syncRepo := repository.NewSyncRepo(db)
	msgSvc := service.NewMessageService(convRepo, userRepo)
	syncSvc := service.NewSyncService(syncRepo, hub)
	msgSvc.SetSyncService(syncSvc)

	cfg := &config.Config{
		App:       config.AppConfig{Name: "agent-os-test", Port: "0", Env: "test", AutoMigrate: false},
		JWT:       config.JWTConfig{Secret: "phase1-router-smoke-secret", Issuer: "agent-os-test", AccessTokenMins: 60},
		CORS:      config.CORSConfig{AllowOrigins: []string{"*"}},
		Admission: config.AdmissionConfig{PolicyType: "protocol"},
	}

	r := router.Setup(cfg, db, redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}), hub, userRepo, convRepo, syncRepo, msgSvc, syncSvc, &phase1RouterSmokeVerifier{}, nil)
	return &phase1RouterSmokeEnv{router: r, msgSvc: msgSvc}
}

func (e *phase1RouterSmokeEnv) verifyNewUser(t *testing.T, deviceID, pubKey string) authResult {
	t.Helper()
	var challenge struct {
		Challenge string `json:"challenge"`
		Nonce     string `json:"nonce"`
		ExpiresAt int64  `json:"expires_at"`
	}
	e.doJSON(t, http.MethodPost, "/api/v1/auth/challenge", "", map[string]any{
		"device_id":   deviceID,
		"user_pubkey": pubKey,
	}, http.StatusOK, &challenge)

	var auth authResult
	e.doJSON(t, http.MethodPost, "/api/v1/auth/verify", "", map[string]any{
		"device_id":   deviceID,
		"user_pubkey": pubKey,
		"nonce":       challenge.Nonce,
		"signature":   "valid-signature",
	}, http.StatusOK, &auth)
	if auth.AccessToken == "" || auth.User.ID == uuid.Nil || auth.Device.DeviceID != deviceID || !auth.IsNewUser {
		t.Fatalf("unexpected auth result: %+v", auth)
	}
	return auth
}

func (e *phase1RouterSmokeEnv) verifyExistingUser(t *testing.T, deviceID, pubKey string) authResult {
	t.Helper()
	var challenge struct {
		Challenge string `json:"challenge"`
		Nonce     string `json:"nonce"`
		ExpiresAt int64  `json:"expires_at"`
	}
	e.doJSON(t, http.MethodPost, "/api/v1/auth/challenge", "", map[string]any{
		"device_id":   deviceID,
		"user_pubkey": pubKey,
	}, http.StatusOK, &challenge)

	var auth authResult
	e.doJSON(t, http.MethodPost, "/api/v1/auth/verify", "", map[string]any{
		"device_id":   deviceID,
		"user_pubkey": pubKey,
		"nonce":       challenge.Nonce,
		"signature":   "valid-signature",
	}, http.StatusOK, &auth)
	if auth.AccessToken == "" || auth.User.ID == uuid.Nil || auth.Device.DeviceID != deviceID || auth.IsNewUser {
		t.Fatalf("unexpected existing auth result: %+v", auth)
	}
	return auth
}

func (e *phase1RouterSmokeEnv) createPrivateConversation(t *testing.T, token string, otherUserID uuid.UUID) model.Conversation {
	t.Helper()
	var conv model.Conversation
	e.doJSON(t, http.MethodPost, "/api/v1/conversations", token, map[string]any{
		"type":            "private",
		"participant_ids": []string{otherUserID.String()},
	}, http.StatusCreated, &conv)
	if conv.ID == uuid.Nil || conv.Type != "private" {
		t.Fatalf("unexpected conversation: %+v", conv)
	}
	return conv
}

func (e *phase1RouterSmokeEnv) updateProfile(t *testing.T, token string, displayName string) model.User {
	t.Helper()
	var user model.User
	e.doJSON(t, http.MethodPut, "/api/v1/users/me", token, map[string]any{"display_name": displayName}, http.StatusOK, &user)
	return user
}

func (e *phase1RouterSmokeEnv) getSyncEvents(t *testing.T, token string, limit int) []model.SyncEvent {
	t.Helper()
	return e.getSyncPull(t, fmt.Sprintf("/api/v1/sync/events?limit=%d", limit), token).Events
}

func (e *phase1RouterSmokeEnv) getSyncEventsAfter(t *testing.T, token string, afterSequence uint64, limit int) []model.SyncEvent {
	t.Helper()
	return e.getSyncPull(t, fmt.Sprintf("/api/v1/sync/events?after_sequence=%d&limit=%d", afterSequence, limit), token).Events
}

func (e *phase1RouterSmokeEnv) getSyncPull(t *testing.T, path string, token string) syncPullResult {
	t.Helper()
	var pull syncPullResult
	e.doJSON(t, http.MethodGet, path, token, nil, http.StatusOK, &pull)
	return pull
}

func (e *phase1RouterSmokeEnv) ackSyncEvents(t *testing.T, token string, sequence uint64) {
	t.Helper()
	var result map[string]string
	e.doJSON(t, http.MethodPost, "/api/v1/sync/ack", token, map[string]any{"last_sequence": sequence}, http.StatusOK, &result)
	if result["status"] != "acked" {
		t.Fatalf("unexpected ack result: %+v", result)
	}
}

func (e *phase1RouterSmokeEnv) startPairing(t *testing.T, token string) service.StartPairingResponse {
	t.Helper()
	var result service.StartPairingResponse
	e.doJSON(t, http.MethodPost, "/api/v1/devices/pairing/start", token, map[string]any{}, http.StatusCreated, &result)
	if result.QRPayload == "" || result.PairingSessionID == uuid.Nil {
		t.Fatalf("unexpected pairing start result: %+v", result)
	}
	return result
}

func (e *phase1RouterSmokeEnv) claimPairing(t *testing.T, qrPayload, newDeviceID, newPubKey string) model.Device {
	t.Helper()
	var device model.Device
	e.doJSON(t, http.MethodPost, "/api/v1/devices/pairing/claim", "", map[string]any{
		"qr_payload":        qrPayload,
		"new_device_id":     newDeviceID,
		"new_device_name":   "Alice Tablet",
		"new_device_pubkey": newPubKey,
		"signature":         "valid-signature",
	}, http.StatusCreated, &device)
	return device
}

func (e *phase1RouterSmokeEnv) doJSON(t *testing.T, method, path, token string, body any, expectedStatus int, out any) {
	t.Helper()
	res := e.doRawJSON(t, method, path, token, body, expectedStatus)
	if out != nil {
		if err := json.Unmarshal(res.Data, out); err != nil {
			t.Fatalf("decode response data: %v data=%s", err, string(res.Data))
		}
	}
}

func (e *phase1RouterSmokeEnv) doRawJSON(t *testing.T, method, path, token string, body any, expectedStatus int) apiResponse {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != expectedStatus {
		t.Fatalf("%s %s expected status %d got %d body=%s", method, path, expectedStatus, rec.Code, rec.Body.String())
	}
	var res apiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode api response: %v body=%s", err, rec.Body.String())
	}
	if expectedStatus < http.StatusBadRequest && res.Code != 0 {
		t.Fatalf("expected api code 0, got response %+v", res)
	}
	if expectedStatus >= http.StatusBadRequest && res.Code != expectedStatus {
		t.Fatalf("expected api error code %d, got response %+v", expectedStatus, res)
	}
	return res
}
