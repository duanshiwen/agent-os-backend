package ws

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 64 * 1024 // 64 KB
)

type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	UserID   uuid.UUID
	DeviceID string
	Send     chan []byte
	mu       sync.Mutex
}

func NewClient(hub *Hub, conn *websocket.Conn, userID uuid.UUID, deviceID string) *Client {
	return &Client{
		Hub:      hub,
		Conn:     conn,
		UserID:   userID,
		DeviceID: deviceID,
		Send:     make(chan []byte, 256),
	}
}

// ReadPump reads messages from the WS connection and dispatches them.
func (c *Client) ReadPump(handler func(client *Client, msgType string, payload json.RawMessage)) {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error: user=%s device=%s err=%v", c.UserID, c.DeviceID, err)
			}
			break
		}

		var env Envelope
		if err := json.Unmarshal(message, &env); err != nil {
			log.Printf("ws unmarshal error: %v", err)
			continue
		}

		handler(c, env.Type, env.Payload)
	}
}

// WritePump sends messages from the Send channel to the WS connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Drain queued messages into the current write
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte("\n"))
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// SendMessage sends a pre-built message to this client's Send channel.
func (c *Client) SendMessage(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case c.Send <- data:
	default:
		// Channel full, drop message
		log.Printf("ws send buffer full for user=%s device=%s", c.UserID, c.DeviceID)
	}
}
