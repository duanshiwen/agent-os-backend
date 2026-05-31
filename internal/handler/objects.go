package handler

import (
	"errors"
	"net/http"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ObjectHandler struct {
	svc *service.ObjectService
}

func NewObjectHandler(svc *service.ObjectService) *ObjectHandler {
	return &ObjectHandler{svc: svc}
}

func (h *ObjectHandler) CreateUploadIntent(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	var req service.CreateUploadIntentInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	intent, err := h.svc.CreateUploadIntent(c.Request.Context(), userID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, intent)
}

func (h *ObjectHandler) CompleteUpload(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	objectID, ok := parseObjectIDParam(c)
	if !ok {
		return
	}
	var req service.CompleteUploadInput
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	object, err := h.svc.CompleteUpload(c.Request.Context(), userID, objectID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, object)
}

func (h *ObjectHandler) Get(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	objectID, ok := parseObjectIDParam(c)
	if !ok {
		return
	}
	object, err := h.svc.GetObject(userID, objectID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, object)
}

func (h *ObjectHandler) CreateDownloadURL(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	objectID, ok := parseObjectIDParam(c)
	if !ok {
		return
	}
	var req service.CreateDownloadURLInput
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	result, err := h.svc.CreateDownloadURL(c.Request.Context(), userID, objectID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *ObjectHandler) Delete(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	objectID, ok := parseObjectIDParam(c)
	if !ok {
		return
	}
	object, err := h.svc.DeleteObject(userID, objectID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, object)
}

func parseObjectIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid object id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *ObjectHandler) handleError(c *gin.Context, err error) {
	if handleGovernanceError(c, err) {
		return
	}
	switch {
	case errors.Is(err, service.ErrObjectNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrObjectForbidden):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrObjectNotActive), errors.Is(err, service.ErrObjectHashMismatch), errors.Is(err, service.ErrObjectSizeMismatch), errors.Is(err, service.ErrObjectTypeMismatch), errors.Is(err, service.ErrObjectHasReferences):
		response.Conflict(c, err.Error())
	case errors.Is(err, service.ErrObjectInvalid):
		response.BadRequest(c, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, err.Error())
	}
}
