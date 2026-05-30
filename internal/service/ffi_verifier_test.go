package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
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

	pullResponse := `{
		"code": 0,
		"message": "ok",
		"data": {
			"events": [{
				"id": "evt-backend-ffi-1",
				"user_id": "user-1",
				"device_id": "device-b",
				"event_type": "knowledge.created",
				"schema_version": 1,
				"object_type": "knowledge",
				"object_id": "notes/backend-ffi",
				"operation": "created",
				"source_device_id": "device-a",
				"client_event_id": "client-backend-ffi-1",
				"payload": {
					"entry_id": "notes/backend-ffi",
					"object_id": "notes/backend-ffi",
					"title": "Backend FFI Contract",
					"content_markdown": "# Backend FFI Contract",
					"summary": "summary",
					"tags": ["agentos", "ffi"],
					"metadata": {"source": "backend-test"},
					"source_uri": "",
					"status": "active",
					"version": 1,
					"content_hash": "hash-backend-ffi-1",
					"updated_by_device_id": "device-a",
					"updated_at": "2026-05-30T02:00:00Z"
				},
				"timestamp": "2026-05-30T02:00:01Z",
				"sequence": 7
			}],
			"next_after_sequence": 7,
			"has_more": false,
			"server_time": 1780106401000,
			"schema_version": 1
		}
	}`

	var errPtr *byte
	out := applyPullResponse("", pullResponse, &errPtr)
	if errPtr != nil {
		defer verifier.freeString(errPtr)
		t.Fatalf("apply knowledge sync pull response returned error: %s", cStringToGo(errPtr))
	}
	if out == nil {
		t.Fatal("expected knowledge sync bridge output")
	}
	defer verifier.freeString(out)

	response := cStringToGo(out)
	if !strings.Contains(response, "notes/backend-ffi") {
		t.Fatalf("expected projected entry id in bridge response, got %s", response)
	}
	if !strings.Contains(response, "last_applied_sequence") || !strings.Contains(response, "7") {
		t.Fatalf("expected advanced cursor in bridge response, got %s", response)
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
