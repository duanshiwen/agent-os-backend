package ws

import (
	"encoding/json"
	"log"
	"time"

	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
)

// Dispatcher routes incoming WS messages to the appropriate service methods.
type Dispatcher struct {
	hub        *Hub
	msgService *service.MessageService
	convService *service.ConversationService
	convRepo   *repository.ConversationRepo
}

func NewDispatcher(hub *Hub, msgService *service.MessageService, convService *service.ConversationService) *Dispatcher {
	return &Dispatcher{
		hub:        hub,
		msgService: msgService,
		convService: convService,
	}
}

// HandleMessage is called by Client.ReadPump for every incoming WS message.
func (d *Dispatcher) HandleMessage(client *Client, msgType string, payload json.RawMessage) {
	switch msgType {
	case "message.send":
		d.handleSendMessage(client, payload)
	case "message.ack":
		d.handleAckOffline(client, payload)
	case "offline.fetch":
		d.handleFetchOffline(client)
	case "ping":
		d.handlePing(client, payload)
	default:
		log.Printf("ws: unknown message type: %s from user=%s", msgType, client.UserID)
	}
}

func (d *Dispatcher) handleSendMessage(client *Client, payload json.RawMessage) {
	var req service.SendMessageRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		d.sendError(client, 400, "invalid message payload")
		return
	}

	delivery, err := d.msgService.SendMessage(client.UserID, &req)
	if err != nil {
		d.sendError(client, 403, err.Error())
		return
	}

	outMsg := MsgNewMessage{
		ConversationID: delivery.Message.ConversationID,
		MessageID:      delivery.Message.ID,
		SenderID:       delivery.Message.SenderID,
		Type:           delivery.Message.Type,
		Content:        delivery.Message.Content,
		Metadata:       delivery.Message.Metadata,
		CreatedAt:      delivery.Message.CreatedAt,
	}
	data := marshalEnvelope("message.new", outMsg)

	onlineDeviceIDs := d.hub.AllOnlineDeviceIDs()
	for _, p := range delivery.Participants {
		if p.UserID == client.UserID {
			continue
		}
		d.hub.SendToUser(p.UserID, data)
	}

	_ = d.msgService.SaveOfflineMessages(delivery.Message, delivery.Participants, onlineDeviceIDs)

	ack := MsgDeliveryAck{
		MessageID: delivery.Message.ID,
		Status:    "sent",
	}
	client.SendMessage(marshalEnvelope("message.ack", ack))
}

func (d *Dispatcher) handleFetchOffline(client *Client) {
	offlineMsgs, err := d.msgService.FetchOfflineMessages(client.UserID, client.DeviceID)
	if err != nil {
		d.sendError(client, 500, "failed to fetch offline messages")
		return
	}

	if len(offlineMsgs) == 0 {
		return
	}

	// Fetch actual message content for each offline message
	batch := MsgOfflineBatch{
		Messages: make([]MsgNewMessage, 0, len(offlineMsgs)),
	}
	for _, om := range offlineMsgs {
		// The OfflineMessage stores MessageID — we need to look up the actual message
		// For now, we'll include what we have from the OfflineMessage record
		batch.Messages = append(batch.Messages, MsgNewMessage{
			MessageID: om.MessageID,
			SenderID:  om.UserID,
		})
	}

	client.SendMessage(marshalEnvelope("offline.batch", batch))
}

func (d *Dispatcher) handleAckOffline(client *Client, payload json.RawMessage) {
	var ack MsgAckOffline
	if err := json.Unmarshal(payload, &ack); err != nil {
		d.sendError(client, 400, "invalid ack payload")
		return
	}

	if len(ack.MessageIDs) > 0 {
		_ = d.msgService.AckOfflineMessages(ack.MessageIDs)
	}
}

func (d *Dispatcher) handlePing(client *Client, payload json.RawMessage) {
	var ping MsgPing
	_ = json.Unmarshal(payload, &ping)

	pong := MsgPong{Timestamp: time.Now().UnixMilli()}
	client.SendMessage(marshalEnvelope("pong", pong))
}

func (d *Dispatcher) sendError(client *Client, code int, msg string) {
	env := marshalEnvelope("error", MsgError{Code: code, Message: msg})
	client.SendMessage(env)
}
