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
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stubVerifier struct {
	valid         bool
	err           error
	calls         int
	lastChallenge string
	lastSignature string
	lastPublicKey string
}

func (s *stubVerifier) VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error) {
	_ = ctx
	s.calls++
	s.lastChallenge = challenge
	s.lastSignature = signatureHex
	s.lastPublicKey = publicKeyHex
	return s.valid, s.err
}

func newIdentityTestService(t *testing.T, verifier SignatureVerifier) *IdentityService {
	t.Helper()
	svc, _, _ := newIdentityAdmissionTestService(t, verifier)
	return svc
}

func newIdentityAdmissionTestService(t *testing.T, verifier SignatureVerifier) (*IdentityService, *AdmissionService, *repository.UserRepo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.AuthChallenge{}, &model.AdmissionRequest{}, &model.ServerAdmission{}, &model.SyncEvent{}, &model.SyncCursor{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewUserRepo(db)
	admissionRepo := repository.NewAdmissionRepo(db)
	admissionSvc := NewAdmissionServiceWithRepo(repo, admissionRepo, config.AdmissionConfig{PolicyType: "protocol"})
	identitySvc := NewIdentityServiceWithAdmission(repo, config.JWTConfig{
		Secret:          "test-secret-that-is-long-enough-for-unit-tests",
		AccessTokenMins: 60,
		Issuer:          "agent-os-test",
	}, verifier, admissionSvc)
	return identitySvc, admissionSvc, repo
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

func TestIdentityServiceVerifySignatureDeniesNewUserWithoutInvitationCode(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	svc, admissionSvc, repo := newIdentityAdmissionTestService(t, verifier)
	adminID := mustUUIDForTest(t)
	if _, err := admissionSvc.UpdatePolicy("default", "invitation", adminID); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if _, err := admissionSvc.UpdateInvitationCode("default", "secret-code", adminID); err != nil {
		t.Fatalf("update invitation: %v", err)
	}

	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	challenge, err := svc.InitiateChallenge("device-invite-missing", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}

	_, err = svc.VerifySignature(&VerifyRequest{
		DeviceID:   "device-invite-missing",
		UserPubKey: pubKey,
		Nonce:      challenge.Nonce,
		Signature:  "ignored-by-stub",
	})
	if err == nil || err.Error() != "admission denied: invalid_invitation_code" {
		t.Fatalf("expected invitation denial, got %v", err)
	}
	if _, err := repo.GetByPubKey(pubKey); err == nil {
		t.Fatal("expected denied user not to be created")
	}
}

func TestIdentityServiceVerifySignatureAllowsNewUserWithInvitationCode(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	svc, admissionSvc, _ := newIdentityAdmissionTestService(t, verifier)
	adminID := mustUUIDForTest(t)
	if _, err := admissionSvc.UpdatePolicy("default", "invitation", adminID); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if _, err := admissionSvc.UpdateInvitationCode("default", "secret-code", adminID); err != nil {
		t.Fatalf("update invitation: %v", err)
	}

	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	challenge, err := svc.InitiateChallenge("device-invite-ok", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}
	code := "secret-code"
	res, err := svc.VerifySignature(&VerifyRequest{
		DeviceID:       "device-invite-ok",
		UserPubKey:     pubKey,
		Nonce:          challenge.Nonce,
		Signature:      "ignored-by-stub",
		InvitationCode: &code,
	})
	if err != nil {
		t.Fatalf("verify with invitation: %v", err)
	}
	if res.AccessToken == "" || res.User == nil || res.Device == nil || !res.IsNewUser {
		t.Fatalf("unexpected auth response: %+v", res)
	}
}

func TestIdentityServiceVerifySignatureApprovalCreatesPendingWithoutUser(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	svc, admissionSvc, repo := newIdentityAdmissionTestService(t, verifier)
	if _, err := admissionSvc.UpdatePolicy("default", "approval", mustUUIDForTest(t)); err != nil {
		t.Fatalf("update policy: %v", err)
	}

	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	challenge, err := svc.InitiateChallenge("device-approval-pending", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}
	res, err := svc.VerifySignature(&VerifyRequest{
		DeviceID:   "device-approval-pending",
		UserPubKey: pubKey,
		Nonce:      challenge.Nonce,
		Signature:  "ignored-by-stub",
	})
	if err != nil {
		t.Fatalf("verify approval: %v", err)
	}
	if res.AdmissionStatus != "pending_approval" || res.AdmissionRequestID == nil || res.AccessToken != "" || res.User != nil {
		t.Fatalf("expected pending response without token/user, got %+v", res)
	}
	if _, err := repo.GetByPubKey(pubKey); err == nil {
		t.Fatal("expected pending user not to be created")
	}

	challenge2, err := svc.InitiateChallenge("device-approval-pending", pubKey)
	if err != nil {
		t.Fatalf("initiate second challenge: %v", err)
	}
	res2, err := svc.VerifySignature(&VerifyRequest{
		DeviceID:   "device-approval-pending",
		UserPubKey: pubKey,
		Nonce:      challenge2.Nonce,
		Signature:  "ignored-by-stub",
	})
	if err != nil {
		t.Fatalf("verify second approval: %v", err)
	}
	if res2.AdmissionRequestID == nil || *res2.AdmissionRequestID != *res.AdmissionRequestID {
		t.Fatalf("expected duplicate pending attempt to reuse request id, got first=%v second=%v", res.AdmissionRequestID, res2.AdmissionRequestID)
	}
	pending, err := repo.GetPendingAdmissionRequests()
	if err != nil {
		t.Fatalf("get pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one pending request, got %+v", pending)
	}
}

func TestIdentityServiceVerifySignatureRejectsRejectedAdmissionRequest(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	svc, admissionSvc, repo := newIdentityAdmissionTestService(t, verifier)
	if _, err := admissionSvc.UpdatePolicy("default", "approval", mustUUIDForTest(t)); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	req := &model.AdmissionRequest{UserPubKey: pubKey, Status: "rejected", Reason: "not allowed"}
	if err := repo.CreateAdmissionRequest(req); err != nil {
		t.Fatalf("create rejected request: %v", err)
	}
	challenge, err := svc.InitiateChallenge("device-rejected", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}

	_, err = svc.VerifySignature(&VerifyRequest{
		DeviceID:   "device-rejected",
		UserPubKey: pubKey,
		Nonce:      challenge.Nonce,
		Signature:  "ignored-by-stub",
	})
	if err == nil || err.Error() != "admission denied: admission_rejected" {
		t.Fatalf("expected rejected admission denial, got %v", err)
	}
	if _, err := repo.GetByPubKey(pubKey); err == nil {
		t.Fatal("expected rejected user not to be created")
	}
}

func TestIdentityServiceVerifySignatureAllowsApprovedAdmissionRequest(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	svc, admissionSvc, repo := newIdentityAdmissionTestService(t, verifier)
	if _, err := admissionSvc.UpdatePolicy("default", "approval", mustUUIDForTest(t)); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	pubKey := hex.EncodeToString(make([]byte, ed25519.PublicKeySize))
	req := &model.AdmissionRequest{UserPubKey: pubKey, Status: "approved"}
	if err := repo.CreateAdmissionRequest(req); err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	challenge, err := svc.InitiateChallenge("device-approved", pubKey)
	if err != nil {
		t.Fatalf("initiate challenge: %v", err)
	}

	res, err := svc.VerifySignature(&VerifyRequest{
		DeviceID:   "device-approved",
		UserPubKey: pubKey,
		Nonce:      challenge.Nonce,
		Signature:  "ignored-by-stub",
	})
	if err != nil {
		t.Fatalf("verify approved admission: %v", err)
	}
	if res.AccessToken == "" || res.User == nil || !res.IsNewUser || res.AdmissionStatus != "" {
		t.Fatalf("unexpected approved auth response: %+v", res)
	}
}

func mustUUIDForTest(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestIdentityServiceUpdateProfileRecordsProfileUpdatedSyncEvent(t *testing.T) {
	svc, _, repo := newIdentityAdmissionTestService(t, &stubVerifier{valid: true})
	syncSvc, _ := newSyncTestService(t)
	svc.SetSyncService(syncSvc)
	user := &model.User{PubKeyEd25519: "profile-sync-pubkey", DisplayName: "Old"}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	newName := "Alice"
	updated, err := svc.UpdateProfile(user.ID, &newName, nil)
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.DisplayName != newName {
		t.Fatalf("expected updated display name, got %+v", updated)
	}

	events, err := syncSvc.GetEventsAfter(user.ID, 0, 100)
	if err != nil {
		t.Fatalf("get sync events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one sync event, got %+v", events)
	}
	event := events[0]
	if event.EventType != "profile.updated" || event.ObjectType != SyncEventProfile || event.ObjectID != user.ID.String() || event.Operation != SyncActionUpdated {
		t.Fatalf("unexpected profile sync event: %+v", event)
	}
	if event.Payload["display_name"] != newName {
		t.Fatalf("unexpected profile sync payload: %+v", event.Payload)
	}
}
