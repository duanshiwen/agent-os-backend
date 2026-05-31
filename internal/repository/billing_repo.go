package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BillingRepo struct {
	db *gorm.DB
}

func NewBillingRepo(db *gorm.DB) *BillingRepo {
	return &BillingRepo{db: db}
}

func (r *BillingRepo) GetOrCreateAccount(userID uuid.UUID, currency string) (*model.BillingAccount, error) {
	if currency == "" {
		currency = "CNY"
	}
	var account model.BillingAccount
	err := r.db.First(&account, "user_id = ?", userID).Error
	if err == nil {
		return &account, nil
	}
	if !IsNotFound(err) {
		return nil, err
	}
	account = model.BillingAccount{UserID: userID, Currency: currency}
	if err := r.db.Create(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *BillingRepo) CreateUsageRecord(record *model.KBUsageRecord) error {
	return r.db.Create(record).Error
}

func (r *BillingRepo) NextPlanVersion(collectionID uuid.UUID) (int, error) {
	var maxVersion int
	err := r.db.Model(&model.KBBillingPlan{}).Where("collection_id = ?", collectionID).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error
	return maxVersion + 1, err
}

func (r *BillingRepo) ArchiveActivePlans(collectionID uuid.UUID) error {
	return r.db.Model(&model.KBBillingPlan{}).Where("collection_id = ? AND status = ?", collectionID, "active").Update("status", "archived").Error
}

func (r *BillingRepo) CreateBillingPlan(plan *model.KBBillingPlan) error {
	return r.db.Create(plan).Error
}

func (r *BillingRepo) GetActiveBillingPlan(collectionID uuid.UUID) (*model.KBBillingPlan, error) {
	var plan model.KBBillingPlan
	err := r.db.Where("collection_id = ? AND status = ?", collectionID, "active").Order("version DESC").First(&plan).Error
	return &plan, err
}

func (r *BillingRepo) ListBillingPlans(collectionID uuid.UUID) ([]model.KBBillingPlan, error) {
	var plans []model.KBBillingPlan
	err := r.db.Where("collection_id = ?", collectionID).Order("version DESC").Find(&plans).Error
	return plans, err
}

func (r *BillingRepo) CreateTransaction(txn *model.BillingTransaction) error {
	return r.db.Create(txn).Error
}

func (r *BillingRepo) CreateInvoiceWithItems(invoice *model.KBInvoice, items []model.KBInvoiceItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(invoice).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].InvoiceID = invoice.ID
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *BillingRepo) GetInvoice(invoiceID uuid.UUID) (*model.KBInvoice, error) {
	var invoice model.KBInvoice
	err := r.db.First(&invoice, "id = ?", invoiceID).Error
	return &invoice, err
}

func (r *BillingRepo) ListInvoices(userID uuid.UUID, limit, offset int) ([]model.KBInvoice, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var invoices []model.KBInvoice
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Offset(offset).Find(&invoices).Error
	return invoices, err
}

func (r *BillingRepo) ListInvoiceItems(invoiceID uuid.UUID) ([]model.KBInvoiceItem, error) {
	var items []model.KBInvoiceItem
	err := r.db.Where("invoice_id = ?", invoiceID).Order("created_at ASC").Find(&items).Error
	return items, err
}

func (r *BillingRepo) UpdateInvoiceStatus(invoiceID uuid.UUID, status string, updates map[string]any) (*model.KBInvoice, error) {
	if updates == nil {
		updates = map[string]any{}
	}
	updates["status"] = status
	if err := r.db.Model(&model.KBInvoice{}).Where("id = ?", invoiceID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.GetInvoice(invoiceID)
}

func (r *BillingRepo) ListTransactions(userID uuid.UUID, limit, offset int) ([]model.BillingTransaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var txns []model.BillingTransaction
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Offset(offset).Find(&txns).Error
	return txns, err
}

func (r *BillingRepo) UpsertPayoutPeriod(contributorID uuid.UUID, period string, grossAmount, platformFee, netAmount int64) (*model.ContributorPayoutPeriod, error) {
	var payout model.ContributorPayoutPeriod
	err := r.db.First(&payout, "contributor_id = ? AND period = ?", contributorID, period).Error
	if err != nil {
		if !IsNotFound(err) {
			return nil, err
		}
		payout = model.ContributorPayoutPeriod{ContributorID: contributorID, Period: period, Status: "open"}
	}
	payout.GrossAmount += grossAmount
	payout.PlatformFee += platformFee
	payout.NetAmount += netAmount
	if payout.ID == uuid.Nil {
		if err := r.db.Create(&payout).Error; err != nil {
			return nil, err
		}
		return &payout, nil
	}
	if err := r.db.Save(&payout).Error; err != nil {
		return nil, err
	}
	return &payout, nil
}

func (r *BillingRepo) ListPayoutPeriods(contributorID uuid.UUID, limit, offset int) ([]model.ContributorPayoutPeriod, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var payouts []model.ContributorPayoutPeriod
	err := r.db.Where("contributor_id = ?", contributorID).Order("period DESC").Limit(limit).Offset(offset).Find(&payouts).Error
	return payouts, err
}

func (r *BillingRepo) MarkPayoutPeriodPaid(payoutID uuid.UUID, paidAt time.Time) (*model.ContributorPayoutPeriod, error) {
	updates := map[string]any{"status": "paid", "paid_at": paidAt.UTC()}
	if err := r.db.Model(&model.ContributorPayoutPeriod{}).Where("id = ?", payoutID).Updates(updates).Error; err != nil {
		return nil, err
	}
	var payout model.ContributorPayoutPeriod
	err := r.db.First(&payout, "id = ?", payoutID).Error
	return &payout, err
}

func (r *BillingRepo) CreateRefund(refund *model.KBRefund) error {
	return r.db.Create(refund).Error
}

func (r *BillingRepo) ListRefunds(userID uuid.UUID, limit, offset int) ([]model.KBRefund, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var refunds []model.KBRefund
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Offset(offset).Find(&refunds).Error
	return refunds, err
}

func (r *BillingRepo) ResolveRefund(refundID, adminID uuid.UUID, status string) (*model.KBRefund, error) {
	now := time.Now().UTC()
	updates := map[string]any{"status": status, "processed_by": &adminID, "processed_at": &now}
	if err := r.db.Model(&model.KBRefund{}).Where("id = ?", refundID).Updates(updates).Error; err != nil {
		return nil, err
	}
	var refund model.KBRefund
	err := r.db.First(&refund, "id = ?", refundID).Error
	return &refund, err
}

func (r *BillingRepo) CreateDispute(dispute *model.KBBillingDispute) error {
	return r.db.Create(dispute).Error
}

func (r *BillingRepo) ListDisputes(userID uuid.UUID, limit, offset int) ([]model.KBBillingDispute, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var disputes []model.KBBillingDispute
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Offset(offset).Find(&disputes).Error
	return disputes, err
}

func (r *BillingRepo) ResolveDispute(disputeID, adminID uuid.UUID, status, resolution string) (*model.KBBillingDispute, error) {
	now := time.Now().UTC()
	updates := map[string]any{"status": status, "resolution": resolution, "resolved_by": &adminID, "resolved_at": &now}
	if err := r.db.Model(&model.KBBillingDispute{}).Where("id = ?", disputeID).Updates(updates).Error; err != nil {
		return nil, err
	}
	var dispute model.KBBillingDispute
	err := r.db.First(&dispute, "id = ?", disputeID).Error
	return &dispute, err
}

func (r *BillingRepo) AddContributorEarning(contributorID, collectionID uuid.UUID, period string, grossAmount, platformFee, netAmount int64) (*model.ContributorEarning, error) {
	var earning model.ContributorEarning
	err := r.db.First(&earning, "contributor_id = ? AND collection_id = ? AND period = ?", contributorID, collectionID, period).Error
	if err != nil {
		if !IsNotFound(err) {
			return nil, err
		}
		earning = model.ContributorEarning{ContributorID: contributorID, CollectionID: collectionID, Period: period}
	}
	earning.GrossAmount += grossAmount
	earning.PlatformFee += platformFee
	earning.NetAmount += netAmount
	if _, err := r.UpsertPayoutPeriod(contributorID, period, grossAmount, platformFee, netAmount); err != nil {
		return nil, err
	}
	if earning.ID == uuid.Nil {
		if err := r.db.Create(&earning).Error; err != nil {
			return nil, err
		}
		return &earning, nil
	}
	if err := r.db.Save(&earning).Error; err != nil {
		return nil, err
	}
	return &earning, nil
}

func (r *BillingRepo) ListContributorEarnings(contributorID uuid.UUID, collectionID *uuid.UUID, limit, offset int) ([]model.ContributorEarning, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	db := r.db.Where("contributor_id = ?", contributorID)
	if collectionID != nil && *collectionID != uuid.Nil {
		db = db.Where("collection_id = ?", *collectionID)
	}
	var earnings []model.ContributorEarning
	err := db.Order("period DESC").Limit(limit).Offset(offset).Find(&earnings).Error
	return earnings, err
}

func BillingPeriod(t time.Time) string {
	return t.UTC().Format("2006-01")
}
