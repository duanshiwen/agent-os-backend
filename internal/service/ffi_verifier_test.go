package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
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
