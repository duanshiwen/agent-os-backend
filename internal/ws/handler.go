package ws

import (
	"encoding/json"
	"log"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/agent-os/backend/internal/service"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Dispatcher routes incoming WS messages to the appropriate service methods.
type Dispatcher struct {
	hub         *Hub
	msgService  *service.MessageService
	convService *service.ConversationService
	convRepo    *repository.ConversationRepo
}

func NewDispatcher(hub *Hub, msgService *service.MessageService, convService *service.ConversationService, convRepo *repository.ConversationRepo) *Dispatcher {
	return &Dispatcher{
		hub:         hub,
		msgService:  msgService,
		convService: convService,
		convRepo:    convRepo,
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
	case "typing.start", "typing.stop":
		d.handleTyping(client, msgType, payload)
	case "ping":
		d.handlePing(client, payload)
	default:
		log.Printf("ws: unknown message type: %s from user=%s", msgType, client.UserID)
	}
}

func (d *Dispatcher) handleSendMessage(client *Client, payload json.RawMessage) {
	var wsReq MsgSendMessage
	if err := json.Unmarshal(payload, &wsReq); err != nil {
		d.sendError(client, 400, "invalid message payload")
		return
	}
	var metadata datatypes.JSONMap
	if len(wsReq.Metadata) > 0 {
		if err := json.Unmarshal(wsReq.Metadata, &metadata); err != nil {
			d.sendError(client, 400, "invalid metadata payload")
			return
		}
	}
	var visibility datatypes.JSONMap
	if len(wsReq.Visibility) > 0 {
		if err := json.Unmarshal(wsReq.Visibility, &visibility); err != nil {
			d.sendError(client, 400, "invalid visibility payload")
			return
		}
	}
	req := service.SendMessageRequest{
		ConversationID: wsReq.ConversationID,
		Type:           wsReq.Type,
		Content:        wsReq.Content,
		Metadata:       metadata,
		ReplyTo:        wsReq.ReplyTo,
		ThreadID:       wsReq.ThreadID,
		Visibility:     visibility,
		ClientEventID:  wsReq.ClientEventID,
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
		MessageID:     delivery.Message.ID,
		Status:        "sent",
		ClientEventID: req.ClientEventID,
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

	// Collect unique message IDs for batch lookup
	msgIDs := make([]uuid.UUID, 0, len(offlineMsgs))
	for _, om := range offlineMsgs {
		msgIDs = append(msgIDs, om.MessageID)
	}

	// Fetch actual messages from repository
	messages, err := d.convRepo.GetMessagesByIDs(msgIDs)
	if err != nil {
		d.sendError(client, 500, "failed to fetch message content")
		return
	}

	// Build message map for efficient lookup
	msgMap := make(map[uuid.UUID]*model.Message, len(messages))
	for i := range messages {
		msgMap[messages[i].ID] = &messages[i]
	}

	batch := MsgOfflineBatch{
		Messages: make([]MsgNewMessage, 0, len(offlineMsgs)),
	}
	for _, om := range offlineMsgs {
		msg, ok := msgMap[om.MessageID]
		if !ok {
			continue
		}
		batch.Messages = append(batch.Messages, MsgNewMessage{
			ConversationID: msg.ConversationID,
			MessageID:      msg.ID,
			SenderID:       msg.SenderID,
			Type:           msg.Type,
			Content:        msg.Content,
			Metadata:       msg.Metadata,
			CreatedAt:      msg.CreatedAt,
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

func (d *Dispatcher) handleTyping(client *Client, msgType string, payload json.RawMessage) {
	var req MsgTypingSignal
	if err := json.Unmarshal(payload, &req); err != nil || req.ConversationID == uuid.Nil {
		d.sendError(client, 400, "invalid typing payload")
		return
	}
	participants, err := d.convService.GetParticipants(req.ConversationID)
	if err != nil {
		d.sendError(client, 404, "conversation not found")
		return
	}
	isParticipant := false
	for _, p := range participants {
		if p.UserID == client.UserID {
			isParticipant = true
			break
		}
	}
	if !isParticipant {
		d.sendError(client, 403, "access denied: not a participant")
		return
	}
	signal := MsgTypingSignal{ConversationID: req.ConversationID, UserID: client.UserID, DeviceID: client.DeviceID, Timestamp: time.Now().UnixMilli()}
	data := marshalEnvelope(msgType, signal)
	for _, p := range participants {
		if p.UserID == client.UserID {
			d.hub.SendToUserExceptDevice(p.UserID, client.DeviceID, data)
			continue
		}
		d.hub.SendToUser(p.UserID, data)
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
