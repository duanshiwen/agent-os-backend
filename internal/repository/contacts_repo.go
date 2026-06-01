package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ContactStatusActive  = "active"
	ContactStatusDeleted = "deleted"
)

type ContactsRepo struct{ db *gorm.DB }

func NewContactsRepo(db *gorm.DB) *ContactsRepo { return &ContactsRepo{db: db} }

func (r *ContactsRepo) Transaction(fn func(tx *gorm.DB, repo *ContactsRepo) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error { return fn(tx, &ContactsRepo{db: tx}) })
}

func (r *ContactsRepo) ListByUser(userID uuid.UUID, includeDeleted bool) ([]model.Contact, error) {
	var contacts []model.Contact
	q := r.db.Where("user_id = ?", userID)
	if !includeDeleted {
		q = q.Where("status = ?", ContactStatusActive)
	}
	err := q.Order("display_name ASC, contact_id ASC").Find(&contacts).Error
	return contacts, err
}

func (r *ContactsRepo) GetByContactID(userID uuid.UUID, contactID string, includeDeleted bool) (*model.Contact, error) {
	var contact model.Contact
	q := r.db.Where("user_id = ? AND contact_id = ?", userID, contactID)
	if !includeDeleted {
		q = q.Where("status = ?", ContactStatusActive)
	}
	err := q.First(&contact).Error
	return &contact, err
}

func (r *ContactsRepo) Create(contact *model.Contact) error { return r.db.Create(contact).Error }

func (r *ContactsRepo) UpdateActive(contact *model.Contact) error {
	return r.db.Model(&model.Contact{}).
		Where("user_id = ? AND contact_id = ? AND status = ?", contact.UserID, contact.ContactID, ContactStatusActive).
		Updates(map[string]any{
			"linked_user_id": contact.LinkedUserID, "agentos_pubkey": contact.AgentOSPubKey,
			"display_name": contact.DisplayName, "alias": contact.Alias, "avatar_url": contact.AvatarURL,
			"phones": contact.Phones, "emails": contact.Emails, "labels": contact.Labels,
			"notes": contact.Notes, "metadata": contact.Metadata, "version": gorm.Expr("version + 1"),
			"updated_by_device_id": contact.UpdatedByDeviceID, "updated_at": time.Now(),
		}).Error
}

func (r *ContactsRepo) Tombstone(userID uuid.UUID, contactID, sourceDeviceID string, deletedAt time.Time) error {
	rows := r.db.Model(&model.Contact{}).
		Where("user_id = ? AND contact_id = ? AND status = ?", userID, contactID, ContactStatusActive).
		Updates(map[string]any{"status": ContactStatusDeleted, "version": gorm.Expr("version + 1"), "deleted_at": deletedAt, "updated_by_device_id": sourceDeviceID, "updated_at": deletedAt}).RowsAffected
	if rows == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
