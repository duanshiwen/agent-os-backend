package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GovernanceRepo struct {
	db *gorm.DB
}

func NewGovernanceRepo(db *gorm.DB) *GovernanceRepo {
	return &GovernanceRepo{db: db}
}

func (r *GovernanceRepo) CreateCapability(capability *model.CapabilityDefinition) error {
	return r.db.Create(capability).Error
}

func (r *GovernanceRepo) GetCapabilityByKey(key string) (*model.CapabilityDefinition, error) {
	var capability model.CapabilityDefinition
	if err := r.db.Where("key = ?", key).First(&capability).Error; err != nil {
		return nil, err
	}
	return &capability, nil
}

func (r *GovernanceRepo) CreatePolicyRule(rule *model.PolicyRule) error {
	return r.db.Create(rule).Error
}

type PolicyRuleMatchQuery struct {
	CapabilityKey string
	SubjectType   string
	SubjectID     string
	ActorUserID   *uuid.UUID
	RiskLevel     string
}

func (r *GovernanceRepo) FindMatchingPolicyRules(query PolicyRuleMatchQuery) ([]model.PolicyRule, error) {
	db := r.db.Where("status = ?", "active")
	db = db.Where("capability_key = ? OR capability_key = ''", query.CapabilityKey)
	db = db.Where("subject_type = ? OR subject_type = ''", query.SubjectType)
	db = db.Where("subject_id = ? OR subject_id = ''", query.SubjectID)
	db = db.Where("risk_level = ? OR risk_level = ''", query.RiskLevel)
	if query.ActorUserID != nil && *query.ActorUserID != uuid.Nil {
		db = db.Where("actor_user_id = ? OR actor_user_id IS NULL", *query.ActorUserID)
	} else {
		db = db.Where("actor_user_id IS NULL")
	}
	var rules []model.PolicyRule
	err := db.Order("priority ASC, created_at ASC").Find(&rules).Error
	return rules, err
}

func (r *GovernanceRepo) CreatePolicyDecision(decision *model.PolicyDecision) error {
	return r.db.Create(decision).Error
}

type KillSwitchQuery struct {
	ScopeType     string
	ScopeID       string
	CapabilityKey string
	ActorUserID   *uuid.UUID
	Now           time.Time
}

func (r *GovernanceRepo) FindActiveKillSwitch(query KillSwitchQuery) (*model.KillSwitch, error) {
	if query.Now.IsZero() {
		query.Now = time.Now().UTC()
	}
	db := r.db.Where("status = ?", "active").Where("expires_at IS NULL OR expires_at > ?", query.Now)
	var switches []model.KillSwitch
	if err := db.Order("created_at ASC").Find(&switches).Error; err != nil {
		return nil, err
	}
	for i := range switches {
		sw := &switches[i]
		switch sw.ScopeType {
		case "global":
			return sw, nil
		case "plugin", "user", "server":
			if sw.ScopeID == query.ScopeID {
				return sw, nil
			}
		case "capability":
			if sw.ScopeID == query.CapabilityKey {
				return sw, nil
			}
		}
		if query.ActorUserID != nil && sw.ScopeType == "user" && sw.ScopeID == query.ActorUserID.String() {
			return sw, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *GovernanceRepo) CreateKillSwitch(killSwitch *model.KillSwitch) error {
	return r.db.Create(killSwitch).Error
}

func (r *GovernanceRepo) CreateApprovalReceipt(receipt *model.ApprovalReceipt) error {
	return r.db.Create(receipt).Error
}

func (r *GovernanceRepo) ConsumeApprovalReceiptByTokenHash(tokenHash, consumedBy string, now time.Time) (*model.ApprovalReceipt, error) {
	var receipt model.ApprovalReceipt
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", tokenHash).First(&receipt).Error; err != nil {
			return err
		}
		if receipt.Status != "pending" {
			return gorm.ErrInvalidData
		}
		if !receipt.ExpiresAt.After(now) {
			return gorm.ErrInvalidData
		}
		receipt.Status = "consumed"
		receipt.ConsumedAt = &now
		receipt.ConsumedBy = consumedBy
		return tx.Save(&receipt).Error
	})
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (r *GovernanceRepo) CreateScanResult(result *model.GovernanceScanResult) error {
	return r.db.Create(result).Error
}
