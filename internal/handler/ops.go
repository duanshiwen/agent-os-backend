package handler

import (
	"strconv"

	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type OpsHandler struct {
	svc *service.OpsService
}

func NewOpsHandler(svc *service.OpsService) *OpsHandler {
	return &OpsHandler{svc: svc}
}

// GET /api/v1/admin/ops/background-job-runs
func (h *OpsHandler) ListBackgroundJobRuns(c *gin.Context) {
	status := c.Query("status")
	if !service.IsBackgroundJobRunStatus(status) {
		response.BadRequest(c, "invalid status")
		return
	}
	page, err := h.svc.ListBackgroundJobRuns(service.ListBackgroundJobRunsInput{
		JobName: c.Query("job_name"),
		Status:  status,
		Limit:   parseOpsIntQuery(c, "limit", 50),
		Offset:  parseOpsIntQuery(c, "offset", 0),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, page)
}

// GET /api/v1/admin/ops/background-job-runs/:id
func (h *OpsHandler) GetBackgroundJobRun(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid job run id")
		return
	}
	run, err := h.svc.GetBackgroundJobRun(id)
	if err != nil {
		response.NotFound(c, "background job run not found")
		return
	}
	response.OK(c, run)
}

// POST /api/v1/admin/ops/background-jobs/run-once
func (h *OpsHandler) RunBackgroundJobsOnce(c *gin.Context) {
	result, err := h.svc.RunBackgroundJobsOnce(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, result)
}

func parseOpsIntQuery(c *gin.Context, key string, fallback int) int {
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
