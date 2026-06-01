package handler

import (
	"errors"
	"net/http"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type ContactsHandler struct{ svc *service.ContactsService }

func NewContactsHandler(svc *service.ContactsService) *ContactsHandler {
	return &ContactsHandler{svc: svc}
}
func (h *ContactsHandler) List(c *gin.Context) {
	contacts, err := h.svc.ListContacts(middleware.MustGetUserID(c), c.Query("include_deleted") == "true")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, contacts)
}
func (h *ContactsHandler) Get(c *gin.Context) {
	contact, err := h.svc.GetContact(middleware.MustGetUserID(c), c.Param("contact_id"), c.Query("include_deleted") == "true")
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, contact)
}
func (h *ContactsHandler) Create(c *gin.Context) {
	var req service.ContactInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	contact, _, err := h.svc.CreateContact(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, contact)
}
func (h *ContactsHandler) Update(c *gin.Context) {
	var req service.ContactInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	contact, _, err := h.svc.UpdateContact(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), c.Param("contact_id"), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, contact)
}
func (h *ContactsHandler) Delete(c *gin.Context) {
	var req struct {
		ClientEventID string  `json:"client_event_id"`
		BaseVersion   *uint64 `json:"base_version"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	if req.BaseVersion != nil {
		contact, _, err := h.svc.DeleteContact(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), c.Param("contact_id"), req.ClientEventID, *req.BaseVersion)
		if err != nil {
			h.handleError(c, err)
			return
		}
		response.OK(c, contact)
		return
	}
	contact, _, err := h.svc.DeleteContact(middleware.MustGetUserID(c), middleware.MustGetDeviceID(c), c.Param("contact_id"), req.ClientEventID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, contact)
}
func (h *ContactsHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrContactNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrContactDeleted), errors.Is(err, service.ErrContactConflict), errors.Is(err, service.ErrSyncIdempotencyConflict):
		response.Conflict(c, err.Error())
	default:
		response.Error(c, http.StatusBadRequest, err.Error())
	}
}
