package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRequestIDMiddlewarePropagatesInboundHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/ping", func(c *gin.Context) {
		if got := middleware.GetRequestID(c); got != "req-test-123" {
			t.Fatalf("expected request id in context, got %q", got)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(middleware.RequestIDHeader, "req-test-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get(middleware.RequestIDHeader); got != "req-test-123" {
		t.Fatalf("expected propagated request id, got %q", got)
	}
}

func TestReadinessHandlerReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	r := gin.New()
	r.GET("/ready", NewReadinessHandler(db).Ready)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected ready 200, got %d body=%s", w.Code, w.Body.String())
	}
}
