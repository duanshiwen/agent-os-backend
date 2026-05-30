package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSensitiveOperationHTTPPasswordAndConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.SensitiveOperationConfirmation{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	auditSvc := service.NewAuditService(repository.NewAuditRepo(db))
	svc := service.NewSensitiveOperationService(userRepo, repository.NewSensitiveOperationRepo(db), auditSvc)
	h := NewSensitiveOperationHandler(svc)
	user := &model.User{PubKeyEd25519: "sensitive-http-pubkey", DisplayName: "Sensitive HTTP", Status: "active"}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	secret := "sensitive-http-secret"
	token, err := middleware.GenerateToken(secret, user.ID, "sensitive-device", "agent-os-test", 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(middleware.JWTAuth(secret))
	protected.POST("/users/me/password", h.SetPassword)
	protected.PUT("/users/me/password", h.ChangePassword)
	protected.POST("/sensitive-operations/confirmations", h.IssueConfirmation)

	setReq := authedJSONReq(http.MethodPost, "/api/v1/users/me/password", `{"password":"secret123"}`, token)
	setW := httptest.NewRecorder()
	r.ServeHTTP(setW, setReq)
	if setW.Code != http.StatusOK {
		t.Fatalf("expected set password 200, got %d body=%s", setW.Code, setW.Body.String())
	}

	changeReq := authedJSONReq(http.MethodPut, "/api/v1/users/me/password", `{"current_password":"secret123","new_password":"secret456"}`, token)
	changeW := httptest.NewRecorder()
	r.ServeHTTP(changeW, changeReq)
	if changeW.Code != http.StatusOK {
		t.Fatalf("expected change password 200, got %d body=%s", changeW.Code, changeW.Body.String())
	}

	confirmReq := authedJSONReq(http.MethodPost, "/api/v1/sensitive-operations/confirmations", `{"password":"secret456","operation":"device.revoke"}`, token)
	confirmW := httptest.NewRecorder()
	r.ServeHTTP(confirmW, confirmReq)
	if confirmW.Code != http.StatusCreated {
		t.Fatalf("expected confirmation 201, got %d body=%s", confirmW.Code, confirmW.Body.String())
	}
	var confirmResp responseEnvelope[service.SensitiveConfirmationResponse]
	decodeJSONForTest(t, confirmW.Body.String(), &confirmResp)
	if confirmResp.Data.ConfirmationToken == "" || confirmResp.Data.Operation != "device.revoke" {
		t.Fatalf("unexpected confirmation response: %+v", confirmResp.Data)
	}
}

func authedJSONReq(method, path, body, token string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}
