package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDevicePairingHTTPStartRequiresJWT(t *testing.T) {
	r, _, _ := newDevicePairingHTTPTestRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/pairing/start", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDevicePairingHTTPStartAndClaim(t *testing.T) {
	r, user, oldDeviceID := newDevicePairingHTTPTestRouter(t)
	token, err := middleware.GenerateToken(testJWTSecret, user.ID, oldDeviceID, "agent-os-test", 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/pairing/start", nil)
	startReq.Header.Set("Authorization", "Bearer "+token)
	startW := httptest.NewRecorder()
	r.ServeHTTP(startW, startReq)
	if startW.Code != http.StatusCreated {
		t.Fatalf("expected 201 from start, got %d body=%s", startW.Code, startW.Body.String())
	}
	var startResp responseEnvelope[service.StartPairingResponse]
	decodeJSONForTest(t, startW.Body.String(), &startResp)
	if startResp.Data.QRPayload == "" {
		t.Fatalf("expected qr payload in start response: %+v", startResp)
	}

	claimBody := `{"qr_payload":"` + startResp.Data.QRPayload + `","new_device_id":"new-device-http","new_device_name":"New HTTP Device","new_device_pubkey":"new-pubkey-http"}`
	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/pairing/claim", strings.NewReader(claimBody))
	claimReq.Header.Set("Content-Type", "application/json")
	claimW := httptest.NewRecorder()
	r.ServeHTTP(claimW, claimReq)
	if claimW.Code != http.StatusCreated {
		t.Fatalf("expected 201 from claim, got %d body=%s", claimW.Code, claimW.Body.String())
	}
	var claimResp responseEnvelope[model.Device]
	decodeJSONForTest(t, claimW.Body.String(), &claimResp)
	if claimResp.Data.UserID != user.ID || claimResp.Data.DeviceID != "new-device-http" || claimResp.Data.DevicePubKey != "new-pubkey-http" {
		t.Fatalf("unexpected claimed device: %+v", claimResp.Data)
	}
}

const testJWTSecret = "test-secret-that-is-long-enough-for-device-pairing-http-tests"

type responseEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func newDevicePairingHTTPTestRouter(t *testing.T) (*gin.Engine, *model.User, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.DevicePairingSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	pairingRepo := repository.NewDevicePairingRepo(db)
	user := &model.User{PubKeyEd25519: "http-user-pubkey", DisplayName: "HTTP User", Status: "active"}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	oldDeviceID := "old-device-http"
	oldDevice := &model.Device{UserID: user.ID, DeviceID: oldDeviceID, DeviceName: "Old HTTP Device", DevicePubKey: "old-pubkey-http", PairedAt: time.Now()}
	if err := userRepo.CreateDevice(oldDevice); err != nil {
		t.Fatalf("create old device: %v", err)
	}

	pairingSvc := service.NewDevicePairingService(pairingRepo, userRepo)
	pairingH := NewDevicePairingHandler(pairingSvc)
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.POST("/devices/pairing/claim", pairingH.Claim)
	protected := v1.Group("")
	protected.Use(middleware.JWTAuth(testJWTSecret))
	protected.POST("/devices/pairing/start", pairingH.Start)
	return r, user, oldDeviceID
}

func decodeJSONForTest(t *testing.T, body string, dest any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), dest); err != nil {
		t.Fatalf("decode json %s: %v", body, err)
	}
}

var _ = response.OK
