package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrContactNotFound = errors.New("contact not found")
	ErrContactDeleted  = errors.New("contact is deleted")
	ErrContactConflict = errors.New("contact conflict")
)

type ContactsService struct {
	repo    *repository.ContactsRepo
	syncSvc *SyncService
}

type ContactInput struct {
	ContactID     string                      `json:"contact_id"`
	LinkedUserID  *uuid.UUID                  `json:"linked_user_id"`
	AgentOSPubKey string                      `json:"agentos_pubkey"`
	DisplayName   string                      `json:"display_name"`
	Alias         string                      `json:"alias"`
	AvatarURL     string                      `json:"avatar_url"`
	Phones        datatypes.JSONSlice[string] `json:"phones"`
	Emails        datatypes.JSONSlice[string] `json:"emails"`
	Labels        datatypes.JSONSlice[string] `json:"labels"`
	Notes         string                      `json:"notes"`
	Metadata      datatypes.JSONMap           `json:"metadata"`
	ClientEventID string                      `json:"client_event_id"`
	BaseVersion   *uint64                     `json:"base_version"`
}

func NewContactsService(repo *repository.ContactsRepo, syncSvc *SyncService) *ContactsService {
	return &ContactsService{repo: repo, syncSvc: syncSvc}
}
func (s *ContactsService) ListContacts(userID uuid.UUID, includeDeleted bool) ([]model.Contact, error) {
	return s.repo.ListByUser(userID, includeDeleted)
}
func (s *ContactsService) GetContact(userID uuid.UUID, contactID string, includeDeleted bool) (*model.Contact, error) {
	contact, err := s.repo.GetByContactID(userID, strings.TrimSpace(contactID), includeDeleted)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrContactNotFound
		}
		return nil, err
	}
	return contact, nil
}

func (s *ContactsService) CreateContact(userID uuid.UUID, sourceDeviceID string, input ContactInput) (*model.Contact, *model.SyncEvent, error) {
	if err := validateContactInput(input, true); err != nil {
		return nil, nil, err
	}
	contactID := strings.TrimSpace(input.ContactID)
	if contactID == "" {
		contactID = uuid.NewString()
	}
	if ev, c, err := s.idempotentContactReplay(userID, sourceDeviceID, contactID, SyncOperationCreated, input.ClientEventID); ev != nil || err != nil {
		return c, ev, err
	}
	contact := contactFromInput(userID, contactID, sourceDeviceID, input)
	contact.Status = repository.ContactStatusActive
	contact.Version = 1
	var persisted *model.Contact
	var event *model.SyncEvent
	if err := s.repo.Transaction(func(tx *gorm.DB, txRepo *repository.ContactsRepo) error {
		if err := txRepo.Create(contact); err != nil {
			return fmt.Errorf("create contact: %w", err)
		}
		created, err := txRepo.GetByContactID(userID, contactID, true)
		if err != nil {
			return err
		}
		persisted = created
		recorded, err := s.recordContactEventWithRepo(s.syncSvc.syncRepo.WithTx(tx), userID, sourceDeviceID, persisted, SyncOperationCreated, input.ClientEventID)
		if err != nil {
			return err
		}
		event = recorded
		return nil
	}); err != nil {
		return nil, nil, err
	}
	s.notifyContactEvent(userID, sourceDeviceID, event)
	return persisted, event, nil
}

func (s *ContactsService) UpdateContact(userID uuid.UUID, sourceDeviceID, contactID string, input ContactInput) (*model.Contact, *model.SyncEvent, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return nil, nil, fmt.Errorf("contact_id is required")
	}
	if err := validateContactInput(input, false); err != nil {
		return nil, nil, err
	}
	if ev, c, err := s.idempotentContactReplay(userID, sourceDeviceID, contactID, SyncOperationUpdated, input.ClientEventID); ev != nil || err != nil {
		return c, ev, err
	}
	existing, err := s.repo.GetByContactID(userID, contactID, true)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, nil, ErrContactNotFound
		}
		return nil, nil, err
	}
	if existing.Status == repository.ContactStatusDeleted {
		return nil, nil, ErrContactDeleted
	}
	if err := validateContactBaseVersion(input.BaseVersion, existing.Version); err != nil {
		return nil, nil, err
	}
	updated := contactFromInput(userID, contactID, sourceDeviceID, input)
	var persisted *model.Contact
	var event *model.SyncEvent
	if err := s.repo.Transaction(func(tx *gorm.DB, txRepo *repository.ContactsRepo) error {
		if err := txRepo.UpdateActive(updated); err != nil {
			return fmt.Errorf("update contact: %w", err)
		}
		current, err := txRepo.GetByContactID(userID, contactID, true)
		if err != nil {
			return err
		}
		persisted = current
		recorded, err := s.recordContactEventWithRepo(s.syncSvc.syncRepo.WithTx(tx), userID, sourceDeviceID, persisted, SyncOperationUpdated, input.ClientEventID)
		if err != nil {
			return err
		}
		event = recorded
		return nil
	}); err != nil {
		return nil, nil, err
	}
	s.notifyContactEvent(userID, sourceDeviceID, event)
	return persisted, event, nil
}

func (s *ContactsService) DeleteContact(userID uuid.UUID, sourceDeviceID, contactID, clientEventID string, baseVersion ...uint64) (*model.Contact, *model.SyncEvent, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return nil, nil, fmt.Errorf("contact_id is required")
	}
	var expected *uint64
	if len(baseVersion) > 0 {
		expected = &baseVersion[0]
	}
	if ev, c, err := s.idempotentContactReplay(userID, sourceDeviceID, contactID, SyncOperationDeleted, clientEventID); ev != nil || err != nil {
		return c, ev, err
	}
	existing, err := s.repo.GetByContactID(userID, contactID, true)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, nil, ErrContactNotFound
		}
		return nil, nil, err
	}
	if existing.Status == repository.ContactStatusDeleted {
		return nil, nil, ErrContactDeleted
	}
	if err := validateContactBaseVersion(expected, existing.Version); err != nil {
		return nil, nil, err
	}
	deletedAt := time.Now()
	var persisted *model.Contact
	var event *model.SyncEvent
	if err := s.repo.Transaction(func(tx *gorm.DB, txRepo *repository.ContactsRepo) error {
		if err := txRepo.Tombstone(userID, contactID, sourceDeviceID, deletedAt); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrContactNotFound
			}
			return err
		}
		current, err := txRepo.GetByContactID(userID, contactID, true)
		if err != nil {
			return err
		}
		persisted = current
		recorded, err := s.recordContactEventWithRepo(s.syncSvc.syncRepo.WithTx(tx), userID, sourceDeviceID, persisted, SyncOperationDeleted, clientEventID)
		if err != nil {
			return err
		}
		event = recorded
		return nil
	}); err != nil {
		return nil, nil, err
	}
	s.notifyContactEvent(userID, sourceDeviceID, event)
	return persisted, event, nil
}

func contactFromInput(userID uuid.UUID, contactID, sourceDeviceID string, input ContactInput) *model.Contact {
	metadata := input.Metadata
	if metadata == nil {
		metadata = datatypes.JSONMap{}
	}
	return &model.Contact{UserID: userID, ContactID: contactID, LinkedUserID: input.LinkedUserID, AgentOSPubKey: strings.TrimSpace(input.AgentOSPubKey), DisplayName: strings.TrimSpace(input.DisplayName), Alias: strings.TrimSpace(input.Alias), AvatarURL: strings.TrimSpace(input.AvatarURL), Phones: input.Phones, Emails: input.Emails, Labels: input.Labels, Notes: input.Notes, Metadata: metadata, UpdatedByDeviceID: sourceDeviceID}
}
func (s *ContactsService) recordContactEventWithRepo(syncRepo *repository.SyncRepo, userID uuid.UUID, sourceDeviceID string, contact *model.Contact, operation, clientEventID string) (*model.SyncEvent, error) {
	if s.syncSvc == nil {
		return nil, nil
	}
	return s.syncSvc.RecordEnvelopeWithRepo(syncRepo, SyncEnvelope{UserID: userID, SourceDeviceID: sourceDeviceID, ObjectType: SyncObjectContact, ObjectID: contact.ContactID, Operation: operation, ClientEventID: clientEventID, Payload: contactPayload(contact)})
}
func (s *ContactsService) notifyContactEvent(userID uuid.UUID, sourceDeviceID string, event *model.SyncEvent) {
	if s.syncSvc != nil && event != nil {
		s.syncSvc.notifyDevices(userID, sourceDeviceID, event)
	}
}
func (s *ContactsService) idempotentContactReplay(userID uuid.UUID, sourceDeviceID, contactID, operation, clientEventID string) (*model.SyncEvent, *model.Contact, error) {
	if s.syncSvc == nil || clientEventID == "" {
		return nil, nil, nil
	}
	existing, err := s.syncSvc.syncRepo.GetEventByClientEventID(userID, clientEventID)
	if err != nil {
		return nil, nil, nil
	}
	if existing.ObjectType != SyncObjectContact || existing.ObjectID != contactID || existing.Operation != operation || existing.SourceDeviceID != sourceDeviceID {
		return nil, nil, ErrSyncIdempotencyConflict
	}
	contact, err := s.repo.GetByContactID(userID, contactID, true)
	if err != nil {
		if repository.IsNotFound(err) {
			return existing, nil, nil
		}
		return nil, nil, err
	}
	return existing, contact, nil
}
func validateContactInput(input ContactInput, creating bool) error {
	if strings.TrimSpace(input.DisplayName) == "" {
		return fmt.Errorf("display_name is required")
	}
	if creating && len(strings.TrimSpace(input.ContactID)) > 128 {
		return fmt.Errorf("contact_id is too long")
	}
	if len(input.Phones) > 50 || len(input.Emails) > 50 || len(input.Labels) > 50 {
		return fmt.Errorf("too many contact values")
	}
	return nil
}
func validateContactBaseVersion(expected *uint64, actual uint64) error {
	if expected != nil && *expected != actual {
		return ErrContactConflict
	}
	return nil
}
func contactPayload(contact *model.Contact) datatypes.JSONMap {
	payload := datatypes.JSONMap{"contact_id": contact.ContactID, "object_id": contact.ContactID, "user_id": contact.UserID.String(), "agentos_pubkey": contact.AgentOSPubKey, "display_name": contact.DisplayName, "alias": contact.Alias, "avatar_url": contact.AvatarURL, "phones": contact.Phones, "emails": contact.Emails, "labels": contact.Labels, "notes": contact.Notes, "metadata": contact.Metadata, "status": contact.Status, "version": contact.Version, "updated_by_device_id": contact.UpdatedByDeviceID, "updated_at": contact.UpdatedAt}
	if contact.LinkedUserID != nil {
		payload["linked_user_id"] = contact.LinkedUserID.String()
	} else {
		payload["linked_user_id"] = nil
	}
	if contact.DeletedAt != nil {
		payload["deleted_at"] = contact.DeletedAt
	} else {
		payload["deleted_at"] = nil
	}
	return payload
}
