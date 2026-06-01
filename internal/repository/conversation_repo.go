package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConversationRepo struct {
	db *gorm.DB
}

func NewConversationRepo(db *gorm.DB) *ConversationRepo {
	return &ConversationRepo{db: db}
}

// === Conversation ===

func (r *ConversationRepo) Create(c *model.Conversation) error {
	return r.db.Create(c).Error
}

func (r *ConversationRepo) GetByID(id uuid.UUID) (*model.Conversation, error) {
	var c model.Conversation
	err := r.db.Where("id = ?", id).First(&c).Error
	return &c, err
}

func (r *ConversationRepo) GetUserConversations(userID uuid.UUID) ([]model.Conversation, error) {
	var convs []model.Conversation
	err := r.db.
		Joins("JOIN conversation_participants cp ON cp.conversation_id = conversations.id").
		Where("cp.user_id = ?", userID).
		Order("conversations.updated_at DESC").
		Find(&convs).Error
	return convs, err
}

// === ConversationParticipant ===

func (r *ConversationRepo) AddParticipant(cp *model.ConversationParticipant) error {
	return r.db.Create(cp).Error
}

func (r *ConversationRepo) RemoveParticipant(conversationID, userID uuid.UUID) error {
	return r.db.Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Delete(&model.ConversationParticipant{}).Error
}

func (r *ConversationRepo) GetParticipants(conversationID uuid.UUID) ([]model.ConversationParticipant, error) {
	var parts []model.ConversationParticipant
	err := r.db.Where("conversation_id = ?", conversationID).Find(&parts).Error
	return parts, err
}

func (r *ConversationRepo) IsParticipant(conversationID, userID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&model.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Count(&count).Error
	return count > 0, err
}

func (r *ConversationRepo) GetParticipant(conversationID, userID uuid.UUID) (*model.ConversationParticipant, error) {
	var cp model.ConversationParticipant
	err := r.db.Where("conversation_id = ? AND user_id = ?", conversationID, userID).First(&cp).Error
	return &cp, err
}

func (r *ConversationRepo) FindPrivateConversation(userA, userB uuid.UUID) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.
		Joins("JOIN conversation_participants cp1 ON cp1.conversation_id = conversations.id AND cp1.user_id = ?", userA).
		Joins("JOIN conversation_participants cp2 ON cp2.conversation_id = conversations.id AND cp2.user_id = ?", userB).
		Where("conversations.type = ?", "private").
		First(&conv).Error
	return &conv, err
}

// === Message ===

func (r *ConversationRepo) CreateMessage(m *model.Message) error {
	return r.db.Create(m).Error
}

func (r *ConversationRepo) GetMessageByID(id uuid.UUID) (*model.Message, error) {
	var msg model.Message
	err := r.db.Where("id = ?", id).First(&msg).Error
	return &msg, err
}

func (r *ConversationRepo) FindMessageByClientEventID(senderID uuid.UUID, clientEventID string) (*model.Message, error) {
	var msg model.Message
	err := r.db.Where("sender_id = ? AND client_event_id = ?", senderID, clientEventID).First(&msg).Error
	return &msg, err
}

func (r *ConversationRepo) UpdateMessage(m *model.Message) error {
	return r.db.Save(m).Error
}

func (r *ConversationRepo) SoftDeleteMessage(messageID, actorID uuid.UUID, deletedAt time.Time) (*model.Message, error) {
	updates := map[string]any{
		"status":     "deleted",
		"deleted_at": deletedAt,
		"deleted_by": actorID,
		"updated_at": deletedAt,
	}
	if err := r.db.Model(&model.Message{}).Where("id = ?", messageID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.GetMessageByID(messageID)
}

func (r *ConversationRepo) GetMessages(conversationID uuid.UUID, limit int, before *time.Time) ([]model.Message, error) {
	var msgs []model.Message
	q := r.db.Where("conversation_id = ?", conversationID)
	if before != nil {
		q = q.Where("created_at < ?", *before)
	}
	err := q.Order("created_at DESC").Limit(limit).Find(&msgs).Error
	return msgs, err
}

func (r *ConversationRepo) GetMessagesSince(conversationID uuid.UUID, since time.Time) ([]model.Message, error) {
	var msgs []model.Message
	err := r.db.Where("conversation_id = ? AND created_at > ?", conversationID, since).
		Order("created_at ASC").Find(&msgs).Error
	return msgs, err
}

func (r *ConversationRepo) GetMessagesByIDs(ids []uuid.UUID) ([]model.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var msgs []model.Message
	err := r.db.Where("id IN ?", ids).Find(&msgs).Error
	return msgs, err
}

// === OfflineMessage ===

func (r *ConversationRepo) SaveOfflineMessage(om *model.OfflineMessage) error {
	return r.db.Create(om).Error
}

func (r *ConversationRepo) GetUndeliveredMessages(userID uuid.UUID, deviceID string, limit int) ([]model.OfflineMessage, error) {
	var msgs []model.OfflineMessage
	err := r.db.
		Where("user_id = ? AND device_id = ? AND delivered = ?", userID, deviceID, false).
		Preload("Message").
		Order("created_at ASC").
		Limit(limit).
		Find(&msgs).Error
	return msgs, err
}

func (r *ConversationRepo) MarkOfflineDelivered(ids []uuid.UUID) error {
	return r.db.Model(&model.OfflineMessage{}).
		Where("id IN ?", ids).
		Update("delivered", true).Error
}

func (r *ConversationRepo) CleanupExpiredOffline(before time.Time) (int64, error) {
	result := r.db.Where("created_at < ?", before).Delete(&model.OfflineMessage{})
	return result.RowsAffected, result.Error
}
