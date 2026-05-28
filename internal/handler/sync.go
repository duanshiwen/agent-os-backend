package handler

import (
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type SyncHandler struct {
	svc *service.SyncService
}

func NewSyncHandler(svc *service.SyncService) *SyncHandler {
	return &SyncHandler{svc: svc}
}

// GET /api/v1/sync/events — fetch sync events since last cursor
func (h *SyncHandler) GetEvents(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)

	var req struct {
		Limit int `form:"limit"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		req.Limit = 100
	}

	events, err := h.svc.GetEvents(userID, deviceID, req.Limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, events)
}

// POST /api/v1/sync/ack — acknowledge sync events
func (h *SyncHandler) AckEvents(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)

	var req struct {
		LastSequence uint64 `json:"last_sequence" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.svc.AckEvents(userID, deviceID, req.LastSequence); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"status": "acked"})
}
