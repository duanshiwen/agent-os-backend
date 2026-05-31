package handler

import (
	"errors"
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type SAGEPluginHandler struct{ svc *service.SAGEPluginService }

func NewSAGEPluginHandler(svc *service.SAGEPluginService) *SAGEPluginHandler {
	return &SAGEPluginHandler{svc: svc}
}

func (h *SAGEPluginHandler) CreatePlugin(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	var req service.CreateSAGEPluginInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	plugin, err := h.svc.CreatePlugin(userID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, plugin)
}
func (h *SAGEPluginHandler) SubmitVersion(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	pluginID, ok := parseUUIDParam(c, "plugin_id")
	if !ok {
		return
	}
	var req service.SubmitSAGEPluginVersionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	version, validation, err := h.svc.SubmitVersion(userID, pluginID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, gin.H{"version": version, "validation": validation})
}
func (h *SAGEPluginHandler) GetValidation(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	pluginID, ok := parseUUIDParam(c, "plugin_id")
	if !ok {
		return
	}
	versionID, ok := parseUUIDParam(c, "version_id")
	if !ok {
		return
	}
	version, err := h.svc.GetValidation(userID, pluginID, versionID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, version)
}
func (h *SAGEPluginHandler) ListReviewQueue(c *gin.Context) {
	versions, err := h.svc.ListReviewQueue(c.Query("status"), parseIntQuery(c, "limit", 0), parseIntQuery(c, "offset", 0))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, versions)
}
func (h *SAGEPluginHandler) ReviewPlugin(c *gin.Context) {
	reviewerID := middleware.MustGetUserID(c)
	pluginID, ok := parseUUIDParam(c, "plugin_id")
	if !ok {
		return
	}
	versionID, ok := parseUUIDParam(c, "version_id")
	if !ok {
		return
	}
	var req service.ReviewSAGEPluginInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	review, err := h.svc.ReviewPlugin(reviewerID, pluginID, versionID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, review)
}
func (h *SAGEPluginHandler) SuspendPlugin(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	pluginID, ok := parseUUIDParam(c, "plugin_id")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	plugin, err := h.svc.SuspendPlugin(actorID, pluginID, req.Reason)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, plugin)
}
func (h *SAGEPluginHandler) SearchCatalog(c *gin.Context) {
	page, err := h.svc.SearchCatalog(c.Query("q"), c.Query("category"), parseIntQuery(c, "limit", 0), parseIntQuery(c, "offset", 0))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, page)
}
func (h *SAGEPluginHandler) GetCatalogPlugin(c *gin.Context) {
	plugin, err := h.svc.GetCatalogPlugin(c.Param("plugin_key"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, plugin)
}
func (h *SAGEPluginHandler) InstallPlugin(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	var req service.InstallSAGEPluginInput
	_ = c.ShouldBindJSON(&req)
	inst, err := h.svc.InstallPlugin(userID, middleware.MustGetDeviceID(c), c.Param("plugin_key"), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, inst)
}
func (h *SAGEPluginHandler) ListInstallations(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	items, err := h.svc.ListInstallations(userID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, items)
}
func (h *SAGEPluginHandler) DisableInstallation(c *gin.Context) {
	h.setInstallationStatus(c, "disabled")
}
func (h *SAGEPluginHandler) EnableInstallation(c *gin.Context) { h.setInstallationStatus(c, "active") }
func (h *SAGEPluginHandler) UninstallPlugin(c *gin.Context) {
	h.setInstallationStatus(c, "uninstalled")
}
func (h *SAGEPluginHandler) setInstallationStatus(c *gin.Context, status string) {
	userID := middleware.MustGetUserID(c)
	installationID, ok := parseUUIDParam(c, "installation_id")
	if !ok {
		return
	}
	inst, err := h.svc.SetInstallationStatus(userID, middleware.MustGetDeviceID(c), installationID, status)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, inst)
}
func (h *SAGEPluginHandler) GrantPermission(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	installationID, ok := parseUUIDParam(c, "installation_id")
	if !ok {
		return
	}
	var req service.GrantSAGEPermissionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	grant, err := h.svc.GrantPermission(userID, middleware.MustGetDeviceID(c), installationID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, grant)
}
func (h *SAGEPluginHandler) RevokeGrant(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	grantID, ok := parseUUIDParam(c, "grant_id")
	if !ok {
		return
	}
	grant, err := h.svc.RevokeGrant(userID, middleware.MustGetDeviceID(c), grantID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, grant)
}
func (h *SAGEPluginHandler) PolicyBundle(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	installationID, ok := parseUUIDParam(c, "installation_id")
	if !ok {
		return
	}
	bundle, err := h.svc.PolicyBundle(userID, installationID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, bundle)
}
func (h *SAGEPluginHandler) CreateInvocation(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	var req service.CreateSAGEInvocationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	inv, err := h.svc.CreateInvocation(userID, middleware.MustGetDeviceID(c), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, inv)
}
func (h *SAGEPluginHandler) SubmitReport(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	invocationID, ok := parseUUIDParam(c, "invocation_id")
	if !ok {
		return
	}
	var req service.SubmitSAGEExecutionReportInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	report, err := h.svc.SubmitReport(userID, invocationID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, report)
}
func (h *SAGEPluginHandler) DeveloperMetrics(c *gin.Context) {
	developerID := middleware.MustGetUserID(c)
	pluginID, ok := parseUUIDParam(c, "plugin_id")
	if !ok {
		return
	}
	metrics, err := h.svc.DeveloperMetrics(developerID, pluginID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, metrics)
}

func (h *SAGEPluginHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSAGEPluginNotFound), errors.Is(err, service.ErrSAGEVersionNotFound), errors.Is(err, service.ErrSAGEInstallationNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrSAGEForbidden):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrSAGEInvalid):
		response.BadRequest(c, err.Error())
	default:
		response.InternalError(c, err.Error())
	}
}
