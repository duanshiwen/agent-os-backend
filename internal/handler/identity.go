package handler

import (
	"net/http"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type IdentityHandler struct {
	svc                   *service.IdentityService
	auditSvc              *service.AuditService
	sensitiveOperationSvc *service.SensitiveOperationService
}

func NewIdentityHandler(svc *service.IdentityService) *IdentityHandler {
	return &IdentityHandler{svc: svc}
}

func (h *IdentityHandler) SetAuditService(auditSvc *service.AuditService) {
	h.auditSvc = auditSvc
}

func (h *IdentityHandler) SetSensitiveOperationService(svc *service.SensitiveOperationService) {
	h.sensitiveOperationSvc = svc
}

// POST /api/v1/auth/challenge
func (h *IdentityHandler) Challenge(c *gin.Context) {
	var req struct {
		DeviceID   string `json:"device_id" binding:"required"`
		UserPubKey string `json:"user_pubkey" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.InitiateChallenge(req.DeviceID, req.UserPubKey)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, result)
}

// POST /api/v1/auth/verify
func (h *IdentityHandler) Verify(c *gin.Context) {
	var req service.VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.VerifySignature(&req)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}
	if result.AdmissionStatus == "pending_approval" {
		response.Accepted(c, result)
		return
	}
	response.OK(c, result)
}

// POST /api/v1/auth/register
// Deprecated: user creation must go through challenge-response verification and admission policy checks.
func (h *IdentityHandler) Register(c *gin.Context) {
	response.Error(c, http.StatusGone, "direct registration is disabled; use /api/v1/auth/challenge followed by /api/v1/auth/verify")
}

// GET /api/v1/users/me
func (h *IdentityHandler) GetProfile(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	user, err := h.svc.GetProfile(userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}
	response.OK(c, user)
}

// PUT /api/v1/users/me
func (h *IdentityHandler) UpdateProfile(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	var req struct {
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := h.svc.UpdateProfile(userID, deviceID, req.DisplayName, req.AvatarURL)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, user)
}

// POST /api/v1/users/me/devices
// Deprecated: use POST /api/v1/devices/pairing/start and POST /api/v1/devices/pairing/claim for QR-only pairing.
func (h *IdentityHandler) PairDevice(c *gin.Context) {
	c.Header("Deprecation", "true")
	c.Header("Link", "</api/v1/devices/pairing/start>; rel=\"successor-version\"")
	response.Error(c, http.StatusGone, "direct device pairing is disabled; use QR-only pairing endpoints")
}

// GET /api/v1/users/me/devices
func (h *IdentityHandler) GetDevices(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	devices, err := h.svc.GetUserDevices(userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, devices)
}

// PUT /api/v1/users/me/devices/:device_id
func (h *IdentityHandler) RenameDevice(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := c.Param("device_id")
	var req struct {
		DeviceName string `json:"device_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	device, err := h.svc.RenameDevice(userID, deviceID, req.DeviceName)
	if err != nil {
		h.recordAudit(c, service.AuditActionDeviceRenamed, "device", deviceID, service.AuditOutcomeFailure, gin.H{"error": err.Error()})
		response.BadRequest(c, err.Error())
		return
	}
	h.recordAudit(c, service.AuditActionDeviceRenamed, "device", deviceID, service.AuditOutcomeSuccess, gin.H{"device_name": device.DeviceName})
	response.OK(c, device)
}

// DELETE /api/v1/users/me/devices/:device_id
func (h *IdentityHandler) RevokeDevice(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	currentDeviceID := middleware.MustGetDeviceID(c)
	targetDeviceID := c.Param("device_id")
	var req struct {
		ConfirmationToken string `json:"confirmation_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !enforceSensitiveConfirmation(c, h.sensitiveOperationSvc, h.auditSvc, req.ConfirmationToken, service.SensitiveOperationDeviceRevoke, "device.revoke", service.AuditActionDeviceRevoked, "device", targetDeviceID) {
		return
	}
	device, err := h.svc.RevokeDevice(userID, currentDeviceID, targetDeviceID)
	if err != nil {
		h.recordAudit(c, service.AuditActionDeviceRevoked, "device", targetDeviceID, service.AuditOutcomeFailure, gin.H{"error": err.Error()})
		response.BadRequest(c, err.Error())
		return
	}
	h.recordAudit(c, service.AuditActionDeviceRevoked, "device", targetDeviceID, service.AuditOutcomeSuccess, nil)
	response.OK(c, device)
}

func (h *IdentityHandler) recordAudit(c *gin.Context, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	if h.auditSvc == nil {
		return
	}
	actorID := middleware.MustGetUserID(c)
	_, _ = h.auditSvc.Record(service.RecordAuditEventInput{ActorUserID: &actorID, ActorDeviceID: middleware.MustGetDeviceID(c), Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent(), Metadata: metadata})
}

// Helper to parse UUID param
func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		response.BadRequest(c, "invalid "+name)
		return uuid.Nil, false
	}
	return id, true
}
