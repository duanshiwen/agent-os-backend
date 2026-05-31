package service

import (
	"errors"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
)

const (
	GovernanceEnforcementModeDisabled = "disabled"
	GovernanceEnforcementModeObserve  = "observe"
	GovernanceEnforcementModeEnforce  = "enforce"
)

var ErrGovernanceDenied = errors.New("governance denied")
var ErrGovernanceApprovalRequired = errors.New("governance approval required")
var ErrGovernanceApprovalInvalid = errors.New("governance approval invalid")

type GovernanceEnforcementConfig struct {
	Mode       string
	SAGEMode   string
	ObjectMode string
	KBMode     string
}

type GovernanceEnforcer struct {
	governance *GovernanceService
	config     GovernanceEnforcementConfig
}

func NewGovernanceEnforcer(governance *GovernanceService, cfg GovernanceEnforcementConfig) *GovernanceEnforcer {
	cfg.Mode = normalizeGovernanceEnforcementMode(cfg.Mode)
	if cfg.Mode == "" {
		cfg.Mode = GovernanceEnforcementModeObserve
	}
	cfg.SAGEMode = normalizeGovernanceEnforcementMode(cfg.SAGEMode)
	cfg.ObjectMode = normalizeGovernanceEnforcementMode(cfg.ObjectMode)
	cfg.KBMode = normalizeGovernanceEnforcementMode(cfg.KBMode)
	return &GovernanceEnforcer{governance: governance, config: cfg}
}

type GovernanceEnforcementInput struct {
	ActorUserID        *uuid.UUID
	ActorDeviceID      string
	SubjectType        string
	SubjectID          string
	CapabilityKey      string
	RiskLevel          string
	Context            map[string]any
	ApprovalToken      string
	ApprovalConsumedBy string
}

type GovernanceEnforcementResult struct {
	Allowed                 bool                   `json:"allowed"`
	Mode                    string                 `json:"mode"`
	WouldHaveBlocked        bool                   `json:"would_have_blocked"`
	Decision                *model.PolicyDecision  `json:"decision,omitempty"`
	ApprovalReceipt         *model.ApprovalReceipt `json:"approval_receipt,omitempty"`
	ApprovalToken           string                 `json:"approval_token,omitempty"`
	ConsumedApprovalReceipt *model.ApprovalReceipt `json:"consumed_approval_receipt,omitempty"`
}

func (e *GovernanceEnforcer) Enforce(input GovernanceEnforcementInput) (*GovernanceEnforcementResult, error) {
	if e == nil || e.governance == nil {
		return nil, ErrGovernanceInvalidInput
	}
	mode := e.modeFor(input.SubjectType)
	if mode == GovernanceEnforcementModeDisabled {
		return &GovernanceEnforcementResult{Allowed: true, Mode: mode}, nil
	}
	if input.ApprovalToken != "" {
		consumedBy := strings.TrimSpace(input.ApprovalConsumedBy)
		if consumedBy == "" {
			consumedBy = strings.TrimSpace(input.ActorDeviceID)
		}
		receipt, err := e.governance.ConsumeApprovalReceipt(input.ApprovalToken, consumedBy)
		if err != nil {
			if mode == GovernanceEnforcementModeObserve {
				return &GovernanceEnforcementResult{Allowed: true, Mode: mode, WouldHaveBlocked: true}, nil
			}
			return nil, ErrGovernanceApprovalInvalid
		}
		return &GovernanceEnforcementResult{Allowed: true, Mode: mode, ConsumedApprovalReceipt: receipt}, nil
	}
	decision, err := e.governance.Evaluate(EvaluatePolicyInput{ActorUserID: input.ActorUserID, ActorDeviceID: input.ActorDeviceID, SubjectType: input.SubjectType, SubjectID: input.SubjectID, CapabilityKey: input.CapabilityKey, RiskLevel: input.RiskLevel, Context: input.Context})
	if err != nil {
		return nil, err
	}
	wouldBlock := decision.Decision != GovernanceDecisionAllow
	if mode == GovernanceEnforcementModeObserve {
		return &GovernanceEnforcementResult{Allowed: true, Mode: mode, WouldHaveBlocked: wouldBlock, Decision: decision}, nil
	}
	switch decision.Decision {
	case GovernanceDecisionAllow:
		return &GovernanceEnforcementResult{Allowed: true, Mode: mode, Decision: decision}, nil
	case GovernanceDecisionDeny:
		return &GovernanceEnforcementResult{Allowed: false, Mode: mode, WouldHaveBlocked: true, Decision: decision}, ErrGovernanceDenied
	case GovernanceDecisionRequireUserApproval, GovernanceDecisionRequireAdminApproval:
		result := &GovernanceEnforcementResult{Allowed: false, Mode: mode, WouldHaveBlocked: true, Decision: decision}
		if input.ActorUserID != nil && *input.ActorUserID != uuid.Nil {
			receipt, token, err := e.governance.CreateApprovalReceipt(CreateApprovalReceiptInput{PolicyDecisionID: decision.ID, ActorUserID: *input.ActorUserID, SubjectType: input.SubjectType, SubjectID: input.SubjectID, CapabilityKey: input.CapabilityKey, Decision: decision.Decision, ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Metadata: map[string]any{"actor_device_id": input.ActorDeviceID, "risk_level": input.RiskLevel}})
			if err != nil {
				return nil, err
			}
			result.ApprovalReceipt = receipt
			result.ApprovalToken = token
		}
		return result, ErrGovernanceApprovalRequired
	default:
		return &GovernanceEnforcementResult{Allowed: false, Mode: mode, WouldHaveBlocked: true, Decision: decision}, ErrGovernanceDenied
	}
}

func (e *GovernanceEnforcer) modeFor(subjectType string) string {
	switch strings.TrimSpace(subjectType) {
	case GovernanceSubjectSAGEPlugin, GovernanceSubjectSAGEPermissionGrant, GovernanceSubjectSAGEInvocation:
		if e.config.SAGEMode != "" {
			return e.config.SAGEMode
		}
	case GovernanceSubjectObjectOperation:
		if e.config.ObjectMode != "" {
			return e.config.ObjectMode
		}
	case GovernanceSubjectKBOperation:
		if e.config.KBMode != "" {
			return e.config.KBMode
		}
	}
	if e.config.Mode != "" {
		return e.config.Mode
	}
	return GovernanceEnforcementModeObserve
}

func normalizeGovernanceEnforcementMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case GovernanceEnforcementModeDisabled, GovernanceEnforcementModeObserve, GovernanceEnforcementModeEnforce:
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return ""
	}
}
