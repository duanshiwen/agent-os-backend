package service

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stubVerifier struct {
	valid bool
	err   error
	calls int
}

func (s *stubVerifier) VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error) {
	_ = ctx
	s.calls++
	return s.valid, s.err
}

func newIdentityTestService(t *testing.T, verifier SignatureVerifier) *IdentityService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.AuthChallenge{}, &model.AdmissionRequest{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewUserRepo(db)
	return NewIdentityServiceWithVerifier(repo, config.JWTConfig{
		Secret:          "test-secret-that-is-long-enough-for-unit-tests",
		AccessTokenMins: 60,
		Issuer:          "agent-os-test",
	}, verifier)
}

func TestIdentityServiceVerifySignatureUsesInjectedVerifier(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	svc := newIdentityTestService(t, verifier)
	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))

	challenge, err := svc.InitiateChallenge("device-1", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}

	res, err := svc.VerifySignature(&VerifyRequest{
		DeviceID:   "device-1",
		UserPubKey: pubKey,
		Nonce:      challenge.Nonce,
		Signature:  "ignored-by-stub",
	})
	if err != nil {
		t.Fatalf("verify signature: %v", err)
	}
	if verifier.calls != 1 {
		t.Fatalf("expected verifier to be called once, got %d", verifier.calls)
	}
	if res.AccessToken == "" || res.User == nil || res.Device == nil || !res.IsNewUser {
		t.Fatalf("unexpected auth response: %+v", res)
	}
}

func TestIdentityServiceVerifySignatureRejectsInvalidSignature(t *testing.T) {
	verifier := &stubVerifier{valid: false}
	svc := newIdentityTestService(t, verifier)
	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	challenge, err := svc.InitiateChallenge("device-1", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}

	_, err = svc.VerifySignature(&VerifyRequest{
		DeviceID:   "device-1",
		UserPubKey: pubKey,
		Nonce:      challenge.Nonce,
		Signature:  "invalid",
	})
	if err == nil || err.Error() != "signature verification failed" {
		t.Fatalf("expected signature verification failed, got %v", err)
	}
}

func TestIdentityServiceVerifySignaturePropagatesVerifierError(t *testing.T) {
	verifierErr := errors.New("verifier unavailable")
	verifier := &stubVerifier{err: verifierErr}
	svc := newIdentityTestService(t, verifier)
	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	challenge, err := svc.InitiateChallenge("device-1", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}

	_, err = svc.VerifySignature(&VerifyRequest{
		DeviceID:   "device-1",
		UserPubKey: pubKey,
		Nonce:      challenge.Nonce,
		Signature:  "ignored-by-stub",
	})
	if !errors.Is(err, verifierErr) {
		t.Fatalf("expected verifier error, got %v", err)
	}
}
