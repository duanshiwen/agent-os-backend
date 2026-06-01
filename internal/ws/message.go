package ws

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope is the top-level WS message structure.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	ID      string          `json:"id,omitempty"` // client correlation ID
}

// === Outbound messages (server → client) ===

type MsgNewMessage struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
	SenderID       uuid.UUID `json:"sender_id"`
	Type           string    `json:"type"`
	Content        string    `json:"content"`
	Metadata       any       `json:"metadata,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type MsgDeliveryAck struct {
	MessageID     uuid.UUID `json:"message_id"`
	Status        string    `json:"status"` // delivered, stored
	ClientEventID string    `json:"client_event_id,omitempty"`
}

type MsgOfflineBatch struct {
	Messages []MsgNewMessage `json:"messages"`
}

type MsgError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type MsgPong struct {
	Timestamp int64 `json:"ts"`
}

type MsgPresenceUpdate struct {
	UserID   uuid.UUID `json:"user_id"`
	DeviceID string    `json:"device_id"`
	Status   string    `json:"status"` // online, offline
}

// === Inbound messages (client → server) ===

type MsgSendMessage struct {
	ConversationID uuid.UUID       `json:"conversation_id"`
	Type           string          `json:"type"`
	Content        string          `json:"content"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	ReplyTo        *uuid.UUID      `json:"reply_to,omitempty"`
	ThreadID       string          `json:"thread_id,omitempty"`
	Visibility     json.RawMessage `json:"visibility,omitempty"`
	ClientEventID  string          `json:"client_event_id,omitempty"`
}

type MsgPing struct {
	Timestamp int64 `json:"ts"`
}

type MsgFetchOffline struct {
	// No fields needed — server uses authenticated user context
}

type MsgAckOffline struct {
	MessageIDs []uuid.UUID `json:"message_ids"`
}

type MsgSyncFetch struct {
	Limit int `json:"limit,omitempty"`
}

type MsgSyncAck struct {
	LastSequence uint64 `json:"last_sequence"`
}

// SyncEventMessage is sent to clients when a sync event occurs.
type SyncEventMessage struct {
	EventType string `json:"event_type"`
	Sequence  uint64 `json:"sequence"`
	Timestamp int64  `json:"timestamp"`
	Payload   any    `json:"payload,omitempty"`
}

// Marshal helper
func marshalEnvelope(msgType string, payload any) []byte {
	data, _ := json.Marshal(payload)
	env := Envelope{Type: msgType, Payload: data}
	out, _ := json.Marshal(env)
	return out
}

// MarshalSyncEvent creates a sync event envelope.
func MarshalSyncEvent(event SyncEventMessage) []byte {
	return marshalEnvelope("sync.event", event)
}
