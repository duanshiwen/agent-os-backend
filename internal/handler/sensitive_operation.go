package handler

import (
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type SensitiveOperationHandler struct {
	svc *service.SensitiveOperationService
}

func NewSensitiveOperationHandler(svc *service.SensitiveOperationService) *SensitiveOperationHandler {
	return &SensitiveOperationHandler{svc: svc}
}

// POST /api/v1/users/me/password
func (h *SensitiveOperationHandler) SetPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.SetPassword(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), req.Password); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"status": "password_set"})
}

// PUT /api/v1/users/me/password
func (h *SensitiveOperationHandler) ChangePassword(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.ChangePassword(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), req.CurrentPassword, req.NewPassword); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"status": "password_changed"})
}

// POST /api/v1/sensitive-operations/confirmations
func (h *SensitiveOperationHandler) IssueConfirmation(c *gin.Context) {
	var req struct {
		Password  string `json:"password" binding:"required"`
		Operation string `json:"operation" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	confirmation, err := h.svc.IssueConfirmation(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), req.Password, req.Operation)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, confirmation)
}
