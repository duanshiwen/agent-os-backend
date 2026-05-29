package handler

import (
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type DevicePairingHandler struct {
	svc *service.DevicePairingService
}

func NewDevicePairingHandler(svc *service.DevicePairingService) *DevicePairingHandler {
	return &DevicePairingHandler{svc: svc}
}

// POST /api/v1/devices/pairing/start
func (h *DevicePairingHandler) Start(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)

	result, err := h.svc.StartPairing(userID, deviceID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, result)
}

// POST /api/v1/devices/pairing/claim
func (h *DevicePairingHandler) Claim(c *gin.Context) {
	var req service.ClaimPairingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	device, err := h.svc.ClaimPairing(&req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, device)
}
