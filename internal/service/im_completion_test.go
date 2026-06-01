package service

import (
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

func TestConversationServiceUpdateConversationRecordsConversationUpdated(t *testing.T) {
	svc, convRepo, syncSvc := newConversationSyncTestService(t)
	adminID := uuid.New()
	memberID := uuid.New()
	conv := &model.Conversation{Type: "group", Name: "old group", CreatedBy: adminID}
	if err := convRepo.Create(conv); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := convRepo.AddParticipant(&model.ConversationParticipant{ConversationID: conv.ID, UserID: adminID, Role: "admin"}); err != nil {
		t.Fatalf("add admin: %v", err)
	}
	if err := convRepo.AddParticipant(&model.ConversationParticipant{ConversationID: conv.ID, UserID: memberID, Role: "member"}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	newName := "updated group"

	updated, err := svc.UpdateConversation(adminID, conv.ID, UpdateConversationRequest{Name: &newName})
	if err != nil {
		t.Fatalf("update conversation: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("expected updated name %q, got %q", newName, updated.Name)
	}

	for _, uid := range []uuid.UUID{adminID, memberID} {
		events, err := syncSvc.GetEvents(uid, uid.String()+"-device", 100)
		if err != nil {
			t.Fatalf("get events for %s: %v", uid, err)
		}
		if len(events) != 1 || events[0].EventType != "conversation.updated" || events[0].Payload["name"] != newName {
			t.Fatalf("expected conversation.updated for %s, got %+v", uid, events)
		}
	}
}

func TestConversationServiceRemoveParticipantRecordsParticipantRemovedForRemovedAndRemaining(t *testing.T) {
	svc, convRepo, syncSvc := newConversationSyncTestService(t)
	convID := uuid.New()
	adminID := uuid.New()
	memberID := uuid.New()
	removedID := uuid.New()
	createConversationForTest(t, convRepo, convID, adminID)
	addParticipantForTest(t, convRepo, convID, adminID, "admin")
	addParticipantForTest(t, convRepo, convID, memberID, "member")
	addParticipantForTest(t, convRepo, convID, removedID, "member")

	if err := svc.RemoveParticipant(convID, adminID, removedID); err != nil {
		t.Fatalf("remove participant: %v", err)
	}

	for _, uid := range []uuid.UUID{adminID, memberID, removedID} {
		events, err := syncSvc.GetEvents(uid, uid.String()+"-device", 100)
		if err != nil {
			t.Fatalf("get events for %s: %v", uid, err)
		}
		if len(events) != 1 || events[0].EventType != "participant.removed" || events[0].Payload["user_id"] != removedID.String() || events[0].Payload["removed_by"] != adminID.String() {
			t.Fatalf("expected participant.removed for %s, got %+v", uid, events)
		}
	}
}

func TestConversationServiceUpdateParticipantRecordsParticipantUpdated(t *testing.T) {
	svc, convRepo, syncSvc := newConversationSyncTestService(t)
	convID := uuid.New()
	ownerID := uuid.New()
	memberID := uuid.New()
	createConversationForTest(t, convRepo, convID, ownerID)
	addParticipantForTest(t, convRepo, convID, ownerID, "owner")
	addParticipantForTest(t, convRepo, convID, memberID, "member")
	role := "admin"

	updated, err := svc.UpdateParticipant(convID, ownerID, memberID, UpdateParticipantRequest{Role: &role})
	if err != nil {
		t.Fatalf("update participant: %v", err)
	}
	if updated.Role != "admin" {
		t.Fatalf("expected admin role, got %q", updated.Role)
	}

	for _, uid := range []uuid.UUID{ownerID, memberID} {
		events, err := syncSvc.GetEvents(uid, uid.String()+"-device", 100)
		if err != nil {
			t.Fatalf("get events for %s: %v", uid, err)
		}
		if len(events) != 1 || events[0].EventType != "participant.updated" || events[0].Payload["role"] != "admin" || events[0].Payload["updated_by"] != ownerID.String() {
			t.Fatalf("expected participant.updated for %s, got %+v", uid, events)
		}
	}
}

func TestConversationServiceMarkConversationReadRecordsActorOwnedReadCursor(t *testing.T) {
	svc, convRepo, syncSvc := newConversationSyncTestService(t)
	convID := uuid.New()
	readerID := uuid.New()
	otherID := uuid.New()
	createConversationForTest(t, convRepo, convID, readerID)
	addParticipantForTest(t, convRepo, convID, readerID, "member")
	addParticipantForTest(t, convRepo, convID, otherID, "member")
	msg := &model.Message{ConversationID: convID, SenderID: otherID, Type: "text", Content: "hello", Status: "active"}
	if err := convRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}

	state, err := svc.MarkConversationRead(readerID, convID, MarkConversationReadRequest{LastReadMessageID: msg.ID})
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if state.LastReadMessageID == nil || *state.LastReadMessageID != msg.ID {
		t.Fatalf("unexpected read state: %+v", state)
	}

	readerEvents, err := syncSvc.GetEvents(readerID, "reader-device", 100)
	if err != nil {
		t.Fatalf("get reader events: %v", err)
	}
	if len(readerEvents) != 1 || readerEvents[0].EventType != "conversation.read" || readerEvents[0].Payload["last_read_message_id"] != msg.ID.String() {
		t.Fatalf("expected conversation.read for reader, got %+v", readerEvents)
	}
	otherEvents, err := syncSvc.GetEvents(otherID, "other-device", 100)
	if err != nil {
		t.Fatalf("get other events: %v", err)
	}
	if len(otherEvents) != 0 {
		t.Fatalf("expected actor-owned read cursor only, got other events %+v", otherEvents)
	}
}

func TestMessageServiceAddAndRemoveReactionRecordsSyncEvents(t *testing.T) {
	msgSvc, convRepo, _, syncSvc := newMessageSyncTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	reactorID := uuid.New()
	createConversationForTest(t, convRepo, convID, senderID)
	addParticipantForTest(t, convRepo, convID, senderID, "member")
	addParticipantForTest(t, convRepo, convID, reactorID, "member")
	msg := &model.Message{ConversationID: convID, SenderID: senderID, Type: "text", Content: "hello", Status: "active"}
	if err := convRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}

	reaction, err := msgSvc.AddReaction(reactorID, convID, msg.ID, AddReactionRequest{Emoji: "👍"})
	if err != nil {
		t.Fatalf("add reaction: %v", err)
	}
	if reaction.Emoji != "👍" || reaction.UserID != reactorID {
		t.Fatalf("unexpected reaction: %+v", reaction)
	}

	for _, uid := range []uuid.UUID{senderID, reactorID} {
		events, err := syncSvc.GetEvents(uid, uid.String()+"-device", 100)
		if err != nil {
			t.Fatalf("get add reaction events for %s: %v", uid, err)
		}
		if len(events) != 1 || events[0].EventType != "message.reaction_added" || events[0].Payload["emoji"] != "👍" {
			t.Fatalf("expected message.reaction_added for %s, got %+v", uid, events)
		}
	}

	if err := msgSvc.RemoveReaction(reactorID, convID, msg.ID, "👍"); err != nil {
		t.Fatalf("remove reaction: %v", err)
	}
	for _, uid := range []uuid.UUID{senderID, reactorID} {
		events, err := syncSvc.GetEvents(uid, uid.String()+"-device", 100)
		if err != nil {
			t.Fatalf("get remove reaction events for %s: %v", uid, err)
		}
		if len(events) != 2 || events[1].EventType != "message.reaction_removed" || events[1].Payload["emoji"] != "👍" {
			t.Fatalf("expected message.reaction_removed for %s, got %+v", uid, events)
		}
	}
}

func TestMessageServiceSendMessageRequiresMetadataForRichTypes(t *testing.T) {
	msgSvc, convRepo, _, _ := newMessageSyncTestService(t)
	convID := uuid.New()
	senderID := uuid.New()
	createConversationForTest(t, convRepo, convID, senderID)
	addParticipantForTest(t, convRepo, convID, senderID, "member")

	_, err := msgSvc.SendMessage(senderID, &SendMessageRequest{ConversationID: convID, Type: "image", Content: "image"})
	if err == nil || err.Error() != "metadata.object_id is required for image messages" {
		t.Fatalf("expected image metadata validation error, got %v", err)
	}
}

func createConversationForTest(t *testing.T, convRepo *repository.ConversationRepo, convID, creatorID uuid.UUID) {
	t.Helper()
	if err := convRepo.Create(&model.Conversation{Base: model.Base{ID: convID}, Type: "group", CreatedBy: creatorID}); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
}

func addParticipantForTest(t *testing.T, convRepo *repository.ConversationRepo, convID, userID uuid.UUID, role string) {
	t.Helper()
	if err := convRepo.AddParticipant(&model.ConversationParticipant{ConversationID: convID, UserID: userID, Role: role}); err != nil {
		t.Fatalf("add participant: %v", err)
	}
}
