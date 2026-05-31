package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuditHandlerVerifyHashChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	auditSvc := service.NewAuditService(repository.NewAuditRepo(db))
	if _, err := auditSvc.Record(service.RecordAuditEventInput{Action: service.AuditActionSAGEPluginCreated, ResourceType: "sage_plugin", ResourceID: "plugin-1"}); err != nil {
		t.Fatalf("record audit: %v", err)
	}

	r := gin.New()
	h := NewAuditHandler(auditSvc)
	r.GET("/api/v1/admin/audit/verify-chain", h.VerifyHashChain)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit/verify-chain", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Valid       bool     `json:"valid"`
			Checked     int      `json:"checked"`
			HeadHash    string   `json:"head_hash"`
			HeadSequence int64    `json:"head_sequence"`
			Breaks      []string `json:"breaks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || !body.Data.Valid || body.Data.Checked != 1 || body.Data.HeadSequence != 1 || body.Data.HeadHash == "" {
		t.Fatalf("unexpected verify response: %+v", body)
	}
}
