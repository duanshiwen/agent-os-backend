package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func TestGoEd25519VerifierAcceptsValidSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	challenge := "challenge-123"
	sig := ed25519.Sign(priv, []byte(challenge))

	valid, err := NewGoEd25519Verifier().VerifyEd25519Challenge(
		context.Background(), challenge, hex.EncodeToString(sig), hex.EncodeToString(pub),
	)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !valid {
		t.Fatal("expected signature to be valid")
	}
}

func TestGoEd25519VerifierRejectsWrongChallenge(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sig := ed25519.Sign(priv, []byte("challenge-123"))

	valid, err := NewGoEd25519Verifier().VerifyEd25519Challenge(
		context.Background(), "different-challenge", hex.EncodeToString(sig), hex.EncodeToString(pub),
	)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if valid {
		t.Fatal("expected signature to be invalid")
	}
}

func TestGoEd25519VerifierRejectsMalformedPublicKey(t *testing.T) {
	_, err := NewGoEd25519Verifier().VerifyEd25519Challenge(
		context.Background(), "challenge", "00", "not-hex",
	)
	if err == nil {
		t.Fatal("expected malformed public key error")
	}
}

func TestGoEd25519VerifierRejectsShortSignature(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_, err = NewGoEd25519Verifier().VerifyEd25519Challenge(
		context.Background(), "challenge", "00", hex.EncodeToString(pub),
	)
	if err == nil {
		t.Fatal("expected short signature error")
	}
}
