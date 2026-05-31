package handler

import (
	"errors"
	"strconv"
	"time"

	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func (h *GovernanceHandler) writeGovernanceError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrGovernanceInvalidInput) {
		response.BadRequest(c, err.Error())
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
