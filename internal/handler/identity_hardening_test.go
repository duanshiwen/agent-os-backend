package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestIdentityHTTPRegisterIsGone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/auth/register", (&IdentityHandler{}).Register)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"pubkey_ed25519":"pubkey","display_name":"User"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestIdentityHTTPDirectPairDeviceIsGone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewIdentityHandler(&service.IdentityService{})
	protected := r.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		c.Set("user_id", uuid.New())
		c.Set("device_id", "old-device")
		c.Next()
	})
	protected.POST("/users/me/devices", h.PairDevice)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/devices", bytes.NewBufferString(`{"device_id":"new-device","device_pubkey":"pubkey"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone, got %d body=%s", w.Code, w.Body.String())
	}
}

var _ = middleware.MustGetUserID
var _ = model.User{}
