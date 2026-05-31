package handler

import (
	"errors"
	"strconv"
	"time"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GovernanceHandler struct {
	svc *service.GovernanceService
}

func NewGovernanceHandler(svc *service.GovernanceService) *GovernanceHandler {
	return &GovernanceHandler{svc: svc}
}

func (h *GovernanceHandler) CreateCapability(c *gin.Context) {
	var req struct {
		Key         string         `json:"key"`
		Name        string         `json:"name"`
		Description string         `json:"description"`
		RiskLevel   string         `json:"risk_level"`
		Status      string         `json:"status"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	capability, err := h.svc.CreateCapability(service.CreateCapabilityInput{Key: req.Key, Name: req.Name, Description: req.Description, RiskLevel: req.RiskLevel, Status: req.Status, Metadata: req.Metadata})
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.Created(c, capability)
}

func (h *GovernanceHandler) ListCapabilities(c *gin.Context) {
	page, err := h.svc.ListCapabilities(parseGovernanceIntQuery(c, "limit", 50), parseGovernanceIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, page)
}

func (h *GovernanceHandler) CreatePolicyRule(c *gin.Context) {
	var req struct {
		Name          string         `json:"name"`
		Description   string         `json:"description"`
		CapabilityKey string         `json:"capability_key"`
		SubjectType   string         `json:"subject_type"`
		SubjectID     string         `json:"subject_id"`
		ActorUserID   string         `json:"actor_user_id"`
		RiskLevel     string         `json:"risk_level"`
		Effect        string         `json:"effect"`
		Priority      int            `json:"priority"`
		Status        string         `json:"status"`
		Conditions    map[string]any `json:"conditions"`
		Metadata      map[string]any `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	actorUserID, ok := parseOptionalUUID(c, req.ActorUserID, "actor_user_id")
	if !ok {
		return
	}
	rule, err := h.svc.CreatePolicyRule(service.CreatePolicyRuleInput{Name: req.Name, Description: req.Description, CapabilityKey: req.CapabilityKey, SubjectType: req.SubjectType, SubjectID: req.SubjectID, ActorUserID: actorUserID, RiskLevel: req.RiskLevel, Effect: req.Effect, Priority: req.Priority, Status: req.Status, Conditions: req.Conditions, Metadata: req.Metadata})
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.Created(c, rule)
}

func (h *GovernanceHandler) ListPolicyRules(c *gin.Context) {
	page, err := h.svc.ListPolicyRules(parseGovernanceIntQuery(c, "limit", 50), parseGovernanceIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, page)
}

func (h *GovernanceHandler) CreateKillSwitch(c *gin.Context) {
	var req struct {
		ScopeType string         `json:"scope_type"`
		ScopeID   string         `json:"scope_id"`
		Reason    string         `json:"reason"`
		Status    string         `json:"status"`
		ExpiresAt string         `json:"expires_at"`
		CreatedBy string         `json:"created_by"`
		Metadata  map[string]any `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	createdBy, ok := parseOptionalUUID(c, req.CreatedBy, "created_by")
	if !ok {
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			response.BadRequest(c, "invalid expires_at")
			return
		}
		expiresAt = &parsed
	}
	killSwitch, err := h.svc.CreateKillSwitch(service.CreateKillSwitchInput{ScopeType: req.ScopeType, ScopeID: req.ScopeID, Reason: req.Reason, Status: req.Status, ExpiresAt: expiresAt, CreatedBy: createdBy, Metadata: req.Metadata})
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.Created(c, killSwitch)
}

func (h *GovernanceHandler) Evaluate(c *gin.Context) {
	var req struct {
		ActorUserID   string         `json:"actor_user_id"`
		ActorDeviceID string         `json:"actor_device_id"`
		SubjectType   string         `json:"subject_type"`
		SubjectID     string         `json:"subject_id"`
		CapabilityKey string         `json:"capability_key"`
		RiskLevel     string         `json:"risk_level"`
		Context       map[string]any `json:"context"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	actorUserID, ok := parseOptionalUUID(c, req.ActorUserID, "actor_user_id")
	if !ok {
		return
	}
	decision, err := h.svc.Evaluate(service.EvaluatePolicyInput{ActorUserID: actorUserID, ActorDeviceID: req.ActorDeviceID, SubjectType: req.SubjectType, SubjectID: req.SubjectID, CapabilityKey: req.CapabilityKey, RiskLevel: req.RiskLevel, Context: req.Context})
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.OK(c, decision)
}

func (h *GovernanceHandler) CreateApprovalReceipt(c *gin.Context) {
	var req struct {
		PolicyDecisionID string         `json:"policy_decision_id"`
		ActorUserID      string         `json:"actor_user_id"`
		SubjectType      string         `json:"subject_type"`
		SubjectID        string         `json:"subject_id"`
		CapabilityKey    string         `json:"capability_key"`
		Decision         string         `json:"decision"`
		ExpiresInSeconds int            `json:"expires_in_seconds"`
		Metadata         map[string]any `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	decisionID, err := uuid.Parse(req.PolicyDecisionID)
	if err != nil {
		response.BadRequest(c, "invalid policy_decision_id")
		return
	}
	actorID, err := uuid.Parse(req.ActorUserID)
	if err != nil {
		response.BadRequest(c, "invalid actor_user_id")
		return
	}
	ttl := req.ExpiresInSeconds
	if ttl <= 0 || ttl > 3600 {
		ttl = 900
	}
	receipt, token, err := h.svc.CreateApprovalReceipt(service.CreateApprovalReceiptInput{PolicyDecisionID: decisionID, ActorUserID: actorID, SubjectType: req.SubjectType, SubjectID: req.SubjectID, CapabilityKey: req.CapabilityKey, Decision: req.Decision, ExpiresAt: time.Now().UTC().Add(time.Duration(ttl) * time.Second), Metadata: req.Metadata})
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.Created(c, gin.H{"receipt": receipt, "approval_token": token})
}

func (h *GovernanceHandler) Summary(c *gin.Context) {
	summary, err := h.svc.Summary(parseGovernanceIntQuery(c, "window", 24))
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.OK(c, summary)
}

func (h *GovernanceHandler) ListPolicyDecisions(c *gin.Context) {
	page, err := h.svc.ListPolicyDecisions(service.ListPolicyDecisionsInput{SubjectType: c.Query("subject_type"), SubjectID: c.Query("subject_id"), CapabilityKey: c.Query("capability_key"), Decision: c.Query("decision"), RiskLevel: c.Query("risk_level")}, parseGovernanceIntQuery(c, "limit", 50), parseGovernanceIntQuery(c, "offset", 0))
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.OK(c, page)
}

func (h *GovernanceHandler) GetPolicyDecision(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid policy decision id")
		return
	}
	decision, err := h.svc.GetPolicyDecision(id)
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.OK(c, decision)
}

func (h *GovernanceHandler) ListApprovalReceipts(c *gin.Context) {
	page, err := h.svc.ListApprovalReceipts(service.ListApprovalReceiptsInput{SubjectType: c.Query("subject_type"), SubjectID: c.Query("subject_id"), CapabilityKey: c.Query("capability_key"), Status: c.Query("status")}, parseGovernanceIntQuery(c, "limit", 50), parseGovernanceIntQuery(c, "offset", 0))
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.OK(c, page)
}

func (h *GovernanceHandler) RevokeApprovalReceipt(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid approval receipt id")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	receipt, err := h.svc.RevokeApprovalReceipt(id, req.Reason)
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.OK(c, receipt)
}

func (h *GovernanceHandler) ListScanResults(c *gin.Context) {
	page, err := h.svc.ListScanResults(service.ListScanResultsInput{SubjectType: c.Query("subject_type"), SubjectID: c.Query("subject_id"), Scanner: c.Query("scanner"), Severity: c.Query("severity"), Status: c.Query("status")}, parseGovernanceIntQuery(c, "limit", 50), parseGovernanceIntQuery(c, "offset", 0))
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.OK(c, page)
}

func (h *GovernanceHandler) ResolveScanResult(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid scan result id")
		return
	}
	result, err := h.svc.ResolveScanResult(id, middleware.MustGetUserID(c))
	if err != nil {
		h.writeGovernanceError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *GovernanceHandler) ConsumeApprovalReceipt(c *gin.Context) {
	var req struct {
		ApprovalToken string `json:"approval_token"`
		ConsumedBy    string `json:"consumed_by"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	receipt, err := h.svc.ConsumeApprovalReceipt(req.ApprovalToken, req.ConsumedBy)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, receipt)
}

func (h *GovernanceHandler) writeGovernanceError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrGovernanceInvalidInput) {
		response.BadRequest(c, err.Error())
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(c, err.Error())
		return
	}
	response.InternalError(c, err.Error())
}

func parseGovernanceIntQuery(c *gin.Context, key string, fallback int) int {
	raw := c.Query(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseOptionalUUID(c *gin.Context, raw, field string) (*uuid.UUID, bool) {
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.BadRequest(c, "invalid "+field)
		return nil, false
	}
	return &id, true
}
