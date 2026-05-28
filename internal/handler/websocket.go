package handler

import (
	"log"
	"net/http"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in dev; tighten in production
	},
}

type WebSocketHandler struct {
	hub        *ws.Hub
	dispatcher *ws.Dispatcher
	jwtSecret  string
}

func NewWebSocketHandler(hub *ws.Hub, dispatcher *ws.Dispatcher, jwtSecret string) *WebSocketHandler {
	return &WebSocketHandler{
		hub:        hub,
		dispatcher: dispatcher,
		jwtSecret:  jwtSecret,
	}
}

// GET /api/v1/ws
func (h *WebSocketHandler) HandleWS(c *gin.Context) {
	// Extract token from query param (WS doesn't have Authorization header)
	token := c.Query("token")
	if token == "" {
		// Also check Authorization header
		token = c.GetHeader("Sec-WebSocket-Protocol")
		if token == "" {
			response.Unauthorized(c, "missing token")
			return
		}
	}

	// Validate JWT
	claims, err := middleware.ValidateToken(h.jwtSecret, token)
	if err != nil {
		response.Unauthorized(c, "invalid token")
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	client := ws.NewClient(h.hub, conn, claims.UserID, claims.DeviceID)

	// Register with hub
	h.hub.Register <- client

	// Start pumps
	go client.WritePump()
	go client.ReadPump(h.dispatcher.HandleMessage)
}
