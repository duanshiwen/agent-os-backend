package service

import (
	"fmt"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type ConversationService struct {
	convRepo *repository.ConversationRepo
	userRepo *repository.UserRepo
	syncSvc  *SyncService
}

func NewConversationService(convRepo *repository.ConversationRepo, userRepo *repository.UserRepo) *ConversationService {
	return &ConversationService{convRepo: convRepo, userRepo: userRepo}
}

func (s *ConversationService) SetSyncService(syncSvc *SyncService) {
	s.syncSvc = syncSvc
}

type CreateConversationRequest struct {
	Type           string      `json:"type" binding:"required"` // private, group, agent_conversation
	Name           string      `json:"name"`
	ParticipantIDs []uuid.UUID `json:"participant_ids" binding:"required,min=1"`
	GeoLat         *float64    `json:"geo_lat"`
	GeoLng         *float64    `json:"geo_lng"`
	GeoRadius      *float64    `json:"geo_radius"`
}

func (s *ConversationService) CreateConversation(creatorID uuid.UUID, req *CreateConversationRequest) (*model.Conversation, error) {
	switch req.Type {
	case "private":
		return s.createPrivate(creatorID, req.ParticipantIDs)
	case "group", "agent_conversation":
		return s.createGroup(creatorID, req)
	default:
		return nil, fmt.Errorf("unsupported conversation type: %s", req.Type)
	}
}

func (s *ConversationService) createPrivate(creatorID uuid.UUID, participantIDs []uuid.UUID) (*model.Conversation, error) {
	if len(participantIDs) != 1 {
		return nil, fmt.Errorf("private conversation requires exactly one other participant")
	}
	otherID := participantIDs[0]

	// Check if conversation already exists
	existing, err := s.convRepo.FindPrivateConversation(creatorID, otherID)
	if err == nil && existing.ID != uuid.Nil {
		return existing, nil
	}

	conv := &model.Conversation{
		Type:      "private",
		CreatedBy: creatorID,
	}
	if err := s.convRepo.Create(conv); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}

	// Add both participants
	for _, uid := range []uuid.UUID{creatorID, otherID} {
		cp := &model.ConversationParticipant{
			ConversationID: conv.ID,
			UserID:         uid,
			Role:           "member",
			JoinedAt:       time.Now(),
		}
		if err := s.convRepo.AddParticipant(cp); err != nil {
			return nil, fmt.Errorf("add participant: %w", err)
		}
	}
	participants, err := s.convRepo.GetParticipants(conv.ID)
	if err != nil {
		return nil, fmt.Errorf("get participants: %w", err)
	}
	s.recordConversationSyncEvents(participants, conv, SyncActionCreated)

	return conv, nil
}

func (s *ConversationService) createGroup(creatorID uuid.UUID, req *CreateConversationRequest) (*model.Conversation, error) {
	conv := &model.Conversation{
		Type:      req.Type,
		Name:      req.Name,
		CreatedBy: creatorID,
		GeoLat:    req.GeoLat,
		GeoLng:    req.GeoLng,
		GeoRadius: req.GeoRadius,
	}
	if err := s.convRepo.Create(conv); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}

	// Add creator as admin
	cp := &model.ConversationParticipant{
		ConversationID: conv.ID,
		UserID:         creatorID,
		Role:           "admin",
		JoinedAt:       time.Now(),
	}
	if err := s.convRepo.AddParticipant(cp); err != nil {
		return nil, fmt.Errorf("add creator: %w", err)
	}

	// Add other participants as members
	for _, uid := range req.ParticipantIDs {
		if uid == creatorID {
			continue
		}
		cp := &model.ConversationParticipant{
			ConversationID: conv.ID,
			UserID:         uid,
			Role:           "member",
			JoinedAt:       time.Now(),
		}
		if err := s.convRepo.AddParticipant(cp); err != nil {
			return nil, fmt.Errorf("add participant: %w", err)
		}
	}
	participants, err := s.convRepo.GetParticipants(conv.ID)
	if err != nil {
		return nil, fmt.Errorf("get participants: %w", err)
	}
	s.recordConversationSyncEvents(participants, conv, SyncActionCreated)

	return conv, nil
}

func (s *ConversationService) GetUserConversations(userID uuid.UUID) ([]model.Conversation, error) {
	return s.convRepo.GetUserConversations(userID)
}

func (s *ConversationService) GetConversation(convID, userID uuid.UUID) (*model.Conversation, error) {
	isParticipant, err := s.convRepo.IsParticipant(convID, userID)
	if err != nil {
		return nil, err
	}
	if !isParticipant {
		return nil, fmt.Errorf("access denied: not a participant")
	}
	return s.convRepo.GetByID(convID)
}

func (s *ConversationService) AddParticipant(convID, actorID, targetID uuid.UUID) error {
	// Only conversation admins/owners can add participants.
	actorParticipant, err := s.convRepo.GetParticipant(convID, actorID)
	if err != nil || actorParticipant == nil || (actorParticipant.Role != "admin" && actorParticipant.Role != "owner") {
		return fmt.Errorf("access denied")
	}

	cp := &model.ConversationParticipant{
		ConversationID: convID,
		UserID:         targetID,
		Role:           "member",
		JoinedAt:       time.Now(),
	}
	if err := s.convRepo.AddParticipant(cp); err != nil {
		return err
	}
	participants, err := s.convRepo.GetParticipants(convID)
	if err != nil {
		return fmt.Errorf("get participants: %w", err)
	}
	s.recordParticipantSyncEvents(participants, cp, SyncActionAdded, actorID)
	return nil
}

func (s *ConversationService) LeaveConversation(convID, userID uuid.UUID) error {
	removedParticipant, err := s.convRepo.GetParticipant(convID, userID)
	if err != nil {
		return err
	}
	participantsBefore, err := s.convRepo.GetParticipants(convID)
	if err != nil {
		return fmt.Errorf("get participants: %w", err)
	}
	if err := s.convRepo.RemoveParticipant(convID, userID); err != nil {
		return err
	}
	s.recordParticipantSyncEvents(participantsBefore, removedParticipant, SyncActionRemoved, userID)
	return nil
}

func (s *ConversationService) GetMessages(convID, userID uuid.UUID, limit int, before *time.Time) ([]model.Message, error) {
	isParticipant, err := s.convRepo.IsParticipant(convID, userID)
	if err != nil || !isParticipant {
		return nil, fmt.Errorf("access denied")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.convRepo.GetMessages(convID, limit, before)
}

func (s *ConversationService) GetParticipants(convID uuid.UUID) ([]model.ConversationParticipant, error) {
	return s.convRepo.GetParticipants(convID)
}

func (s *ConversationService) recordConversationSyncEvents(participants []model.ConversationParticipant, conv *model.Conversation, operation string) {
	if s.syncSvc == nil {
		return
	}
	payload := conversationSyncPayload(conv)
	for _, p := range participants {
		_, _ = s.syncSvc.RecordEnvelope(SyncEnvelope{
			UserID:     p.UserID,
			ObjectType: SyncObjectConversation,
			ObjectID:   conv.ID.String(),
			Operation:  operation,
			Payload:    payload,
		})
	}
}

func (s *ConversationService) recordParticipantSyncEvents(recipients []model.ConversationParticipant, participant *model.ConversationParticipant, operation string, actorID uuid.UUID) {
	if s.syncSvc == nil {
		return
	}
	payload := participantSyncPayload(participant, operation, actorID)
	objectID := participant.ConversationID.String() + ":" + participant.UserID.String()
	for _, p := range recipients {
		_, _ = s.syncSvc.RecordEnvelope(SyncEnvelope{
			UserID:     p.UserID,
			ObjectType: SyncObjectParticipant,
			ObjectID:   objectID,
			Operation:  operation,
			Payload:    payload,
		})
	}
}

func conversationSyncPayload(conv *model.Conversation) datatypes.JSONMap {
	return datatypes.JSONMap{
		"object_id":       conv.ID.String(),
		"conversation_id": conv.ID.String(),
		"type":            conv.Type,
		"name":            conv.Name,
		"created_by":      conv.CreatedBy.String(),
		"geo_lat":         conv.GeoLat,
		"geo_lng":         conv.GeoLng,
		"geo_radius":      conv.GeoRadius,
		"created_at":      conv.CreatedAt,
		"updated_at":      conv.UpdatedAt,
	}
}

func participantSyncPayload(participant *model.ConversationParticipant, operation string, actorID uuid.UUID) datatypes.JSONMap {
	status := "active"
	payload := datatypes.JSONMap{
		"object_id":       participant.ConversationID.String() + ":" + participant.UserID.String(),
		"conversation_id": participant.ConversationID.String(),
		"user_id":         participant.UserID.String(),
		"role":            participant.Role,
		"status":          status,
		"joined_at":       participant.JoinedAt,
		"updated_at":      time.Now(),
	}
	if operation == SyncActionAdded {
		payload["added_by"] = actorID.String()
	}
	if operation == SyncActionRemoved {
		payload["status"] = "removed"
		payload["removed_by"] = actorID.String()
	}
	return payload
}
