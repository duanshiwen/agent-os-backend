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

// PATCH /api/v1/conversations/:id
func (h *ConversationHandler) Update(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	conv, err := h.convSvc.UpdateConversation(userID, convID, req)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, conv)
}

// POST /api/v1/conversations/:id/read-state
func (h *ConversationHandler) MarkRead(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req service.MarkConversationReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	state, err := h.convSvc.MarkConversationRead(userID, convID, req)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, state)
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

// PUT /api/v1/conversations/:id/messages/:message_id
func (h *ConversationHandler) UpdateMessage(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	msgID, ok := parseUUIDParam(c, "message_id")
	if !ok {
		return
	}
	var req service.UpdateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	msg, err := h.msgSvc.UpdateMessage(userID, convID, msgID, req)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, msg)
}

// DELETE /api/v1/conversations/:id/messages/:message_id
func (h *ConversationHandler) DeleteMessage(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	msgID, ok := parseUUIDParam(c, "message_id")
	if !ok {
		return
	}
	var req struct {
		ClientEventID string `json:"client_event_id"`
	}
	_ = c.ShouldBindJSON(&req)
	msg, err := h.msgSvc.DeleteMessage(userID, convID, msgID, req.ClientEventID)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, msg)
}

// POST /api/v1/conversations/:id/messages/:message_id/reactions
func (h *ConversationHandler) AddReaction(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	msgID, ok := parseUUIDParam(c, "message_id")
	if !ok {
		return
	}
	var req service.AddReactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	reaction, err := h.msgSvc.AddReaction(userID, convID, msgID, req)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, reaction)
}

// DELETE /api/v1/conversations/:id/messages/:message_id/reactions/:emoji
func (h *ConversationHandler) RemoveReaction(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	msgID, ok := parseUUIDParam(c, "message_id")
	if !ok {
		return
	}
	if err := h.msgSvc.RemoveReaction(userID, convID, msgID, c.Param("emoji")); err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, gin.H{"status": "removed"})
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

// PATCH /api/v1/conversations/:id/participants/:user_id
func (h *ConversationHandler) UpdateParticipant(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	targetID, ok := parseUUIDParam(c, "user_id")
	if !ok {
		return
	}
	var req service.UpdateParticipantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	participant, err := h.convSvc.UpdateParticipant(convID, actorID, targetID, req)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, participant)
}

// DELETE /api/v1/conversations/:id/participants/:user_id
func (h *ConversationHandler) RemoveParticipant(c *gin.Context) {
	actorID := middleware.MustGetUserID(c)
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	targetID, ok := parseUUIDParam(c, "user_id")
	if !ok {
		return
	}
	if err := h.convSvc.RemoveParticipant(convID, actorID, targetID); err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, gin.H{"status": "removed"})
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
