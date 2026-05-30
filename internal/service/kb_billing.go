package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

const (
	KBOperationSearchMetadata               = "search.metadata"
	KBOperationSearchLexical                = "search.lexical"
	KBOperationSearchSemantic               = "search.semantic"
	KBOperationManifestDownloadURLPublic    = "manifest_download_url.created.public"
	KBOperationManifestDownloadURLInstalled = "manifest_download_url.created.installed"
	KBOperationEntryContentDownloadURL      = "entry_content_download_url.created.installed"
	KBOperationEntryFullTextFetch           = "entry.fulltext.fetch"
)

type KBBillingService struct {
	billingRepo *repository.BillingRepo
}

type RecordKBUsageInput struct {
	UserID          uuid.UUID
	OwnerID         uuid.UUID
	CollectionID    uuid.UUID
	SnapshotID      uuid.UUID
	SnapshotEntryID *uuid.UUID
	OperationType   string
	TokensUsed      int
	IsFree          bool
	UnitPrice       int64
	Currency        string
}

func NewKBBillingService(billingRepo *repository.BillingRepo) *KBBillingService {
	return &KBBillingService{billingRepo: billingRepo}
}

func (s *KBBillingService) RecordUsage(input RecordKBUsageInput) (*model.KBUsageRecord, *model.BillingTransaction, *model.ContributorEarning, error) {
	if input.CollectionID == uuid.Nil || input.SnapshotID == uuid.Nil {
		return nil, nil, nil, fmt.Errorf("%w: collection_id and snapshot_id are required", ErrKBInvalid)
	}
	operation := strings.TrimSpace(input.OperationType)
	if operation == "" {
		return nil, nil, nil, fmt.Errorf("%w: operation_type is required", ErrKBInvalid)
	}
	if input.TokensUsed < 0 {
		return nil, nil, nil, fmt.Errorf("%w: tokens_used must be non-negative", ErrKBInvalid)
	}
	currency := strings.TrimSpace(input.Currency)
	if currency == "" {
		currency = "CNY"
	}
	unitPrice := input.UnitPrice
	if input.IsFree {
		unitPrice = 0
	}
	amount := int64(input.TokensUsed) * unitPrice
	now := time.Now().UTC()
	record := &model.KBUsageRecord{
		UserID:          input.UserID,
		CollectionID:    input.CollectionID,
		SnapshotID:      input.SnapshotID,
		SnapshotEntryID: input.SnapshotEntryID,
		OperationType:   operation,
		TokensUsed:      input.TokensUsed,
		UnitPrice:       unitPrice,
		Amount:          amount,
		Currency:        currency,
		BilledAt:        now,
	}
	if err := s.billingRepo.CreateUsageRecord(record); err != nil {
		return nil, nil, nil, err
	}

	var txn *model.BillingTransaction
	var earning *model.ContributorEarning
	if amount > 0 && input.UserID != uuid.Nil {
		if _, err := s.billingRepo.GetOrCreateAccount(input.UserID, currency); err != nil {
			return record, nil, nil, err
		}
		createdTxn := &model.BillingTransaction{
			UserID:      input.UserID,
			Type:        "kb_usage",
			Amount:      -amount,
			Description: fmt.Sprintf("KB usage %s for collection %s", operation, input.CollectionID.String()),
			RelatedID:   record.ID.String(),
		}
		if err := s.billingRepo.CreateTransaction(createdTxn); err != nil {
			return record, nil, nil, err
		}
		txn = createdTxn
		if input.OwnerID != uuid.Nil {
			platformFee := amount / 10
			netAmount := amount - platformFee
			createdEarning, err := s.billingRepo.AddContributorEarning(input.OwnerID, input.CollectionID, repository.BillingPeriod(now), amount, platformFee, netAmount)
			if err != nil {
				return record, txn, nil, err
			}
			earning = createdEarning
		}
	}
	return record, txn, earning, nil
}

func (s *KBBillingService) GetAccount(userID uuid.UUID) (*model.BillingAccount, error) {
	return s.billingRepo.GetOrCreateAccount(userID, "CNY")
}

func (s *KBBillingService) ListTransactions(userID uuid.UUID, limit, offset int) ([]model.BillingTransaction, error) {
	return s.billingRepo.ListTransactions(userID, limit, offset)
}

func (s *KBBillingService) ListContributorEarnings(contributorID uuid.UUID, collectionID *uuid.UUID, limit, offset int) ([]model.ContributorEarning, error) {
	return s.billingRepo.ListContributorEarnings(contributorID, collectionID, limit, offset)
}
