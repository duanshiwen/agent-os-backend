package handler

import (
	"strconv"

	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuditHandler struct {
	svc *service.AuditService
}

func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// GET /api/v1/admin/audit/events
func (h *AuditHandler) List(c *gin.Context) {
	input := service.ListAuditEventsInput{
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		ResourceID:   c.Query("resource_id"),
		Outcome:      c.Query("outcome"),
		Limit:        parseAuditIntQuery(c, "limit", 50),
		Offset:       parseAuditIntQuery(c, "offset", 0),
	}
	if raw := c.Query("actor_user_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(c, "invalid actor_user_id")
			return
		}
		input.ActorUserID = &id
	}
	page, err := h.svc.List(input)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, page)
}

func parseAuditIntQuery(c *gin.Context, key string, fallback int) int {
	raw := c.Query(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
