package handler

import (
	"errors"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type SkillHubHandler struct{ svc *service.SkillHubService }

func NewSkillHubHandler(svc *service.SkillHubService) *SkillHubHandler {
	return &SkillHubHandler{svc: svc}
}

func (h *SkillHubHandler) CreateSkill(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	var req service.CreateSkillInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	skill, err := h.svc.CreateSkill(userID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, skill)
}

func (h *SkillHubHandler) SubmitVersion(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	skillID, ok := parseUUIDParam(c, "skill_id")
	if !ok {
		return
	}
	var req service.SubmitSkillVersionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	version, validation, err := h.svc.SubmitVersion(userID, skillID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, gin.H{"version": version, "validation": validation})
}

func (h *SkillHubHandler) GetValidation(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	skillID, ok := parseUUIDParam(c, "skill_id")
	if !ok {
		return
	}
	versionID, ok := parseUUIDParam(c, "version_id")
	if !ok {
		return
	}
	version, err := h.svc.GetValidation(userID, skillID, versionID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, version)
}

func (h *SkillHubHandler) SearchCatalog(c *gin.Context) {
	page, err := h.svc.SearchCatalog(c.Query("q"), c.Query("category"), parseIntQuery(c, "limit", 0), parseIntQuery(c, "offset", 0))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, page)
}

func (h *SkillHubHandler) GetCatalogSkill(c *gin.Context) {
	item, err := h.svc.GetCatalogSkill(c.Param("skill_key"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *SkillHubHandler) InstallSkill(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	var req service.InstallSkillInput
	_ = c.ShouldBindJSON(&req)
	inst, err := h.svc.InstallSkill(userID, middleware.MustGetDeviceID(c), c.Param("skill_key"), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, inst)
}

func (h *SkillHubHandler) ListInstallations(c *gin.Context) {
	items, err := h.svc.ListInstallations(middleware.MustGetUserID(c))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, items)
}

func (h *SkillHubHandler) EnableInstallation(c *gin.Context) {
	h.setInstallationStatus(c, repository.SkillInstallationStatusActive)
}
func (h *SkillHubHandler) DisableInstallation(c *gin.Context) {
	h.setInstallationStatus(c, repository.SkillInstallationStatusDisabled)
}
func (h *SkillHubHandler) UninstallSkill(c *gin.Context) {
	h.setInstallationStatus(c, repository.SkillInstallationStatusUninstalled)
}

func (h *SkillHubHandler) setInstallationStatus(c *gin.Context, status string) {
	installationID, ok := parseUUIDParam(c, "installation_id")
	if !ok {
		return
	}
	inst, err := h.svc.SetInstallationStatus(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), installationID, status)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, inst)
}

func (h *SkillHubHandler) UpdateInstallation(c *gin.Context) {
	installationID, ok := parseUUIDParam(c, "installation_id")
	if !ok {
		return
	}
	var req service.UpdateSkillInstallationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	inst, err := h.svc.UpdateInstallation(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), installationID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, inst)
}

func (h *SkillHubHandler) TakedownSkill(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	skillID, ok := parseUUIDParam(c, "skill_id")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	skill, err := h.svc.TakedownSkill(actorID, skillID, req.Reason)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, skill)
}

func (h *SkillHubHandler) RestrictPublisher(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	publisherID, ok := parseUUIDParam(c, "publisher_id")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	restriction, err := h.svc.RestrictPublisher(actorID, publisherID, req.Reason)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, restriction)
}

func (h *SkillHubHandler) LiftPublisherRestriction(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	publisherID, ok := parseUUIDParam(c, "publisher_id")
	if !ok {
		return
	}
	restriction, err := h.svc.LiftPublisherRestriction(actorID, publisherID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, restriction)
}

func (h *SkillHubHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSkillNotFound), errors.Is(err, service.ErrSkillVersionNotFound), errors.Is(err, service.ErrSkillInstallationFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrSkillForbidden), errors.Is(err, service.ErrSkillPublisherBlocked):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrSkillInvalid):
		response.BadRequest(c, err.Error())
	default:
		response.InternalError(c, err.Error())
	}
}
