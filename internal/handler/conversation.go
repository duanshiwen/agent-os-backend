package handler

import (
	"strconv"
	"time"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ConversationHandler struct {
	convSvc *service.ConversationService
	msgSvc  *service.MessageService
}

func NewConversationHandler(convSvc *service.ConversationService, msgSvc *service.MessageService) *ConversationHandler {
	return &ConversationHandler{convSvc: convSvc, msgSvc: msgSvc}
}

// POST /api/v1/conversations
func (h *ConversationHandler) Create(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	var req service.CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	conv, err := h.convSvc.CreateConversation(userID, &req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, conv)
}

// GET /api/v1/conversations
func (h *ConversationHandler) List(c *gin.Context) {
	userID := middleware.MustGetUserID(c)

	convs, err := h.convSvc.GetUserConversations(userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, convs)
}

// GET /api/v1/conversations/:id
func (h *ConversationHandler) Get(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	conv, err := h.convSvc.GetConversation(convID, userID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, conv)
}

// GET /api/v1/conversations/:id/messages
func (h *ConversationHandler) GetMessages(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var before *time.Time
	if b := c.Query("before"); b != "" {
		if t, err := time.Parse(time.RFC3339, b); err == nil {
			before = &t
		}
	}

	msgs, err := h.convSvc.GetMessages(convID, userID, limit, before)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, msgs)
}

// POST /api/v1/conversations/:id/participants
func (h *ConversationHandler) AddParticipant(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var req struct {
		UserID uuid.UUID `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.convSvc.AddParticipant(convID, userID, req.UserID); err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, gin.H{"status": "added"})
}

// DELETE /api/v1/conversations/:id/participants/me
func (h *ConversationHandler) Leave(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	if err := h.convSvc.LeaveConversation(convID, userID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"status": "left"})
}
