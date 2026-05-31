package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func mustParseUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}

func newGovernanceHandlerTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	r, _ := newGovernanceHandlerTestRouterWithDB(t)
	return r
}

func newGovernanceHandlerTestRouterWithDB(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.CapabilityDefinition{}, &model.PolicyRule{}, &model.PolicyDecision{}, &model.KillSwitch{}, &model.ApprovalReceipt{}, &model.GovernanceScanResult{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	h := NewGovernanceHandler(service.NewGovernanceService(repository.NewGovernanceRepo(db)))
	r := gin.New()
	admin := r.Group("/api/v1/admin")
	admin.POST("/governance/capabilities", h.CreateCapability)
	admin.GET("/governance/capabilities", h.ListCapabilities)
	admin.POST("/governance/policy-rules", h.CreatePolicyRule)
	admin.POST("/governance/kill-switches", h.CreateKillSwitch)
	admin.POST("/governance/evaluate", h.Evaluate)
	admin.GET("/governance/summary", h.Summary)
	admin.GET("/governance/policy-decisions", h.ListPolicyDecisions)
	admin.GET("/governance/policy-decisions/:id", h.GetPolicyDecision)
	admin.POST("/governance/approval-receipts", h.CreateApprovalReceipt)
	admin.GET("/governance/approval-receipts", h.ListApprovalReceipts)
	admin.GET("/governance/scan-results", h.ListScanResults)
	admin.POST("/governance/scan-results/:id/resolve", func(c *gin.Context) {
		c.Set("user_id", mustParseUUID(t, "22222222-2222-2222-2222-222222222222"))
		h.ResolveScanResult(c)
	})
	r.POST("/api/v1/governance/approval-receipts/consume", h.ConsumeApprovalReceipt)
	return r, db
}

func TestGovernanceHandlerCreateListAndEvaluate(t *testing.T) {
	r := newGovernanceHandlerTestRouter(t)

	capabilityBody := `{"key":"sage.permission.payments.write","name":"Payment Write","risk_level":"high","description":"payment-affecting writes"}`
	capabilityRec := httptest.NewRecorder()
	r.ServeHTTP(capabilityRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/capabilities", bytes.NewBufferString(capabilityBody)))
	if capabilityRec.Code != http.StatusCreated {
		t.Fatalf("expected capability create 201, got %d body=%s", capabilityRec.Code, capabilityRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/governance/capabilities", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected capability list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listBody struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				Key string `json:"key"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listBody.Data.Total != 1 || len(listBody.Data.Items) != 1 || listBody.Data.Items[0].Key != "sage.permission.payments.write" {
		t.Fatalf("unexpected capability list: %+v", listBody)
	}

	ruleBody := `{"name":"Payments require user approval","capability_key":"sage.permission.payments.write","subject_type":"sage_permission_grant","effect":"require_user_approval","priority":10}`
	ruleRec := httptest.NewRecorder()
	r.ServeHTTP(ruleRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/policy-rules", bytes.NewBufferString(ruleBody)))
	if ruleRec.Code != http.StatusCreated {
		t.Fatalf("expected policy rule create 201, got %d body=%s", ruleRec.Code, ruleRec.Body.String())
	}

	evalBody := `{"subject_type":"sage_permission_grant","subject_id":"grant-1","capability_key":"sage.permission.payments.write","risk_level":"high"}`
	evalRec := httptest.NewRecorder()
	r.ServeHTTP(evalRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/evaluate", bytes.NewBufferString(evalBody)))
	if evalRec.Code != http.StatusOK {
		t.Fatalf("expected evaluate 200, got %d body=%s", evalRec.Code, evalRec.Body.String())
	}
	var evalResp struct {
		Data struct {
			Decision string `json:"decision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evalRec.Body.Bytes(), &evalResp); err != nil {
		t.Fatalf("decode evaluate: %v", err)
	}
	if evalResp.Data.Decision != service.GovernanceDecisionRequireUserApproval {
		t.Fatalf("expected require_user_approval, got %+v", evalResp)
	}
}

func TestGovernanceHandlerKillSwitchOverridesPolicy(t *testing.T) {
	r := newGovernanceHandlerTestRouter(t)

	for _, req := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/admin/governance/capabilities", `{"key":"sage.plugin.install","name":"Install Plugin","risk_level":"medium"}`},
		{http.MethodPost, "/api/v1/admin/governance/policy-rules", `{"name":"Allow install","capability_key":"sage.plugin.install","subject_type":"sage_plugin","effect":"allow","priority":10}`},
		{http.MethodPost, "/api/v1/admin/governance/kill-switches", `{"scope_type":"plugin","scope_id":"plugin-1","reason":"malicious plugin"}`},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(req.method, req.path, bytes.NewBufferString(req.body)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected create %s to return 201, got %d body=%s", req.path, rec.Code, rec.Body.String())
		}
	}

	evalRec := httptest.NewRecorder()
	r.ServeHTTP(evalRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/evaluate", bytes.NewBufferString(`{"subject_type":"sage_plugin","subject_id":"plugin-1","capability_key":"sage.plugin.install","risk_level":"medium"}`)))
	if evalRec.Code != http.StatusOK {
		t.Fatalf("expected evaluate 200, got %d body=%s", evalRec.Code, evalRec.Body.String())
	}
	var evalResp struct {
		Data struct {
			Decision string `json:"decision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evalRec.Body.Bytes(), &evalResp); err != nil {
		t.Fatalf("decode evaluate: %v", err)
	}
	if evalResp.Data.Decision != service.GovernanceDecisionDeny {
		t.Fatalf("expected deny from kill switch, got %+v", evalResp)
	}
}

func TestGovernanceHandlerAdminReadAPIs(t *testing.T) {
	r := newGovernanceHandlerTestRouter(t)
	actorID := "11111111-1111-1111-1111-111111111111"

	evalRec := httptest.NewRecorder()
	r.ServeHTTP(evalRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/evaluate", bytes.NewBufferString(`{"actor_user_id":"`+actorID+`","subject_type":"sage_permission_grant","subject_id":"grant-1","capability_key":"sage.permission.payments.write","risk_level":"high"}`)))
	if evalRec.Code != http.StatusOK {
		t.Fatalf("expected evaluate 200, got %d body=%s", evalRec.Code, evalRec.Body.String())
	}
	var evalResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evalRec.Body.Bytes(), &evalResp); err != nil {
		t.Fatalf("decode evaluate: %v", err)
	}

	listDecisionsRec := httptest.NewRecorder()
	r.ServeHTTP(listDecisionsRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/governance/policy-decisions?subject_type=sage_permission_grant&decision=allow", nil))
	if listDecisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions list 200, got %d body=%s", listDecisionsRec.Code, listDecisionsRec.Body.String())
	}
	var decisionsResp struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID          string `json:"id"`
				SubjectType string `json:"subject_type"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listDecisionsRec.Body.Bytes(), &decisionsResp); err != nil {
		t.Fatalf("decode decisions list: %v", err)
	}
	if decisionsResp.Data.Total != 1 || len(decisionsResp.Data.Items) != 1 || decisionsResp.Data.Items[0].ID != evalResp.Data.ID || decisionsResp.Data.Items[0].SubjectType != "sage_permission_grant" {
		t.Fatalf("unexpected decisions list: %+v", decisionsResp)
	}

	getDecisionRec := httptest.NewRecorder()
	r.ServeHTTP(getDecisionRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/governance/policy-decisions/"+evalResp.Data.ID, nil))
	if getDecisionRec.Code != http.StatusOK {
		t.Fatalf("expected decision get 200, got %d body=%s", getDecisionRec.Code, getDecisionRec.Body.String())
	}

	createRec := httptest.NewRecorder()
	createBody := `{"policy_decision_id":"` + evalResp.Data.ID + `","actor_user_id":"` + actorID + `","subject_type":"sage_permission_grant","subject_id":"grant-1","capability_key":"sage.permission.payments.write","decision":"require_user_approval","expires_in_seconds":900}`
	r.ServeHTTP(createRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/approval-receipts", bytes.NewBufferString(createBody)))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create receipt 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	listReceiptsRec := httptest.NewRecorder()
	r.ServeHTTP(listReceiptsRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/governance/approval-receipts?status=pending&subject_id=grant-1", nil))
	if listReceiptsRec.Code != http.StatusOK {
		t.Fatalf("expected receipts list 200, got %d body=%s", listReceiptsRec.Code, listReceiptsRec.Body.String())
	}
	var receiptsResp struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listReceiptsRec.Body.Bytes(), &receiptsResp); err != nil {
		t.Fatalf("decode receipts list: %v", err)
	}
	if receiptsResp.Data.Total != 1 || len(receiptsResp.Data.Items) != 1 || receiptsResp.Data.Items[0].Status != service.GovernanceApprovalPending {
		t.Fatalf("unexpected receipts list: %+v", receiptsResp)
	}

	summaryRec := httptest.NewRecorder()
	r.ServeHTTP(summaryRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/governance/summary?window=24", nil))
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("expected summary 200, got %d body=%s", summaryRec.Code, summaryRec.Body.String())
	}
	var summaryResp struct {
		Data struct {
			WindowHours          int   `json:"window_hours"`
			PolicyDecisionTotal  int64 `json:"policy_decision_total"`
			ApprovalReceiptTotal int64 `json:"approval_receipt_total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(summaryRec.Body.Bytes(), &summaryResp); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summaryResp.Data.WindowHours != 24 || summaryResp.Data.PolicyDecisionTotal != 1 || summaryResp.Data.ApprovalReceiptTotal != 1 {
		t.Fatalf("unexpected summary: %+v", summaryResp)
	}
}

func TestGovernanceHandlerScanResultListAndResolve(t *testing.T) {
	r, db := newGovernanceHandlerTestRouterWithDB(t)
	scanID := mustParseUUID(t, "33333333-3333-3333-3333-333333333333")
	if err := db.Create(&model.GovernanceScanResult{Base: model.Base{ID: scanID}, SubjectType: service.GovernanceSubjectSAGEPlugin, SubjectID: "plugin-1", Scanner: "sage_manifest", Severity: service.GovernanceRiskHigh, Status: "open", Message: "prompt injection phrase detected"}).Error; err != nil {
		t.Fatalf("seed scan result: %v", err)
	}

	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/governance/scan-results?status=open&severity=high", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected scan list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data struct {
			Total int `json:"total"`
			Items []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode scan list: %v", err)
	}
	if listResp.Data.Total != 1 || len(listResp.Data.Items) != 1 || listResp.Data.Items[0].ID != scanID.String() {
		t.Fatalf("unexpected scan list: %+v", listResp)
	}

	resolveRec := httptest.NewRecorder()
	r.ServeHTTP(resolveRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/scan-results/"+scanID.String()+"/resolve", nil))
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("expected scan resolve 200, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}
	var resolveResp struct {
		Data struct {
			Status     string  `json:"status"`
			ResolvedAt *string `json:"resolved_at"`
			ResolvedBy *string `json:"resolved_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resolveRec.Body.Bytes(), &resolveResp); err != nil {
		t.Fatalf("decode scan resolve: %v", err)
	}
	if resolveResp.Data.Status != "resolved" || resolveResp.Data.ResolvedAt == nil || resolveResp.Data.ResolvedBy == nil {
		t.Fatalf("unexpected resolved scan: %+v", resolveResp)
	}
}

func TestGovernanceHandlerApprovalReceiptRoutes(t *testing.T) {
	r := newGovernanceHandlerTestRouter(t)
	actorID := "11111111-1111-1111-1111-111111111111"

	evalRec := httptest.NewRecorder()
	r.ServeHTTP(evalRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/evaluate", bytes.NewBufferString(`{"actor_user_id":"`+actorID+`","subject_type":"sage_permission_grant","subject_id":"grant-1","capability_key":"sage.permission.payments.write","risk_level":"high"}`)))
	if evalRec.Code != http.StatusOK {
		t.Fatalf("expected evaluate 200, got %d body=%s", evalRec.Code, evalRec.Body.String())
	}
	var evalResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evalRec.Body.Bytes(), &evalResp); err != nil {
		t.Fatalf("decode evaluate: %v", err)
	}

	createRec := httptest.NewRecorder()
	createBody := `{"policy_decision_id":"` + evalResp.Data.ID + `","actor_user_id":"` + actorID + `","subject_type":"sage_permission_grant","subject_id":"grant-1","capability_key":"sage.permission.payments.write","decision":"require_user_approval","expires_in_seconds":900}`
	r.ServeHTTP(createRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/governance/approval-receipts", bytes.NewBufferString(createBody)))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create receipt 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ApprovalToken string `json:"approval_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if createResp.Data.ApprovalToken == "" {
		t.Fatalf("expected approval token, got %+v", createResp)
	}

	consumeBody := `{"approval_token":"` + createResp.Data.ApprovalToken + `","consumed_by":"device-a"}`
	consumeRec := httptest.NewRecorder()
	r.ServeHTTP(consumeRec, httptest.NewRequest(http.MethodPost, "/api/v1/governance/approval-receipts/consume", bytes.NewBufferString(consumeBody)))
	if consumeRec.Code != http.StatusOK {
		t.Fatalf("expected consume 200, got %d body=%s", consumeRec.Code, consumeRec.Body.String())
	}
	reuseRec := httptest.NewRecorder()
	r.ServeHTTP(reuseRec, httptest.NewRequest(http.MethodPost, "/api/v1/governance/approval-receipts/consume", bytes.NewBufferString(consumeBody)))
	if reuseRec.Code != http.StatusBadRequest {
		t.Fatalf("expected reuse 400, got %d body=%s", reuseRec.Code, reuseRec.Body.String())
	}
}
