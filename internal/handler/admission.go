package handler

import (
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type AdmissionHandler struct {
	svc *service.AdmissionService
}

func NewAdmissionHandler(svc *service.AdmissionService) *AdmissionHandler {
	return &AdmissionHandler{svc: svc}
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
		response.BadRequest(c, err.Error())
		return
	}
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
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"status": "rejected"})
}
