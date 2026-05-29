package handler

import (
	"errors"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type AgentSettingsHandler struct {
	svc *service.AgentSettingsService
}

func NewAgentSettingsHandler(svc *service.AgentSettingsService) *AgentSettingsHandler {
	return &AgentSettingsHandler{svc: svc}
}

func (h *AgentSettingsHandler) List(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	settings, err := h.svc.ListSettings(userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, settings)
}

func (h *AgentSettingsHandler) Update(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	agentID := c.Param("agent_id")

	var req struct {
		DisplayName   *string           `json:"display_name"`
		Config        datatypes.JSONMap `json:"config"`
		ClientEventID string            `json:"client_event_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	setting, _, err := h.svc.UpdateAgent(userID, deviceID, agentID, req.DisplayName, req.Config, req.ClientEventID)
	if err != nil {
		if errors.Is(err, service.ErrSyncIdempotencyConflict) {
			response.Conflict(c, err.Error())
			return
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, setting)
}
