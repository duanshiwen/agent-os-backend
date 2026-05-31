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

const (
	KBEntitlementFree    = "free"
	KBEntitlementPaid    = "paid"
	KBEntitlementTrial   = "trial"
	KBEntitlementGranted = "granted"

	KBBillingIntervalNone  = "none"
	KBBillingIntervalMonth = "month"
	KBBillingIntervalYear  = "year"
)

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

type UpsertKBBillingPlanInput struct {
	CollectionID    uuid.UUID
	EntitlementType string         `json:"entitlement_type"`
	BillingInterval string         `json:"billing_interval"`
	Price           int64          `json:"price"`
	Currency        string         `json:"currency"`
	TrialDays       int            `json:"trial_days"`
	Metadata        map[string]any `json:"metadata"`
}

type CreateKBInvoiceInput struct {
	UserID         uuid.UUID
	CollectionID   *uuid.UUID
	SubscriptionID *uuid.UUID
	Currency       string
	PeriodStart    *time.Time
	PeriodEnd      *time.Time
	Items          []CreateKBInvoiceItemInput
	Metadata       map[string]any
}

type CreateKBInvoiceItemInput struct {
	UsageRecordID *uuid.UUID
	Description   string
	Quantity      int64
	UnitPrice     int64
	Amount        int64
	Metadata      map[string]any
}

type KBInvoiceDetail struct {
	Invoice *model.KBInvoice      `json:"invoice"`
	Items   []model.KBInvoiceItem `json:"items"`
}

type RequestKBRefundInput struct {
	InvoiceID uuid.UUID `json:"invoice_id"`
	Amount    int64     `json:"amount"`
	Reason    string    `json:"reason"`
}

type ResolveKBRefundInput struct {
	Status string `json:"status"`
}

type OpenKBBillingDisputeInput struct {
	InvoiceID uuid.UUID `json:"invoice_id"`
	Reason    string    `json:"reason"`
	Detail    string    `json:"detail"`
}

type ResolveKBBillingDisputeInput struct {
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
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

func (s *KBBillingService) UpsertBillingPlan(input UpsertKBBillingPlanInput) (*model.KBBillingPlan, error) {
	if input.CollectionID == uuid.Nil {
		return nil, fmt.Errorf("%w: collection_id is required", ErrKBInvalid)
	}
	entitlement := normalizeEntitlement(input.EntitlementType)
	if entitlement == "" {
		return nil, fmt.Errorf("%w: invalid entitlement_type", ErrKBInvalid)
	}
	interval := normalizeBillingInterval(input.BillingInterval, entitlement)
	if interval == "" {
		return nil, fmt.Errorf("%w: invalid billing_interval", ErrKBInvalid)
	}
	if input.Price < 0 {
		return nil, fmt.Errorf("%w: price must be non-negative", ErrKBInvalid)
	}
	if input.TrialDays < 0 {
		return nil, fmt.Errorf("%w: trial_days must be non-negative", ErrKBInvalid)
	}
	if entitlement == KBEntitlementFree || entitlement == KBEntitlementGranted {
		input.Price = 0
		interval = KBBillingIntervalNone
	}
	currency := strings.TrimSpace(input.Currency)
	if currency == "" {
		currency = "CNY"
	}
	version, err := s.billingRepo.NextPlanVersion(input.CollectionID)
	if err != nil {
		return nil, err
	}
	if err := s.billingRepo.ArchiveActivePlans(input.CollectionID); err != nil {
		return nil, err
	}
	plan := &model.KBBillingPlan{CollectionID: input.CollectionID, Version: version, EntitlementType: entitlement, BillingInterval: interval, Price: input.Price, Currency: currency, TrialDays: input.TrialDays, Status: "active", EffectiveAt: time.Now().UTC(), Metadata: input.Metadata}
	if err := s.billingRepo.CreateBillingPlan(plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func (s *KBBillingService) GetActiveBillingPlan(collectionID uuid.UUID) (*model.KBBillingPlan, error) {
	return s.billingRepo.GetActiveBillingPlan(collectionID)
}

func (s *KBBillingService) ListBillingPlans(collectionID uuid.UUID) ([]model.KBBillingPlan, error) {
	return s.billingRepo.ListBillingPlans(collectionID)
}

func (s *KBBillingService) CreateInvoice(input CreateKBInvoiceInput) (*KBInvoiceDetail, error) {
	if input.UserID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrKBInvalid)
	}
	currency := strings.TrimSpace(input.Currency)
	if currency == "" {
		currency = "CNY"
	}
	items := make([]model.KBInvoiceItem, 0, len(input.Items))
	var subtotal int64
	for _, item := range input.Items {
		description := strings.TrimSpace(item.Description)
		if description == "" {
			return nil, fmt.Errorf("%w: invoice item description is required", ErrKBInvalid)
		}
		quantity := item.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		amount := item.Amount
		if amount == 0 && item.UnitPrice > 0 {
			amount = quantity * item.UnitPrice
		}
		if amount < 0 {
			return nil, fmt.Errorf("%w: invoice item amount must be non-negative", ErrKBInvalid)
		}
		subtotal += amount
		items = append(items, model.KBInvoiceItem{UsageRecordID: item.UsageRecordID, Description: description, Quantity: quantity, UnitPrice: item.UnitPrice, Amount: amount, Metadata: item.Metadata})
	}
	platformFee := subtotal / 10
	total := subtotal
	now := time.Now().UTC()
	invoice := &model.KBInvoice{UserID: input.UserID, CollectionID: input.CollectionID, SubscriptionID: input.SubscriptionID, Status: "issued", Currency: currency, Subtotal: subtotal, PlatformFee: platformFee, Total: total, PeriodStart: input.PeriodStart, PeriodEnd: input.PeriodEnd, IssuedAt: &now, Metadata: input.Metadata}
	if err := s.billingRepo.CreateInvoiceWithItems(invoice, items); err != nil {
		return nil, err
	}
	return &KBInvoiceDetail{Invoice: invoice, Items: items}, nil
}

func (s *KBBillingService) ListInvoices(userID uuid.UUID, limit, offset int) ([]model.KBInvoice, error) {
	return s.billingRepo.ListInvoices(userID, limit, offset)
}

func (s *KBBillingService) GetInvoiceDetail(userID, invoiceID uuid.UUID) (*KBInvoiceDetail, error) {
	invoice, err := s.billingRepo.GetInvoice(invoiceID)
	if err != nil {
		return nil, err
	}
	if invoice.UserID != userID {
		return nil, ErrKBInvalid
	}
	items, err := s.billingRepo.ListInvoiceItems(invoiceID)
	if err != nil {
		return nil, err
	}
	return &KBInvoiceDetail{Invoice: invoice, Items: items}, nil
}

func (s *KBBillingService) MarkInvoicePaid(invoiceID uuid.UUID) (*model.KBInvoice, error) {
	now := time.Now().UTC()
	return s.billingRepo.UpdateInvoiceStatus(invoiceID, "paid", map[string]any{"paid_at": &now})
}

func (s *KBBillingService) RequestRefund(userID uuid.UUID, input RequestKBRefundInput) (*model.KBRefund, error) {
	invoice, err := s.billingRepo.GetInvoice(input.InvoiceID)
	if err != nil {
		return nil, err
	}
	if invoice.UserID != userID {
		return nil, ErrKBInvalid
	}
	if input.Amount <= 0 || input.Amount > invoice.Total {
		return nil, fmt.Errorf("%w: invalid refund amount", ErrKBInvalid)
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: refund reason is required", ErrKBInvalid)
	}
	refund := &model.KBRefund{InvoiceID: input.InvoiceID, UserID: userID, Amount: input.Amount, Reason: reason, Status: "pending"}
	if err := s.billingRepo.CreateRefund(refund); err != nil {
		return nil, err
	}
	return refund, nil
}

func (s *KBBillingService) ListRefunds(userID uuid.UUID, limit, offset int) ([]model.KBRefund, error) {
	return s.billingRepo.ListRefunds(userID, limit, offset)
}

func (s *KBBillingService) ResolveRefund(adminID, refundID uuid.UUID, input ResolveKBRefundInput) (*model.KBRefund, error) {
	status := strings.TrimSpace(input.Status)
	if status != "approved" && status != "rejected" && status != "processed" {
		return nil, fmt.Errorf("%w: invalid refund status", ErrKBInvalid)
	}
	return s.billingRepo.ResolveRefund(refundID, adminID, status)
}

func (s *KBBillingService) OpenDispute(userID uuid.UUID, input OpenKBBillingDisputeInput) (*model.KBBillingDispute, error) {
	invoice, err := s.billingRepo.GetInvoice(input.InvoiceID)
	if err != nil {
		return nil, err
	}
	if invoice.UserID != userID {
		return nil, ErrKBInvalid
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: dispute reason is required", ErrKBInvalid)
	}
	dispute := &model.KBBillingDispute{InvoiceID: input.InvoiceID, UserID: userID, Reason: reason, Detail: strings.TrimSpace(input.Detail), Status: "open"}
	if err := s.billingRepo.CreateDispute(dispute); err != nil {
		return nil, err
	}
	return dispute, nil
}

func (s *KBBillingService) ListDisputes(userID uuid.UUID, limit, offset int) ([]model.KBBillingDispute, error) {
	return s.billingRepo.ListDisputes(userID, limit, offset)
}

func (s *KBBillingService) ResolveDispute(adminID, disputeID uuid.UUID, input ResolveKBBillingDisputeInput) (*model.KBBillingDispute, error) {
	status := strings.TrimSpace(input.Status)
	if status != "accepted" && status != "rejected" && status != "cancelled" {
		return nil, fmt.Errorf("%w: invalid dispute status", ErrKBInvalid)
	}
	return s.billingRepo.ResolveDispute(disputeID, adminID, status, strings.TrimSpace(input.Resolution))
}

func (s *KBBillingService) ListPayoutPeriods(contributorID uuid.UUID, limit, offset int) ([]model.ContributorPayoutPeriod, error) {
	return s.billingRepo.ListPayoutPeriods(contributorID, limit, offset)
}

func (s *KBBillingService) MarkPayoutPaid(payoutID uuid.UUID) (*model.ContributorPayoutPeriod, error) {
	return s.billingRepo.MarkPayoutPeriodPaid(payoutID, time.Now().UTC())
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

func normalizeEntitlement(value string) string {
	switch strings.TrimSpace(value) {
	case "", KBEntitlementFree:
		return KBEntitlementFree
	case KBEntitlementPaid:
		return KBEntitlementPaid
	case KBEntitlementTrial:
		return KBEntitlementTrial
	case KBEntitlementGranted:
		return KBEntitlementGranted
	default:
		return ""
	}
}

func normalizeBillingInterval(value, entitlement string) string {
	trimmed := strings.TrimSpace(value)
	if entitlement == KBEntitlementFree || entitlement == KBEntitlementGranted {
		if trimmed == "" || trimmed == KBBillingIntervalNone {
			return KBBillingIntervalNone
		}
		return ""
	}
	switch trimmed {
	case "":
		return KBBillingIntervalMonth
	case KBBillingIntervalNone:
		if entitlement == KBEntitlementTrial {
			return KBBillingIntervalNone
		}
		return ""
	case KBBillingIntervalMonth, KBBillingIntervalYear:
		return trimmed
	default:
		return ""
	}
}
