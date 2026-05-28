package service

import (
	"fmt"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

type ConversationService struct {
	convRepo *repository.ConversationRepo
	userRepo *repository.UserRepo
}

func NewConversationService(convRepo *repository.ConversationRepo, userRepo *repository.UserRepo) *ConversationService {
	return &ConversationService{convRepo: convRepo, userRepo: userRepo}
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
	return s.convRepo.AddParticipant(cp)
}

func (s *ConversationService) LeaveConversation(convID, userID uuid.UUID) error {
	return s.convRepo.RemoveParticipant(convID, userID)
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
