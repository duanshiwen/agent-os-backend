package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type KnowledgeEntriesHandler struct {
	svc *service.KnowledgeEntriesService
}

func NewKnowledgeEntriesHandler(svc *service.KnowledgeEntriesService) *KnowledgeEntriesHandler {
	return &KnowledgeEntriesHandler{svc: svc}
}

func (h *KnowledgeEntriesHandler) List(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	includeDeleted := c.Query("include_deleted") == "true"
	entries, err := h.svc.ListEntries(userID, includeDeleted)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, entries)
}

func (h *KnowledgeEntriesHandler) Get(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	includeDeleted := c.Query("include_deleted") == "true"
	entry, err := h.svc.GetEntry(userID, knowledgeEntryIDParam(c), includeDeleted)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, entry)
}

func (h *KnowledgeEntriesHandler) Create(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	var req service.KnowledgeEntryInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	entry, _, err := h.svc.CreateEntry(userID, deviceID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, entry)
}

func (h *KnowledgeEntriesHandler) Update(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	var req service.KnowledgeEntryInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	entry, _, err := h.svc.UpdateEntry(userID, deviceID, knowledgeEntryIDParam(c), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, entry)
}

func (h *KnowledgeEntriesHandler) Delete(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	var req struct {
		ClientEventID string `json:"client_event_id"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	entry, _, err := h.svc.DeleteEntry(userID, deviceID, knowledgeEntryIDParam(c), req.ClientEventID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, entry)
}

func knowledgeEntryIDParam(c *gin.Context) string {
	return strings.TrimPrefix(c.Param("entry_id"), "/")
}

func (h *KnowledgeEntriesHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrKnowledgeEntryNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrKnowledgeEntryDeleted), errors.Is(err, service.ErrKnowledgeEntryConflict), errors.Is(err, service.ErrSyncIdempotencyConflict):
		response.Conflict(c, err.Error())
	default:
		response.Error(c, http.StatusBadRequest, err.Error())
	}
}
