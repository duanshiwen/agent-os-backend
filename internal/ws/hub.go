package ws

import (
	"log"
	"sync"

	"github.com/google/uuid"
)

// Hub maintains the set of active clients and broadcasts messages.
type Hub struct {
	// Registered clients
	clients    map[*Client]bool
	clientsMu  sync.RWMutex

	// User → Device → Client mapping
	userDevices map[uuid.UUID]map[string]*Client
	udMu        sync.RWMutex

	// Channels
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:     make(map[*Client]bool),
		userDevices: make(map[uuid.UUID]map[string]*Client),
		Register:    make(chan *Client, 64),
		Unregister:  make(chan *Client, 64),
		Broadcast:   make(chan []byte, 256),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.register(client)

		case client := <-h.Unregister:
			h.unregister(client)

		case message := <-h.Broadcast:
			h.broadcastToAll(message)
		}
	}
}

func (h *Hub) register(c *Client) {
	h.clientsMu.Lock()
	h.clients[c] = true
	h.clientsMu.Unlock()

	h.udMu.Lock()
	if h.userDevices[c.UserID] == nil {
		h.userDevices[c.UserID] = make(map[string]*Client)
	}
	h.userDevices[c.UserID][c.DeviceID] = c
	h.udMu.Unlock()

	log.Printf("ws: user=%s device=%s connected", c.UserID, c.DeviceID)

	// Notify presence update
	h.broadcastPresence(c.UserID, c.DeviceID, "online")
}

func (h *Hub) unregister(c *Client) {
	h.clientsMu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.Send)
	}
	h.clientsMu.Unlock()

	h.udMu.Lock()
	if devices, ok := h.userDevices[c.UserID]; ok {
		delete(devices, c.DeviceID)
		if len(devices) == 0 {
			delete(h.userDevices, c.UserID)
		}
	}
	h.udMu.Unlock()

	log.Printf("ws: user=%s device=%s disconnected", c.UserID, c.DeviceID)

	// Notify presence update
	h.broadcastPresence(c.UserID, c.DeviceID, "offline")
}

func (h *Hub) broadcastToAll(message []byte) {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	for client := range h.clients {
		select {
		case client.Send <- message:
		default:
			close(client.Send)
			delete(h.clients, client)
		}
	}
}

// SendToUser sends a message to all devices of a specific user.
func (h *Hub) SendToUser(userID uuid.UUID, message []byte) {
	h.udMu.RLock()
	defer h.udMu.RUnlock()

	devices, ok := h.userDevices[userID]
	if !ok {
		return
	}
	for _, client := range devices {
		client.SendMessage(message)
	}
}

// SendToUserDevice sends a message to a specific device of a user.
func (h *Hub) SendToUserDevice(userID uuid.UUID, deviceID string, message []byte) {
	h.udMu.RLock()
	defer h.udMu.RUnlock()

	if devices, ok := h.userDevices[userID]; ok {
		if client, ok := devices[deviceID]; ok {
			client.SendMessage(message)
		}
	}
}

// SendToUsers sends a message to multiple users (all their devices).
func (h *Hub) SendToUsers(userIDs []uuid.UUID, message []byte) {
	for _, uid := range userIDs {
		h.SendToUser(uid, message)
	}
}

// SendToUserExceptDevice sends a message to all devices of a user except the specified device.
func (h *Hub) SendToUserExceptDevice(userID uuid.UUID, excludeDeviceID string, message []byte) {
	h.udMu.RLock()
	defer h.udMu.RUnlock()

	devices, ok := h.userDevices[userID]
	if !ok {
		return
	}
	for deviceID, client := range devices {
		if deviceID != excludeDeviceID {
			client.SendMessage(message)
		}
	}
}

// IsUserOnline checks if a user has any connected device.
func (h *Hub) IsUserOnline(userID uuid.UUID) bool {
	h.udMu.RLock()
	defer h.udMu.RUnlock()
	devices, ok := h.userDevices[userID]
	return ok && len(devices) > 0
}

// IsDeviceOnline checks if a specific device is connected.
func (h *Hub) IsDeviceOnline(userID uuid.UUID, deviceID string) bool {
	h.udMu.RLock()
	defer h.udMu.RUnlock()
	if devices, ok := h.userDevices[userID]; ok {
		_, exists := devices[deviceID]
		return exists
	}
	return false
}

// GetOnlineDeviceIDs returns all online device IDs for a user.
func (h *Hub) GetOnlineDeviceIDs(userID uuid.UUID) map[string]bool {
	h.udMu.RLock()
	defer h.udMu.RUnlock()

	result := make(map[string]bool)
	if devices, ok := h.userDevices[userID]; ok {
		for deviceID := range devices {
			result[deviceID] = true
		}
	}
	return result
}

// AllOnlineDeviceIDs returns a map of all online device IDs across all users.
func (h *Hub) AllOnlineDeviceIDs() map[string]bool {
	h.udMu.RLock()
	defer h.udMu.RUnlock()

	result := make(map[string]bool)
	for _, devices := range h.userDevices {
		for deviceID := range devices {
			result[deviceID] = true
		}
	}
	return result
}

func (h *Hub) broadcastPresence(userID uuid.UUID, deviceID, status string) {
	msg := MsgPresenceUpdate{
		UserID:   userID,
		DeviceID: deviceID,
		Status:   status,
	}
	data := marshalEnvelope("presence.update", msg)
	h.broadcastToAll(data)
}

// OnlineCount returns the number of connected clients.
func (h *Hub) OnlineCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	return len(h.clients)
}
