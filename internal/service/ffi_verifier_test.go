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

func decodeKnowledgeBridgeProjection(t *testing.T, bridgeResponseJSON string) knowledgeBridgeProjection {
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

	var projection knowledgeBridgeProjection
	if err := json.Unmarshal([]byte(bridgeResponse.JSON), &projection); err != nil {
		t.Fatalf("decode knowledge projection JSON: %v; projection=%s", err, bridgeResponse.JSON)
	}
	return projection
}

type knowledgeBridgeProjection struct {
	Cursor struct {
		LastAppliedSequence uint64 `json:"last_applied_sequence"`
	} `json:"cursor"`
	Entries map[string]knowledgeBridgeEntry `json:"entries"`
}

type knowledgeBridgeEntry struct {
	EntryID     string `json:"entry_id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Version     uint64 `json:"version"`
	ContentHash string `json:"content_hash"`
}
