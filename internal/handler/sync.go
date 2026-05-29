package handler

import (
	"time"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type SyncHandler struct {
	svc *service.SyncService
}

type SyncPullResponse struct {
	Events            []model.SyncEvent `json:"events"`
	NextAfterSequence uint64            `json:"next_after_sequence"`
	HasMore           bool              `json:"has_more"`
	ServerTime        int64             `json:"server_time"`
	SchemaVersion     int               `json:"schema_version"`
}

func NewSyncHandler(svc *service.SyncService) *SyncHandler {
	return &SyncHandler{svc: svc}
}

func normalizeSyncLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}

// GET /api/v1/sync/events — fetch sync events since last cursor
func (h *SyncHandler) GetEvents(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)

	var req struct {
		Limit         int     `form:"limit"`
		AfterSequence *uint64 `form:"after_sequence"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	limit := normalizeSyncLimit(req.Limit)
	queryLimit := limit + 1

	var events []model.SyncEvent
	var err error
	if req.AfterSequence != nil {
		events, err = h.svc.GetEventsAfter(userID, *req.AfterSequence, queryLimit)
	} else {
		events, err = h.svc.GetEvents(userID, deviceID, queryLimit)
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	nextAfterSequence := uint64(0)
	if len(events) > 0 {
		nextAfterSequence = events[len(events)-1].Sequence
	} else if req.AfterSequence != nil {
		nextAfterSequence = *req.AfterSequence
	}

	response.OK(c, SyncPullResponse{
		Events:            events,
		NextAfterSequence: nextAfterSequence,
		HasMore:           hasMore,
		ServerTime:        time.Now().UnixMilli(),
		SchemaVersion:     service.SyncSchemaVersion,
	})
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
