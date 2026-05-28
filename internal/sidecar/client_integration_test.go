package sidecar

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

func TestClientIntegrationWithRustSidecar(t *testing.T) {
	socketPath := os.Getenv("AGENTOS_SIDECAR_INTEGRATION_SOCKET")
	if socketPath == "" {
		t.Skip("set AGENTOS_SIDECAR_INTEGRATION_SOCKET to run Rust sidecar integration test")
	}

	client, err := NewClient(socketPath)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	health, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("expected sidecar status ok, got %q", health.Status)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	challenge := "agentos-sidecar-integration-challenge"
	sig := ed25519.Sign(priv, []byte(challenge))

	valid, err := client.VerifyEd25519Challenge(
		context.Background(), challenge, hex.EncodeToString(sig), hex.EncodeToString(pub),
	)
	if err != nil {
		t.Fatalf("verify valid signature: %v", err)
	}
	if !valid {
		t.Fatal("expected valid signature")
	}

	valid, err = client.VerifyEd25519Challenge(
		context.Background(), challenge+"-tampered", hex.EncodeToString(sig), hex.EncodeToString(pub),
	)
	if err != nil {
		t.Fatalf("verify invalid signature: %v", err)
	}
	if valid {
		t.Fatal("expected tampered challenge to be invalid")
	}
}
