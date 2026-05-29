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

type ServerConnectionsHandler struct {
	svc *service.ServerConnectionsService
}

func NewServerConnectionsHandler(svc *service.ServerConnectionsService) *ServerConnectionsHandler {
	return &ServerConnectionsHandler{svc: svc}
}

func (h *ServerConnectionsHandler) List(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	connections, err := h.svc.ListServers(userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, connections)
}

func (h *ServerConnectionsHandler) Create(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	var req service.AddServerConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	connection, _, err := h.svc.AddServer(userID, deviceID, req)
	if err != nil {
		h.handleMutationError(c, err)
		return
	}
	response.Created(c, connection)
}

func (h *ServerConnectionsHandler) Update(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	connectionID, ok := parseServerConnectionID(c)
	if !ok {
		return
	}
	var req service.UpdateServerConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	connection, _, err := h.svc.UpdateServer(userID, deviceID, connectionID, req)
	if err != nil {
		h.handleMutationError(c, err)
		return
	}
	response.OK(c, connection)
}

func (h *ServerConnectionsHandler) Delete(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	deviceID := middleware.MustGetDeviceID(c)
	connectionID, ok := parseServerConnectionID(c)
	if !ok {
		return
	}
	var req struct {
		ClientEventID string `json:"client_event_id"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	if _, err := h.svc.RemoveServer(userID, deviceID, connectionID, req.ClientEventID); err != nil {
		h.handleMutationError(c, err)
		return
	}
	response.OK(c, gin.H{"status": "removed"})
}

func (h *ServerConnectionsHandler) handleMutationError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrSyncIdempotencyConflict) {
		response.Conflict(c, err.Error())
		return
	}
	if err.Error() == "server connection not found" {
		response.NotFound(c, err.Error())
		return
	}
	response.BadRequest(c, err.Error())
}

func parseServerConnectionID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}
