package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIdentityHTTPRenameAndRevokeDeviceWithAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.AuthChallenge{}, &model.AdmissionRequest{}, &model.ServerAdmission{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	identitySvc := service.NewIdentityService(userRepo, config.JWTConfig{Secret: "device-management-secret", AccessTokenMins: 60, Issuer: "agent-os-test"}, &handlerStubVerifier{valid: true})
	auditSvc := service.NewAuditService(repository.NewAuditRepo(db))
	identityH := NewIdentityHandler(identitySvc)
	identityH.SetAuditService(auditSvc)
	auditH := NewAuditHandler(auditSvc)

	user := &model.User{PubKeyEd25519: "device-http-user-pubkey", DisplayName: "Device HTTP", Status: "active"}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := &model.Device{UserID: user.ID, DeviceID: "http-current", DeviceName: "Current", DevicePubKey: "current-pubkey", Status: "active", PairedAt: time.Now()}
	target := &model.Device{UserID: user.ID, DeviceID: "http-target", DeviceName: "Target", DevicePubKey: "target-pubkey", Status: "active", PairedAt: time.Now()}
	if err := userRepo.CreateDevice(current); err != nil {
		t.Fatalf("create current: %v", err)
	}
	if err := userRepo.CreateDevice(target); err != nil {
		t.Fatalf("create target: %v", err)
	}
	token, err := middleware.GenerateToken("device-management-secret", user.ID, "http-current", "agent-os-test", 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(middleware.JWTAuth("device-management-secret"))
	protected.PUT("/users/me/devices/:device_id", identityH.RenameDevice)
	protected.DELETE("/users/me/devices/:device_id", identityH.RevokeDevice)
	protected.GET("/admin/audit/events", auditH.List)

	renameReq := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/devices/http-target", strings.NewReader(`{"device_name":"Renamed HTTP"}`))
	renameReq.Header.Set("Content-Type", "application/json")
	renameReq.Header.Set("Authorization", "Bearer "+token)
	renameW := httptest.NewRecorder()
	r.ServeHTTP(renameW, renameReq)
	if renameW.Code != http.StatusOK {
		t.Fatalf("expected rename 200, got %d body=%s", renameW.Code, renameW.Body.String())
	}
	var renameResp responseEnvelope[model.Device]
	decodeJSONForTest(t, renameW.Body.String(), &renameResp)
	if renameResp.Data.DeviceName != "Renamed HTTP" {
		t.Fatalf("unexpected renamed device: %+v", renameResp.Data)
	}

	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/users/me/devices/http-target", nil)
	revokeReq.Header.Set("Authorization", "Bearer "+token)
	revokeW := httptest.NewRecorder()
	r.ServeHTTP(revokeW, revokeReq)
	if revokeW.Code != http.StatusOK {
		t.Fatalf("expected revoke 200, got %d body=%s", revokeW.Code, revokeW.Body.String())
	}
	var revokeResp responseEnvelope[model.Device]
	decodeJSONForTest(t, revokeW.Body.String(), &revokeResp)
	if revokeResp.Data.Status != "revoked" || revokeResp.Data.RevokedAt == nil {
		t.Fatalf("unexpected revoked device: %+v", revokeResp.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit/events?resource_type=device&limit=10", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected audit list 200, got %d body=%s", listW.Code, listW.Body.String())
	}
	var listResp responseEnvelope[service.AuditEventsPage]
	decodeJSONForTest(t, listW.Body.String(), &listResp)
	if listResp.Data.Total != 2 || len(listResp.Data.Items) != 2 {
		t.Fatalf("expected two device audit events, got %+v", listResp.Data)
	}
}
