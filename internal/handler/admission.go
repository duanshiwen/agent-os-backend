package handler

import (
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type AdmissionHandler struct {
	svc                   *service.AdmissionService
	auditSvc              *service.AuditService
	sensitiveOperationSvc *service.SensitiveOperationService
}

func NewAdmissionHandler(svc *service.AdmissionService) *AdmissionHandler {
	return &AdmissionHandler{svc: svc}
}

func (h *AdmissionHandler) SetAuditService(auditSvc *service.AuditService) {
	h.auditSvc = auditSvc
}

func (h *AdmissionHandler) SetSensitiveOperationService(svc *service.SensitiveOperationService) {
	h.sensitiveOperationSvc = svc
}

// GET /api/v1/admin/admission/policy
func (h *AdmissionHandler) GetPolicy(c *gin.Context) {
	policy, err := h.svc.GetPolicy("default")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, policy)
}

// PUT /api/v1/admin/admission/policy
func (h *AdmissionHandler) UpdatePolicy(c *gin.Context) {
	var req struct {
		PolicyType        string `json:"policy_type" binding:"required"`
		ConfirmationToken string `json:"confirmation_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	adminID := middleware.MustGetUserID(c)
	if !enforceSensitiveConfirmation(c, h.sensitiveOperationSvc, h.auditSvc, req.ConfirmationToken, service.SensitiveOperationAdmissionPolicyUpdate, "admission.policy.update", service.AuditActionAdmissionPolicyUpdated, "server_admission", "default") {
		return
	}
	policy, err := h.svc.UpdatePolicy("default", req.PolicyType, adminID)
	if err != nil {
		h.recordAudit(c, service.AuditActionAdmissionPolicyUpdated, "server_admission", "default", service.AuditOutcomeFailure, gin.H{"policy_type": req.PolicyType, "error": err.Error()})
		response.BadRequest(c, err.Error())
		return
	}
	h.recordAudit(c, service.AuditActionAdmissionPolicyUpdated, "server_admission", "default", service.AuditOutcomeSuccess, gin.H{"policy_type": policy.PolicyType})
	response.OK(c, policy)
}

// PUT /api/v1/admin/admission/invitation-code
func (h *AdmissionHandler) UpdateInvitationCode(c *gin.Context) {
	var req struct {
		InvitationCode    string `json:"invitation_code" binding:"required"`
		ConfirmationToken string `json:"confirmation_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	adminID := middleware.MustGetUserID(c)
	if !enforceSensitiveConfirmation(c, h.sensitiveOperationSvc, h.auditSvc, req.ConfirmationToken, service.SensitiveOperationAdmissionInvitationUpdate, "admission.invitation_code.update", service.AuditActionAdmissionInvitationCodeSet, "server_admission", "default") {
		return
	}
	policy, err := h.svc.UpdateInvitationCode("default", req.InvitationCode, adminID)
	if err != nil {
		h.recordAudit(c, service.AuditActionAdmissionInvitationCodeSet, "server_admission", "default", service.AuditOutcomeFailure, gin.H{"error": err.Error()})
		response.BadRequest(c, err.Error())
		return
	}
	h.recordAudit(c, service.AuditActionAdmissionInvitationCodeSet, "server_admission", "default", service.AuditOutcomeSuccess, gin.H{"policy_type": policy.PolicyType})
	response.OK(c, policy)
}

// GET /api/v1/admin/admission/requests
func (h *AdmissionHandler) GetPending(c *gin.Context) {
	reqs, err := h.svc.GetPendingRequests()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, reqs)
}

// POST /api/v1/admin/admission/requests/:id/approve
func (h *AdmissionHandler) Approve(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	if err := h.svc.ApproveRequest(id); err != nil {
		h.recordAudit(c, service.AuditActionAdmissionRequestApproved, "admission_request", id.String(), service.AuditOutcomeFailure, gin.H{"error": err.Error()})
		response.BadRequest(c, err.Error())
		return
	}
	h.recordAudit(c, service.AuditActionAdmissionRequestApproved, "admission_request", id.String(), service.AuditOutcomeSuccess, nil)
	response.OK(c, gin.H{"status": "approved"})
}

// POST /api/v1/admin/admission/requests/:id/reject
func (h *AdmissionHandler) Reject(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)

	if err := h.svc.RejectRequest(id, req.Reason); err != nil {
		h.recordAudit(c, service.AuditActionAdmissionRequestRejected, "admission_request", id.String(), service.AuditOutcomeFailure, gin.H{"error": err.Error(), "reason_present": req.Reason != ""})
		response.BadRequest(c, err.Error())
		return
	}
	h.recordAudit(c, service.AuditActionAdmissionRequestRejected, "admission_request", id.String(), service.AuditOutcomeSuccess, gin.H{"reason_present": req.Reason != ""})
	response.OK(c, gin.H{"status": "rejected"})
}

func (h *AdmissionHandler) recordAudit(c *gin.Context, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	if h.auditSvc == nil {
		return
	}
	actorID := middleware.MustGetUserID(c)
	_, _ = h.auditSvc.Record(service.RecordAuditEventInput{
		ActorUserID:   &actorID,
		ActorDeviceID: middleware.MustGetDeviceID(c),
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Outcome:       outcome,
		IPAddress:     c.ClientIP(),
		UserAgent:     c.Request.UserAgent(),
		Metadata:      metadata,
	})
}
