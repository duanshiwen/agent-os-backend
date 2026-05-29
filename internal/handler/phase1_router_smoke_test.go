package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/router"
	"github.com/agent-os/backend/internal/service"
	"github.com/agent-os/backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPhase1RouterSmokeAuthConversationSyncAndQRPairing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "alice-device-1", "alice-pubkey")
	bob := env.verifyNewUser(t, "bob-device-1", "bob-pubkey")

	conv := env.createPrivateConversation(t, alice.AccessToken, bob.User.ID)
	delivery, err := env.msgSvc.SendMessage(alice.User.ID, &service.SendMessageRequest{
		ConversationID: conv.ID,
		Content:        "hello router smoke",
	})
	if err != nil {
		t.Fatalf("send message via service setup: %v", err)
	}

	bobEvents := env.getSyncEvents(t, bob.AccessToken, 100)
	if len(bobEvents) != 1 || bobEvents[0].EventType != "message.created" || bobEvents[0].Payload["message_id"] != delivery.Message.ID.String() {
		t.Fatalf("expected bob message.created event for sent message, got %+v", bobEvents)
	}
	env.ackSyncEvents(t, bob.AccessToken, bobEvents[0].Sequence)
	bobEvents = env.getSyncEvents(t, bob.AccessToken, 100)
	if len(bobEvents) != 0 {
		t.Fatalf("expected no bob sync events after ack, got %+v", bobEvents)
	}

	aliceProfile := env.updateProfile(t, alice.AccessToken, "Alice Router Smoke")
	if aliceProfile.ID != alice.User.ID || aliceProfile.DisplayName != "Alice Router Smoke" {
		t.Fatalf("unexpected updated profile: %+v", aliceProfile)
	}
	aliceEvents := env.getSyncEventsAfter(t, alice.AccessToken, 1, 100)
	if len(aliceEvents) != 1 || aliceEvents[0].EventType != "profile.updated" || aliceEvents[0].ObjectType != service.SyncEventProfile || aliceEvents[0].ObjectID != alice.User.ID.String() || aliceEvents[0].Operation != service.SyncActionUpdated {
		t.Fatalf("expected alice profile.updated sync event, got %+v", aliceEvents)
	}
	if aliceEvents[0].Payload["display_name"] != "Alice Router Smoke" {
		t.Fatalf("unexpected profile sync payload: %+v", aliceEvents[0].Payload)
	}

	start := env.startPairing(t, alice.AccessToken)
	paired := env.claimPairing(t, start.QRPayload, "alice-device-2", "alice-device-2-pubkey")
	if paired.UserID != alice.User.ID || paired.DeviceID != "alice-device-2" {
		t.Fatalf("unexpected paired device: %+v", paired)
	}
}

func TestSyncEventsResponseIncludesStableEnvelopeFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "contract-device-1", "contract-pubkey")
	env.updateProfile(t, alice.AccessToken, "Contract Alice")

	res := env.doRawJSON(t, http.MethodGet, "/api/v1/sync/events?after_sequence=0&limit=100", alice.AccessToken, nil, http.StatusOK)
	var events []map[string]any
	if err := json.Unmarshal(res.Data, &events); err != nil {
		t.Fatalf("decode raw sync events: %v data=%s", err, string(res.Data))
	}
	if len(events) != 1 {
		t.Fatalf("expected one sync event, got %+v", events)
	}
	event := events[0]
	for _, field := range []string{"id", "user_id", "device_id", "event_type", "schema_version", "object_type", "object_id", "operation", "source_device_id", "client_event_id", "payload", "timestamp", "sequence", "created_at", "updated_at"} {
		if _, ok := event[field]; !ok {
			t.Fatalf("expected sync event JSON field %q in %+v", field, event)
		}
	}
	if event["event_type"] != "profile.updated" || event["object_type"] != service.SyncEventProfile || event["operation"] != service.SyncActionUpdated || event["object_id"] != alice.User.ID.String() {
		t.Fatalf("unexpected sync event contract values: %+v", event)
	}
	payload, ok := event["payload"].(map[string]any)
	if !ok || payload["display_name"] != "Contract Alice" {
		t.Fatalf("unexpected sync payload contract: %+v", event["payload"])
	}
}

func TestSyncEventsRejectsInvalidAfterSequenceQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "bad-query-device-1", "bad-query-pubkey")
	res := env.doRawJSON(t, http.MethodGet, "/api/v1/sync/events?after_sequence=not-a-number", alice.AccessToken, nil, http.StatusBadRequest)
	if res.Code != http.StatusBadRequest || res.Message == "" {
		t.Fatalf("expected bad request response for invalid after_sequence, got %+v", res)
	}
}

func TestSyncEventsLimitQueryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newPhase1RouterSmokeEnv(t)

	alice := env.verifyNewUser(t, "limit-device-1", "limit-pubkey")
	for _, name := range []string{"Limit Alice 1", "Limit Alice 2", "Limit Alice 3"} {
		env.updateProfile(t, alice.AccessToken, name)
	}

	limited := env.getSyncEventsAfter(t, alice.AccessToken, 0, 2)
	if len(limited) != 2 {
		t.Fatalf("expected limit=2 to return 2 events, got %+v", limited)
	}
	if limited[0].Sequence != 1 || limited[1].Sequence != 2 {
		t.Fatalf("expected first two events ordered by sequence, got %+v", limited)
	}

	fallbackZero := env.getSyncEventsAfter(t, alice.AccessToken, 0, 0)
	if len(fallbackZero) != 3 {
		t.Fatalf("expected limit=0 to fallback to default and return all 3 events, got %+v", fallbackZero)
	}
	fallbackTooLarge := env.getSyncEventsAfter(t, alice.AccessToken, 0, 501)
	if len(fallbackTooLarge) != 3 {
		t.Fatalf("expected limit>500 to fallback to default and return all 3 events, got %+v", fallbackTooLarge)
	}
}

type phase1RouterSmokeVerifier struct{}

func (v *phase1RouterSmokeVerifier) VerifyEd25519Challenge(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

type phase1RouterSmokeEnv struct {
	router *gin.Engine
	msgSvc *service.MessageService
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type authResult struct {
	AccessToken string       `json:"access_token"`
	User        model.User   `json:"user"`
	Device      model.Device `json:"device"`
	IsNewUser   bool         `json:"is_new_user"`
}

func newPhase1RouterSmokeEnv(t *testing.T) *phase1RouterSmokeEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.AuthChallenge{}, &model.AdmissionRequest{}, &model.ServerAdmission{}, &model.DevicePairingSession{}, &model.Conversation{}, &model.ConversationParticipant{}, &model.Message{}, &model.OfflineMessage{}, &model.SyncEvent{}, &model.SyncCursor{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	hub := ws.NewHub()
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	syncRepo := repository.NewSyncRepo(db)
	msgSvc := service.NewMessageService(convRepo, userRepo)
	syncSvc := service.NewSyncService(syncRepo, hub)
	msgSvc.SetSyncService(syncSvc)

	cfg := &config.Config{
		App:       config.AppConfig{Name: "agent-os-test", Port: "0", Env: "test", AutoMigrate: false},
		JWT:       config.JWTConfig{Secret: "phase1-router-smoke-secret", Issuer: "agent-os-test", AccessTokenMins: 60},
		CORS:      config.CORSConfig{AllowOrigins: []string{"*"}},
		Admission: config.AdmissionConfig{PolicyType: "protocol"},
	}

	r := router.Setup(cfg, db, redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}), hub, userRepo, convRepo, syncRepo, msgSvc, syncSvc, &phase1RouterSmokeVerifier{})
	return &phase1RouterSmokeEnv{router: r, msgSvc: msgSvc}
}

func (e *phase1RouterSmokeEnv) verifyNewUser(t *testing.T, deviceID, pubKey string) authResult {
	t.Helper()
	var challenge struct {
		Challenge string `json:"challenge"`
		Nonce     string `json:"nonce"`
		ExpiresAt int64  `json:"expires_at"`
	}
	e.doJSON(t, http.MethodPost, "/api/v1/auth/challenge", "", map[string]any{
		"device_id":   deviceID,
		"user_pubkey": pubKey,
	}, http.StatusOK, &challenge)

	var auth authResult
	e.doJSON(t, http.MethodPost, "/api/v1/auth/verify", "", map[string]any{
		"device_id":   deviceID,
		"user_pubkey": pubKey,
		"nonce":       challenge.Nonce,
		"signature":   "valid-signature",
	}, http.StatusOK, &auth)
	if auth.AccessToken == "" || auth.User.ID == uuid.Nil || auth.Device.DeviceID != deviceID || !auth.IsNewUser {
		t.Fatalf("unexpected auth result: %+v", auth)
	}
	return auth
}

func (e *phase1RouterSmokeEnv) createPrivateConversation(t *testing.T, token string, otherUserID uuid.UUID) model.Conversation {
	t.Helper()
	var conv model.Conversation
	e.doJSON(t, http.MethodPost, "/api/v1/conversations", token, map[string]any{
		"type":            "private",
		"participant_ids": []string{otherUserID.String()},
	}, http.StatusCreated, &conv)
	if conv.ID == uuid.Nil || conv.Type != "private" {
		t.Fatalf("unexpected conversation: %+v", conv)
	}
	return conv
}

func (e *phase1RouterSmokeEnv) updateProfile(t *testing.T, token string, displayName string) model.User {
	t.Helper()
	var user model.User
	e.doJSON(t, http.MethodPut, "/api/v1/users/me", token, map[string]any{"display_name": displayName}, http.StatusOK, &user)
	return user
}

func (e *phase1RouterSmokeEnv) getSyncEvents(t *testing.T, token string, limit int) []model.SyncEvent {
	t.Helper()
	var events []model.SyncEvent
	e.doJSON(t, http.MethodGet, fmt.Sprintf("/api/v1/sync/events?limit=%d", limit), token, nil, http.StatusOK, &events)
	return events
}

func (e *phase1RouterSmokeEnv) getSyncEventsAfter(t *testing.T, token string, afterSequence uint64, limit int) []model.SyncEvent {
	t.Helper()
	var events []model.SyncEvent
	e.doJSON(t, http.MethodGet, fmt.Sprintf("/api/v1/sync/events?after_sequence=%d&limit=%d", afterSequence, limit), token, nil, http.StatusOK, &events)
	return events
}

func (e *phase1RouterSmokeEnv) ackSyncEvents(t *testing.T, token string, sequence uint64) {
	t.Helper()
	var result map[string]string
	e.doJSON(t, http.MethodPost, "/api/v1/sync/ack", token, map[string]any{"last_sequence": sequence}, http.StatusOK, &result)
	if result["status"] != "acked" {
		t.Fatalf("unexpected ack result: %+v", result)
	}
}

func (e *phase1RouterSmokeEnv) startPairing(t *testing.T, token string) service.StartPairingResponse {
	t.Helper()
	var result service.StartPairingResponse
	e.doJSON(t, http.MethodPost, "/api/v1/devices/pairing/start", token, map[string]any{}, http.StatusCreated, &result)
	if result.QRPayload == "" || result.PairingSessionID == uuid.Nil {
		t.Fatalf("unexpected pairing start result: %+v", result)
	}
	return result
}

func (e *phase1RouterSmokeEnv) claimPairing(t *testing.T, qrPayload, newDeviceID, newPubKey string) model.Device {
	t.Helper()
	var device model.Device
	e.doJSON(t, http.MethodPost, "/api/v1/devices/pairing/claim", "", map[string]any{
		"qr_payload":        qrPayload,
		"new_device_id":     newDeviceID,
		"new_device_name":   "Alice Tablet",
		"new_device_pubkey": newPubKey,
		"signature":         "valid-signature",
	}, http.StatusCreated, &device)
	return device
}

func (e *phase1RouterSmokeEnv) doJSON(t *testing.T, method, path, token string, body any, expectedStatus int, out any) {
	t.Helper()
	res := e.doRawJSON(t, method, path, token, body, expectedStatus)
	if out != nil {
		if err := json.Unmarshal(res.Data, out); err != nil {
			t.Fatalf("decode response data: %v data=%s", err, string(res.Data))
		}
	}
}

func (e *phase1RouterSmokeEnv) doRawJSON(t *testing.T, method, path, token string, body any, expectedStatus int) apiResponse {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != expectedStatus {
		t.Fatalf("%s %s expected status %d got %d body=%s", method, path, expectedStatus, rec.Code, rec.Body.String())
	}
	var res apiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode api response: %v body=%s", err, rec.Body.String())
	}
	if expectedStatus < http.StatusBadRequest && res.Code != 0 {
		t.Fatalf("expected api code 0, got response %+v", res)
	}
	if expectedStatus >= http.StatusBadRequest && res.Code != expectedStatus {
		t.Fatalf("expected api error code %d, got response %+v", expectedStatus, res)
	}
	return res
}
