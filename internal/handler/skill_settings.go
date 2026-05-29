package handler

import (
	"errors"
	"net/http"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type SkillSettingsHandler struct {
	svc *service.SkillSettingsService
}

func NewSkillSettingsHandler(svc *service.SkillSettingsService) *SkillSettingsHandler {
	return &SkillSettingsHandler{svc: svc}
}

func (h *SkillSettingsHandler) List(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	settings, err := h.svc.ListSettings(userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, settings)
}

func (h *SkillSettingsHandler) Enable(c *gin.Context) {
	h.mutate(c, service.SyncOperationEnabled)
}

func (h *SkillSettingsHandler) Disable(c *gin.Context) {
	h.mutate(c, service.SyncOperationDisabled)
}

func (h *SkillSettingsHandler) Update(c *gin.Context) {
	h.mutate(c, service.SyncOperationUpdated)
}

func (h *SkillSettingsHandler) mutate(c *gin.Context, operation string) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	skillID := c.Param("skill_id")

	var req struct {
		ClientEventID string            `json:"client_event_id"`
		Config        datatypes.JSONMap `json:"config"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}

	var result any
	var err error
	switch operation {
	case service.SyncOperationEnabled:
		result, _, err = h.svc.EnableSkill(userID, deviceID, skillID, req.ClientEventID)
	case service.SyncOperationDisabled:
		result, _, err = h.svc.DisableSkill(userID, deviceID, skillID, req.ClientEventID)
	case service.SyncOperationUpdated:
		result, _, err = h.svc.UpdateSkill(userID, deviceID, skillID, req.Config, req.ClientEventID)
	default:
		response.Error(c, http.StatusInternalServerError, "unsupported skill setting operation")
		return
	}

	if err != nil {
		if errors.Is(err, service.ErrSyncIdempotencyConflict) {
			response.Conflict(c, err.Error())
			return
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}
