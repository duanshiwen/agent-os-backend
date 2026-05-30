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

func (r *BillingRepo) CreateTransaction(txn *model.BillingTransaction) error {
	return r.db.Create(txn).Error
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
