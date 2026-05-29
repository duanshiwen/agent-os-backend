package handler

import (
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type IdentityHandler struct {
	svc *service.IdentityService
}

func NewIdentityHandler(svc *service.IdentityService) *IdentityHandler {
	return &IdentityHandler{svc: svc}
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
func (h *IdentityHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := h.svc.Register(&req)
	if err != nil {
		response.Conflict(c, err.Error())
		return
	}
	response.Created(c, user)
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
	var req struct {
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := h.svc.UpdateProfile(userID, req.DisplayName, req.AvatarURL)
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
	userID := middleware.MustGetUserID(c)
	var req service.PairDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	device, err := h.svc.PairDevice(userID, &req)
	if err != nil {
		response.Conflict(c, err.Error())
		return
	}
	response.Created(c, device)
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

// Helper to parse UUID param
func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		response.BadRequest(c, "invalid "+name)
		return uuid.Nil, false
	}
	return id, true
}
