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

func handleGovernanceError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, service.ErrGovernanceDenied):
		response.ErrorWithCode(c, http.StatusForbidden, GovernanceErrorCodeDenied, err.Error(), nil)
		return true
	case errors.Is(err, service.ErrGovernanceApprovalRequired):
		response.ErrorWithCode(c, http.StatusPreconditionRequired, GovernanceErrorCodeApprovalRequired, err.Error(), nil)
		return true
	case errors.Is(err, service.ErrGovernanceApprovalInvalid):
		response.ErrorWithCode(c, http.StatusForbidden, GovernanceErrorCodeApprovalInvalid, err.Error(), nil)
		return true
	default:
		return false
	}
}
