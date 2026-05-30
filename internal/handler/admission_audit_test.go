package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAdmissionAdminMutationRecordsAuditEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.ServerAdmission{}, &model.AdmissionRequest{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	admissionSvc := service.NewAdmissionServiceWithRepo(userRepo, repository.NewAdmissionRepo(db), config.AdmissionConfig{PolicyType: "protocol"})
	auditSvc := service.NewAuditService(repository.NewAuditRepo(db))
	admissionH := NewAdmissionHandler(admissionSvc)
	admissionH.SetAuditService(auditSvc)
	auditH := NewAuditHandler(auditSvc)
	admin := &model.User{PubKeyEd25519: "admin-pubkey", DisplayName: "Admin", Status: "active", Role: "admin", IsAdmin: true}
	if err := userRepo.Create(admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	secret := "admission-audit-secret"
	token, err := middleware.GenerateToken(secret, admin.ID, "admin-device", "agent-os-test", 60)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	r := gin.New()
	adminRoutes := r.Group("/api/v1/admin")
	adminRoutes.Use(middleware.JWTAuth(secret))
	adminRoutes.Use(middleware.AdminMiddleware(userRepo))
	adminRoutes.PUT("/admission/policy", admissionH.UpdatePolicy)
	adminRoutes.GET("/audit/events", auditH.List)

	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/admission/policy", strings.NewReader(`{"policy_type":"approval"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer "+token)
	updateW := httptest.NewRecorder()
	r.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", updateW.Code, updateW.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit/events?action="+service.AuditActionAdmissionPolicyUpdated, nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected audit list 200, got %d body=%s", listW.Code, listW.Body.String())
	}
	var listResp responseEnvelope[service.AuditEventsPage]
	decodeJSONForTest(t, listW.Body.String(), &listResp)
	if listResp.Data.Total != 1 || len(listResp.Data.Items) != 1 {
		t.Fatalf("expected one audit event, got %+v", listResp.Data)
	}
	event := listResp.Data.Items[0]
	if event.ActorUserID == nil || *event.ActorUserID != admin.ID || event.ActorDeviceID != "admin-device" || event.Outcome != service.AuditOutcomeSuccess {
		t.Fatalf("unexpected audit actor/outcome: %+v", event)
	}
	if event.Metadata["policy_type"] != "approval" {
		t.Fatalf("expected policy metadata, got %+v", event.Metadata)
	}
}

var _ = response.OK
