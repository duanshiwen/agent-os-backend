package handler

import (
	"errors"
	"net/http"

	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	GovernanceErrorCodeDenied           = "governance_denied"
	GovernanceErrorCodeApprovalRequired = "governance_approval_required"
	GovernanceErrorCodeApprovalInvalid  = "governance_approval_invalid"
)

type governanceErrorDetails struct {
	PolicyDecisionID  string `json:"policy_decision_id,omitempty"`
	SubjectType       string `json:"subject_type,omitempty"`
	SubjectID         string `json:"subject_id,omitempty"`
	CapabilityKey     string `json:"capability_key,omitempty"`
	RiskLevel         string `json:"risk_level,omitempty"`
	Decision          string `json:"decision,omitempty"`
	Reason            string `json:"reason,omitempty"`
	ApprovalReceiptID string `json:"approval_receipt_id,omitempty"`
	ApprovalExpiresAt string `json:"approval_expires_at,omitempty"`
}

func handleGovernanceError(c *gin.Context, err error) bool {
	details := buildGovernanceErrorDetails(err)
	switch {
	case errors.Is(err, service.ErrGovernanceDenied):
		response.ErrorWithCode(c, http.StatusForbidden, GovernanceErrorCodeDenied, err.Error(), details)
		return true
	case errors.Is(err, service.ErrGovernanceApprovalRequired):
		response.ErrorWithCode(c, http.StatusPreconditionRequired, GovernanceErrorCodeApprovalRequired, err.Error(), details)
		return true
	case errors.Is(err, service.ErrGovernanceApprovalInvalid):
		response.ErrorWithCode(c, http.StatusForbidden, GovernanceErrorCodeApprovalInvalid, err.Error(), details)
		return true
	default:
		return false
	}
}

func buildGovernanceErrorDetails(err error) any {
	var enforcementErr *service.GovernanceEnforcementError
	if !errors.As(err, &enforcementErr) || enforcementErr == nil {
		return nil
	}
	details := governanceErrorDetails{
		SubjectType:   enforcementErr.Input.SubjectType,
		SubjectID:     enforcementErr.Input.SubjectID,
		CapabilityKey: enforcementErr.Input.CapabilityKey,
		RiskLevel:     enforcementErr.Input.RiskLevel,
	}
	if enforcementErr.Result != nil {
		if d := enforcementErr.Result.Decision; d != nil {
			details.PolicyDecisionID = d.ID.String()
			details.SubjectType = firstNonEmpty(details.SubjectType, d.SubjectType)
			details.SubjectID = firstNonEmpty(details.SubjectID, d.SubjectID)
			details.CapabilityKey = firstNonEmpty(details.CapabilityKey, d.CapabilityKey)
			details.RiskLevel = firstNonEmpty(details.RiskLevel, d.RiskLevel)
			details.Decision = d.Decision
			details.Reason = d.Reason
		}
		if r := enforcementErr.Result.ApprovalReceipt; r != nil {
			details.ApprovalReceiptID = r.ID.String()
			details.ApprovalExpiresAt = r.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		}
	}
	return details
}

func firstNonEmpty(current, fallback string) string {
	if current != "" {
		return current
	}
	return fallback
}
