package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuthVerifyHTTPProtocolPolicyCreatesUserAndToken(t *testing.T) {
	r, _, repo := newAuthAdmissionHTTPTestRouter(t)

	res := verifyViaHTTP(t, r, "device-protocol", "pubkey-protocol", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.StatusCode, res.Body)
	}
	if res.Auth.AccessToken == "" || res.Auth.User == nil || res.Auth.Device == nil || !res.Auth.IsNewUser {
		t.Fatalf("unexpected auth response: %+v", res.Auth)
	}
	if _, err := repo.GetByPubKey("pubkey-protocol"); err != nil {
		t.Fatalf("expected user to be created: %v", err)
	}
}

func TestAuthVerifyHTTPInvitationPolicyRequiresCorrectCode(t *testing.T) {
	r, admissionSvc, repo := newAuthAdmissionHTTPTestRouter(t)
	adminID := uuid.New()
	if _, err := admissionSvc.UpdatePolicy("default", "invitation", adminID); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if _, err := admissionSvc.UpdateInvitationCode("default", "secret-code", adminID); err != nil {
		t.Fatalf("update invitation code: %v", err)
	}

	missing := verifyViaHTTP(t, r, "device-invite-missing", "pubkey-invite", nil)
	if missing.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without invitation, got %d body=%s", missing.StatusCode, missing.Body)
	}
	wrongCode := "wrong-code"
	wrong := verifyViaHTTP(t, r, "device-invite-wrong", "pubkey-invite", &wrongCode)
	if wrong.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong invitation, got %d body=%s", wrong.StatusCode, wrong.Body)
	}
	if _, err := repo.GetByPubKey("pubkey-invite"); err == nil {
		t.Fatal("expected denied invitation attempts not to create user")
	}

	correctCode := "secret-code"
	ok := verifyViaHTTP(t, r, "device-invite-ok", "pubkey-invite", &correctCode)
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with correct invitation, got %d body=%s", ok.StatusCode, ok.Body)
	}
	if ok.Auth.AccessToken == "" || ok.Auth.User == nil || ok.Auth.Device == nil || !ok.Auth.IsNewUser {
		t.Fatalf("unexpected auth response: %+v", ok.Auth)
	}
}

func TestAuthVerifyHTTPApprovalPolicyReturnsPendingThenAllowsApprovedRequest(t *testing.T) {
	r, admissionSvc, repo := newAuthAdmissionHTTPTestRouter(t)
	if _, err := admissionSvc.UpdatePolicy("default", "approval", uuid.New()); err != nil {
		t.Fatalf("update policy: %v", err)
	}

	pending := verifyViaHTTP(t, r, "device-approval", "pubkey-approval-http", nil)
	if pending.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 pending approval, got %d body=%s", pending.StatusCode, pending.Body)
	}
	if pending.Auth.AdmissionStatus != "pending_approval" || pending.Auth.AdmissionRequestID == nil || pending.Auth.AccessToken != "" || pending.Auth.User != nil {
		t.Fatalf("expected pending response without token/user, got %+v", pending.Auth)
	}
	if _, err := repo.GetByPubKey("pubkey-approval-http"); err == nil {
		t.Fatal("expected pending approval not to create user")
	}

	if err := admissionSvc.ApproveRequest(*pending.Auth.AdmissionRequestID); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	approved := verifyViaHTTP(t, r, "device-approval", "pubkey-approval-http", nil)
	if approved.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after approval, got %d body=%s", approved.StatusCode, approved.Body)
	}
	if approved.Auth.AccessToken == "" || approved.Auth.User == nil || approved.Auth.Device == nil || !approved.Auth.IsNewUser {
		t.Fatalf("unexpected approved auth response: %+v", approved.Auth)
	}
}

type authAdmissionHTTPVerifier struct{}

func (v *authAdmissionHTTPVerifier) VerifyEd25519Challenge(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

type authAdmissionHTTPResult struct {
	StatusCode int
	Body       string
	Auth       service.AuthResponse
}

func newAuthAdmissionHTTPTestRouter(t *testing.T) (*gin.Engine, *service.AdmissionService, *repository.UserRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.AuthChallenge{}, &model.AdmissionRequest{}, &model.ServerAdmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	admissionRepo := repository.NewAdmissionRepo(db)
	admissionSvc := service.NewAdmissionServiceWithRepo(userRepo, admissionRepo, config.AdmissionConfig{PolicyType: "protocol"})
	identitySvc := service.NewIdentityServiceWithAdmission(userRepo, config.JWTConfig{
		Secret:          "test-secret-that-is-long-enough-for-auth-admission-http",
		AccessTokenMins: 60,
		Issuer:          "agent-os-test",
	}, &authAdmissionHTTPVerifier{}, admissionSvc)
	identityH := NewIdentityHandler(identitySvc)

	r := gin.New()
	auth := r.Group("/api/v1/auth")
	auth.POST("/challenge", identityH.Challenge)
	auth.POST("/verify", identityH.Verify)
	return r, admissionSvc, userRepo
}

func verifyViaHTTP(t *testing.T, r *gin.Engine, deviceID, pubKey string, invitationCode *string) authAdmissionHTTPResult {
	t.Helper()
	challengeBody := `{"device_id":"` + deviceID + `","user_pubkey":"` + pubKey + `"}`
	challengeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/challenge", bytes.NewBufferString(challengeBody))
	challengeReq.Header.Set("Content-Type", "application/json")
	challengeW := httptest.NewRecorder()
	r.ServeHTTP(challengeW, challengeReq)
	if challengeW.Code != http.StatusOK {
		t.Fatalf("expected challenge 200, got %d body=%s", challengeW.Code, challengeW.Body.String())
	}
	var challengeResp responseEnvelope[service.ChallengeResult]
	decodeJSONForTest(t, challengeW.Body.String(), &challengeResp)

	verifyPayload := map[string]any{
		"device_id":   deviceID,
		"user_pubkey": pubKey,
		"nonce":       challengeResp.Data.Nonce,
		"signature":   "valid-signature",
	}
	if invitationCode != nil {
		verifyPayload["invitation_code"] = *invitationCode
	}
	verifyJSON, err := json.Marshal(verifyPayload)
	if err != nil {
		t.Fatalf("marshal verify payload: %v", err)
	}
	verifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewReader(verifyJSON))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyW := httptest.NewRecorder()
	r.ServeHTTP(verifyW, verifyReq)

	result := authAdmissionHTTPResult{StatusCode: verifyW.Code, Body: verifyW.Body.String()}
	if verifyW.Code == http.StatusOK || verifyW.Code == http.StatusAccepted {
		var verifyResp responseEnvelope[service.AuthResponse]
		decodeJSONForTest(t, verifyW.Body.String(), &verifyResp)
		result.Auth = verifyResp.Data
	}
	return result
}
