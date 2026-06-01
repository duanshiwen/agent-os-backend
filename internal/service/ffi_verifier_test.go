package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ebitengine/purego"
)

func TestFFIArtifactContractIntegration(t *testing.T) {
	verifier, err := NewFFIVerifier("")
	if err != nil {
		t.Fatalf("new ffi verifier: %v", err)
	}
	defer verifier.Close()

	if verifier.LibraryPath() == "" {
		t.Fatal("expected verifier to resolve bundled library path")
	}
	if _, err := os.Stat(verifier.LibraryPath()); err != nil {
		t.Fatalf("expected bundled FFI library to exist at %s: %v", verifier.LibraryPath(), err)
	}
	if verifier.Version() != "0.1.0" {
		t.Fatalf("expected bundled FFI version 0.1.0, got %q", verifier.Version())
	}

	var applyKnowledgeEvents func(projectionJSON string, eventsJSON string, errorOut **byte) *byte
	var applyKnowledgePullResponse func(projectionJSON string, pullResponseJSON string, errorOut **byte) *byte
	var applyClientReadyPullResponse func(projectionJSON string, pullResponseJSON string, errorOut **byte) *byte
	purego.RegisterLibFunc(&applyKnowledgeEvents, verifier.handle, "agentos_apply_knowledge_sync_events_json")
	purego.RegisterLibFunc(&applyKnowledgePullResponse, verifier.handle, "agentos_apply_knowledge_sync_pull_response_json")
	purego.RegisterLibFunc(&applyClientReadyPullResponse, verifier.handle, "agentos_apply_sync_pull_response_json")

	if applyKnowledgeEvents == nil || applyKnowledgePullResponse == nil || applyClientReadyPullResponse == nil {
		t.Fatal("expected required FFI bridge symbols to be registered")
	}
}

func TestFFIVerifierIntegration(t *testing.T) {
	verifier, err := NewFFIVerifier("")
	if err != nil {
		t.Fatalf("new ffi verifier: %v", err)
	}
	defer verifier.Close()

	if verifier.Backend() != "ffi" {
		t.Fatalf("expected ffi backend, got %q", verifier.Backend())
	}
	if verifier.Version() == "" {
		t.Fatal("expected non-empty ffi version")
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	challenge := "agentos-ffi-integration-challenge"
	sig := ed25519.Sign(priv, []byte(challenge))

	valid, err := verifier.VerifyEd25519Challenge(context.Background(), challenge, hex.EncodeToString(sig), hex.EncodeToString(pub))
	if err != nil {
		t.Fatalf("verify valid signature: %v", err)
	}
	if !valid {
		t.Fatal("expected valid signature")
	}

	valid, err = verifier.VerifyEd25519Challenge(context.Background(), challenge+"-tampered", hex.EncodeToString(sig), hex.EncodeToString(pub))
	if err != nil {
		t.Fatalf("verify invalid signature: %v", err)
	}
	if valid {
		t.Fatal("expected tampered challenge to be invalid")
	}

	_, err = verifier.VerifyEd25519Challenge(context.Background(), challenge, "not-hex", hex.EncodeToString(pub))
	if err == nil || !strings.Contains(err.Error(), "agentos ffi verifier error") {
		t.Fatalf("expected ffi malformed signature error, got %v", err)
	}
}

func TestFFIKnowledgeSyncBridgeIntegration(t *testing.T) {
	verifier, err := NewFFIVerifier("")
	if err != nil {
		t.Fatalf("new ffi verifier: %v", err)
	}
	defer verifier.Close()

	var applyPullResponse func(projectionJSON string, pullResponseJSON string, errorOut **byte) *byte
	purego.RegisterLibFunc(&applyPullResponse, verifier.handle, "agentos_apply_knowledge_sync_pull_response_json")

	pullResponse, err := os.ReadFile("testdata/m2_3_knowledge_sync_pull_response.json")
	if err != nil {
		t.Fatalf("read M2.3 knowledge sync fixture: %v", err)
	}

	var errPtr *byte
	out := applyPullResponse("", string(pullResponse), &errPtr)
	if errPtr != nil {
		defer verifier.freeString(errPtr)
		t.Fatalf("apply knowledge sync pull response returned error: %s", cStringToGo(errPtr))
	}
	if out == nil {
		t.Fatal("expected knowledge sync bridge output")
	}
	defer verifier.freeString(out)

	projection := decodeKnowledgeBridgeProjection(t, cStringToGo(out))
	if projection.Cursor.LastAppliedSequence != 10 {
		t.Fatalf("expected cursor to advance through stale event sequence 10, got %+v", projection.Cursor)
	}
	entry, ok := projection.Entries["notes/backend-ffi"]
	if !ok {
		t.Fatalf("expected projected entry notes/backend-ffi, got %+v", projection.Entries)
	}
	if entry.Version != 3 || entry.Status != "deleted" || entry.Title != "Backend FFI Contract v2" || entry.ContentHash != "hash-backend-ffi-3" {
		t.Fatalf("expected deleted v3 tombstone to win over stale v2 event, got %+v", entry)
	}

	errPtr = nil
	out = applyPullResponse("", "not-json", &errPtr)
	if out != nil {
		defer verifier.freeString(out)
		t.Fatalf("expected invalid JSON to fail, got output %s", cStringToGo(out))
	}
	if errPtr == nil {
		t.Fatal("expected invalid JSON error from knowledge sync bridge")
	}
	defer verifier.freeString(errPtr)
	if !strings.Contains(cStringToGo(errPtr), "invalid backend sync pull response json") {
		t.Fatalf("expected invalid pull response error, got %s", cStringToGo(errPtr))
	}
}

func TestFFIClientReadySyncBridgeIntegration(t *testing.T) {
	verifier, err := NewFFIVerifier("")
	if err != nil {
		t.Fatalf("new ffi verifier: %v", err)
	}
	defer verifier.Close()

	var applyPullResponse func(projectionJSON string, pullResponseJSON string, errorOut **byte) *byte
	purego.RegisterLibFunc(&applyPullResponse, verifier.handle, "agentos_apply_sync_pull_response_json")

	pullResponse, err := os.ReadFile("testdata/stage5a_client_ready_sync_pull_response.json")
	if err != nil {
		t.Fatalf("read Stage 5A sync fixture: %v", err)
	}

	var errPtr *byte
	out := applyPullResponse("", string(pullResponse), &errPtr)
	if errPtr != nil {
		defer verifier.freeString(errPtr)
		t.Fatalf("apply client-ready sync pull response returned error: %s", cStringToGo(errPtr))
	}
	if out == nil {
		t.Fatal("expected client-ready sync bridge output")
	}
	defer verifier.freeString(out)

	projection := decodeClientReadyBridgeProjection(t, cStringToGo(out))
	if projection.Cursor.LastAppliedSequence != 26 {
		t.Fatalf("expected cursor sequence 26, got %+v", projection.Cursor)
	}
	if profile, ok := projection.Profiles["user-1"]; !ok || profile["display_name"] != "Stage 5A User" {
		t.Fatalf("expected user profile projection, got %+v", projection.Profiles)
	}
	if conversation, ok := projection.Conversations["conversation-1"]; !ok || conversation["name"] != "Stage 5A Conversation" {
		t.Fatalf("expected conversation projection, got %+v", projection.Conversations)
	}
	if participant, ok := projection.Participants["conversation-1:user-1"]; !ok || participant["status"] != "active" {
		t.Fatalf("expected participant projection, got %+v", projection.Participants)
	}
	if read, ok := projection.ConversationReads["conversation-1:user-1"]; !ok || read["last_read_message_id"] != "message-1" {
		t.Fatalf("expected conversation read projection, got %+v", projection.ConversationReads)
	}
	if message, ok := projection.Messages["message-1"]; !ok || message["conversation_id"] != "conversation-1" {
		t.Fatalf("expected message projection, got %+v", projection.Messages)
	}
	if _, ok := projection.MessageReactions["message-1:user-1:👍"]; ok {
		t.Fatalf("expected removed message reaction to be absent, got %+v", projection.MessageReactions)
	}
	if skill, ok := projection.Skills["superpowers"]; !ok || skill["enabled"] != true {
		t.Fatalf("expected enabled superpowers skill projection, got %+v", projection.Skills)
	}
	if skill, ok := projection.Skills["legacy-skill"]; !ok || skill["enabled"] != false {
		t.Fatalf("expected disabled legacy skill projection, got %+v", projection.Skills)
	}
	if skill, ok := projection.Skills["installation-1"]; !ok || skill["skill_key"] != "research.brief" {
		t.Fatalf("expected installed Skill Hub projection, got %+v", projection.Skills)
	}
	if _, ok := projection.Skills["installation-removed"]; ok {
		t.Fatalf("expected uninstalled Skill Hub projection to be absent, got %+v", projection.Skills)
	}
	if agent, ok := projection.Agents["assistant-main"]; !ok || agent["display_name"] != "Assistant" {
		t.Fatalf("expected agent projection, got %+v", projection.Agents)
	}
	if server, ok := projection.Servers["server-primary"]; !ok || server["base_url"] != "https://agent.example" {
		t.Fatalf("expected primary server projection, got %+v", projection.Servers)
	}
	if _, ok := projection.Servers["server-removed"]; ok {
		t.Fatalf("expected removed server to be absent, got %+v", projection.Servers)
	}
	plugin, ok := projection.Plugins["installation-1"]
	if !ok || plugin["plugin_key"] != "com.example.hotel" || plugin["status"] != "active" {
		t.Fatalf("expected active installed plugin projection, got %+v", projection.Plugins)
	}
	if _, ok := projection.Plugins["installation-removed"]; ok {
		t.Fatalf("expected uninstalled plugin to be absent, got %+v", projection.Plugins)
	}
	if _, ok := projection.PluginPermissions["grant-1"]; ok {
		t.Fatalf("expected revoked plugin permission grant-1 to be absent, got %+v", projection.PluginPermissions)
	}
	permission, ok := projection.PluginPermissions["grant-2"]
	if !ok || permission["permission_key"] != "plugin.user.confirm" {
		t.Fatalf("expected active plugin permission grant-2 projection, got %+v", projection.PluginPermissions)
	}
	entry, ok := projection.Knowledge.Entries["notes/stage5a"]
	if !ok {
		t.Fatalf("expected knowledge projection entry, got %+v", projection.Knowledge.Entries)
	}
	if entry.Version != 3 || entry.Status != "deleted" || entry.ContentHash != "hash-stage5a-3" {
		t.Fatalf("expected deleted v3 knowledge tombstone, got %+v", entry)
	}
}

func decodeKnowledgeBridgeProjection(t *testing.T, bridgeResponseJSON string) knowledgeBridgeProjection {
	projection := decodeBridgeProjectionJSON[knowledgeBridgeProjection](t, bridgeResponseJSON)
	return projection
}

func decodeClientReadyBridgeProjection(t *testing.T, bridgeResponseJSON string) clientReadyBridgeProjection {
	projection := decodeBridgeProjectionJSON[clientReadyBridgeProjection](t, bridgeResponseJSON)
	return projection
}

func decodeBridgeProjectionJSON[T any](t *testing.T, bridgeResponseJSON string) T {
	t.Helper()
	var bridgeResponse struct {
		OK   bool   `json:"ok"`
		JSON string `json:"json"`
	}
	if err := json.Unmarshal([]byte(bridgeResponseJSON), &bridgeResponse); err != nil {
		t.Fatalf("decode bridge response JSON: %v; response=%s", err, bridgeResponseJSON)
	}
	if !bridgeResponse.OK || bridgeResponse.JSON == "" {
		t.Fatalf("expected successful bridge response, got %+v", bridgeResponse)
	}

	var projection T
	if err := json.Unmarshal([]byte(bridgeResponse.JSON), &projection); err != nil {
		t.Fatalf("decode projection JSON: %v; projection=%s", err, bridgeResponse.JSON)
	}
	return projection
}

type knowledgeBridgeProjection struct {
	Cursor struct {
		LastAppliedSequence uint64 `json:"last_applied_sequence"`
	} `json:"cursor"`
	Entries map[string]knowledgeBridgeEntry `json:"entries"`
}

type clientReadyBridgeProjection struct {
	Cursor struct {
		LastAppliedSequence uint64 `json:"last_applied_sequence"`
	} `json:"cursor"`
	Knowledge         knowledgeBridgeProjection `json:"knowledge"`
	Profiles          map[string]map[string]any `json:"profiles"`
	Conversations     map[string]map[string]any `json:"conversations"`
	Participants      map[string]map[string]any `json:"participants"`
	ConversationReads map[string]map[string]any `json:"conversation_reads"`
	Messages          map[string]map[string]any `json:"messages"`
	MessageReactions  map[string]map[string]any `json:"message_reactions"`
	Skills            map[string]map[string]any `json:"skills"`
	Agents            map[string]map[string]any `json:"agents"`
	Servers           map[string]map[string]any `json:"servers"`
	Plugins           map[string]map[string]any `json:"plugins"`
	PluginPermissions map[string]map[string]any `json:"plugin_permissions"`
}

type knowledgeBridgeEntry struct {
	EntryID     string `json:"entry_id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Version     uint64 `json:"version"`
	ContentHash string `json:"content_hash"`
}
