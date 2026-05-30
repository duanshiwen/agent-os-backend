package service

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
)

func TestIdentityServiceRenameAndRevokeDevice(t *testing.T) {
	svc, _, repo := newIdentityAdmissionTestService(t, &stubVerifier{valid: true})
	user := &model.User{PubKeyEd25519: "device-user-pubkey", DisplayName: "Device User", Status: "active"}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := &model.Device{UserID: user.ID, DeviceID: "current-device", DeviceName: "Current", DevicePubKey: "current-pubkey", Status: "active", PairedAt: time.Now()}
	target := &model.Device{UserID: user.ID, DeviceID: "target-device", DeviceName: "Old Target", DevicePubKey: "target-pubkey", Status: "active", PairedAt: time.Now()}
	if err := repo.CreateDevice(current); err != nil {
		t.Fatalf("create current device: %v", err)
	}
	if err := repo.CreateDevice(target); err != nil {
		t.Fatalf("create target device: %v", err)
	}

	renamed, err := svc.RenameDevice(user.ID, "target-device", "Renamed Target")
	if err != nil {
		t.Fatalf("rename device: %v", err)
	}
	if renamed.DeviceName != "Renamed Target" || renamed.Status != "active" {
		t.Fatalf("unexpected renamed device: %+v", renamed)
	}

	revoked, err := svc.RevokeDevice(user.ID, "current-device", "target-device")
	if err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	if revoked.Status != "revoked" || revoked.RevokedAt == nil || revoked.RevokedBy == nil || *revoked.RevokedBy != user.ID {
		t.Fatalf("expected revoked metadata, got %+v", revoked)
	}
	if _, err := svc.RenameDevice(user.ID, "target-device", "Should Fail"); err == nil || err.Error() != "device revoked" {
		t.Fatalf("expected rename revoked device to fail, got %v", err)
	}
}

func TestIdentityServiceRevokeDeviceSafetyChecks(t *testing.T) {
	svc, _, repo := newIdentityAdmissionTestService(t, &stubVerifier{valid: true})
	user := &model.User{PubKeyEd25519: "device-safety-pubkey", DisplayName: "Device Safety", Status: "active"}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := &model.Device{UserID: user.ID, DeviceID: "only-device", DeviceName: "Only", DevicePubKey: "only-pubkey", Status: "active", PairedAt: time.Now()}
	if err := repo.CreateDevice(current); err != nil {
		t.Fatalf("create only device: %v", err)
	}
	if _, err := svc.RevokeDevice(user.ID, "only-device", "only-device"); err == nil || err.Error() != "cannot revoke current device" {
		t.Fatalf("expected current device revoke denial, got %v", err)
	}
	second := &model.Device{UserID: user.ID, DeviceID: "second-device", DeviceName: "Second", DevicePubKey: "second-pubkey", Status: "active", PairedAt: time.Now()}
	if err := repo.CreateDevice(second); err != nil {
		t.Fatalf("create second device: %v", err)
	}
	if _, err := svc.RevokeDevice(user.ID, "only-device", "second-device"); err != nil {
		t.Fatalf("revoke second device: %v", err)
	}
	if _, err := svc.RevokeDevice(user.ID, "only-device", "second-device"); err == nil || err.Error() != "device already revoked" {
		t.Fatalf("expected already revoked denial, got %v", err)
	}
}

func TestIdentityServiceVerifySignatureRejectsRevokedDevice(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	svc, _, repo := newIdentityAdmissionTestService(t, verifier)
	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	user := &model.User{PubKeyEd25519: pubKey, DisplayName: "Revoked User", Status: "active"}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	device := &model.Device{UserID: user.ID, DeviceID: "revoked-device", DeviceName: "Revoked", DevicePubKey: pubKey, Status: "revoked", PairedAt: time.Now()}
	if err := repo.CreateDevice(device); err != nil {
		t.Fatalf("create revoked device: %v", err)
	}
	challenge, err := svc.InitiateChallenge("revoked-device", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}
	_, err = svc.VerifySignature(&VerifyRequest{DeviceID: "revoked-device", UserPubKey: pubKey, Nonce: challenge.Nonce, Signature: "ignored"})
	if err == nil || err.Error() != "device revoked" {
		t.Fatalf("expected device revoked error, got %v", err)
	}
}
