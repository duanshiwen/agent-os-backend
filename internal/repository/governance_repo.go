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

func (r *GovernanceRepo) ListCapabilities(limit, offset int) ([]model.CapabilityDefinition, int64, error) {
	var total int64
	if err := r.db.Model(&model.CapabilityDefinition{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.CapabilityDefinition
	err := r.db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
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

func (r *GovernanceRepo) ListPolicyRules(limit, offset int) ([]model.PolicyRule, int64, error) {
	var total int64
	if err := r.db.Model(&model.PolicyRule{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.PolicyRule
	err := r.db.Order("priority ASC, created_at DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}

func (r *GovernanceRepo) CreatePolicyDecision(decision *model.PolicyDecision) error {
	return r.db.Create(decision).Error
}

type PolicyDecisionListFilter struct {
	SubjectType   string
	SubjectID     string
	CapabilityKey string
	Decision      string
	RiskLevel     string
}

func (r *GovernanceRepo) ListPolicyDecisions(filter PolicyDecisionListFilter, limit, offset int) ([]model.PolicyDecision, int64, error) {
	db := r.db.Model(&model.PolicyDecision{})
	if filter.SubjectType != "" {
		db = db.Where("subject_type = ?", filter.SubjectType)
	}
	if filter.SubjectID != "" {
		db = db.Where("subject_id = ?", filter.SubjectID)
	}
	if filter.CapabilityKey != "" {
		db = db.Where("capability_key = ?", filter.CapabilityKey)
	}
	if filter.Decision != "" {
		db = db.Where("decision = ?", filter.Decision)
	}
	if filter.RiskLevel != "" {
		db = db.Where("risk_level = ?", filter.RiskLevel)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.PolicyDecision
	err := db.Order("decided_at DESC, created_at DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}

func (r *GovernanceRepo) GetPolicyDecision(id uuid.UUID) (*model.PolicyDecision, error) {
	var decision model.PolicyDecision
	if err := r.db.First(&decision, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &decision, nil
}

type GovernanceCountRow struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

func (r *GovernanceRepo) CountPolicyDecisionsSince(since time.Time) (int64, []GovernanceCountRow, []GovernanceCountRow, error) {
	db := r.db.Model(&model.PolicyDecision{}).Where("decided_at >= ?", since)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, nil, err
	}
	var byDecision []GovernanceCountRow
	if err := db.Select("decision AS key, count(*) AS count").Group("decision").Order("count DESC").Scan(&byDecision).Error; err != nil {
		return 0, nil, nil, err
	}
	var byRisk []GovernanceCountRow
	if err := db.Select("risk_level AS key, count(*) AS count").Group("risk_level").Order("count DESC").Scan(&byRisk).Error; err != nil {
		return 0, nil, nil, err
	}
	return total, byDecision, byRisk, nil
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

type ApprovalReceiptListFilter struct {
	SubjectType   string
	SubjectID     string
	CapabilityKey string
	Status        string
}

func (r *GovernanceRepo) ListApprovalReceipts(filter ApprovalReceiptListFilter, limit, offset int) ([]model.ApprovalReceipt, int64, error) {
	db := r.db.Model(&model.ApprovalReceipt{})
	if filter.SubjectType != "" {
		db = db.Where("subject_type = ?", filter.SubjectType)
	}
	if filter.SubjectID != "" {
		db = db.Where("subject_id = ?", filter.SubjectID)
	}
	if filter.CapabilityKey != "" {
		db = db.Where("capability_key = ?", filter.CapabilityKey)
	}
	if filter.Status != "" {
		db = db.Where("status = ?", filter.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.ApprovalReceipt
	err := db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}

func (r *GovernanceRepo) CountApprovalReceiptsSince(since time.Time) (int64, []GovernanceCountRow, error) {
	db := r.db.Model(&model.ApprovalReceipt{}).Where("created_at >= ?", since)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var byStatus []GovernanceCountRow
	if err := db.Select("status AS key, count(*) AS count").Group("status").Order("count DESC").Scan(&byStatus).Error; err != nil {
		return 0, nil, err
	}
	return total, byStatus, nil
}

func (r *GovernanceRepo) RevokeApprovalReceipt(id uuid.UUID, reason string, now time.Time) (*model.ApprovalReceipt, error) {
	var receipt model.ApprovalReceipt
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&receipt, "id = ?", id).Error; err != nil {
			return err
		}
		if receipt.Status != "pending" {
			return gorm.ErrInvalidData
		}
		receipt.Status = "revoked"
		metadata := map[string]any(receipt.Metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["revoked_at"] = now.Format(time.RFC3339)
		if reason != "" {
			metadata["revocation_reason"] = reason
		}
		receipt.Metadata = metadata
		return tx.Save(&receipt).Error
	})
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (r *GovernanceRepo) ReissueApprovalReceiptToken(id uuid.UUID, tokenHash, reason string, expiresAt, now time.Time) (*model.ApprovalReceipt, error) {
	var receipt model.ApprovalReceipt
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&receipt, "id = ?", id).Error; err != nil {
			return err
		}
		if receipt.Status != "pending" || !receipt.ExpiresAt.After(now) {
			return gorm.ErrInvalidData
		}
		metadata := map[string]any(receipt.Metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["token_reissued_at"] = now.Format(time.RFC3339)
		if reason != "" {
			metadata["token_reissue_reason"] = reason
		}
		receipt.TokenHash = tokenHash
		receipt.ExpiresAt = expiresAt
		receipt.Metadata = metadata
		return tx.Save(&receipt).Error
	})
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

type ApprovalReceiptConsumeConstraints struct {
	ActorUserID   *uuid.UUID
	SubjectType   string
	SubjectID     string
	CapabilityKey string
}

func (r *GovernanceRepo) ConsumeApprovalReceiptByTokenHash(tokenHash, consumedBy string, now time.Time, constraints ApprovalReceiptConsumeConstraints) (*model.ApprovalReceipt, error) {
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
		if constraints.ActorUserID != nil && *constraints.ActorUserID != uuid.Nil && receipt.ActorUserID != *constraints.ActorUserID {
			return gorm.ErrInvalidData
		}
		if constraints.SubjectType != "" && receipt.SubjectType != constraints.SubjectType {
			return gorm.ErrInvalidData
		}
		if constraints.SubjectID != "" && receipt.SubjectID != constraints.SubjectID {
			return gorm.ErrInvalidData
		}
		if constraints.CapabilityKey != "" && receipt.CapabilityKey != constraints.CapabilityKey {
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

type ScanResultListFilter struct {
	SubjectType string
	SubjectID   string
	Scanner     string
	Severity    string
	Status      string
}

func (r *GovernanceRepo) ListScanResults(filter ScanResultListFilter, limit, offset int) ([]model.GovernanceScanResult, int64, error) {
	db := r.db.Model(&model.GovernanceScanResult{})
	if filter.SubjectType != "" {
		db = db.Where("subject_type = ?", filter.SubjectType)
	}
	if filter.SubjectID != "" {
		db = db.Where("subject_id = ?", filter.SubjectID)
	}
	if filter.Scanner != "" {
		db = db.Where("scanner = ?", filter.Scanner)
	}
	if filter.Severity != "" {
		db = db.Where("severity = ?", filter.Severity)
	}
	if filter.Status != "" {
		db = db.Where("status = ?", filter.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.GovernanceScanResult
	err := db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}

func (r *GovernanceRepo) ResolveScanResult(id uuid.UUID, resolvedBy uuid.UUID, now time.Time) (*model.GovernanceScanResult, error) {
	var result model.GovernanceScanResult
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&result, "id = ?", id).Error; err != nil {
			return err
		}
		result.Status = "resolved"
		result.ResolvedAt = &now
		if resolvedBy != uuid.Nil {
			result.ResolvedBy = &resolvedBy
		}
		return tx.Save(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *GovernanceRepo) CountOpenScanResults() (int64, []GovernanceCountRow, error) {
	db := r.db.Model(&model.GovernanceScanResult{}).Where("status = ?", "open")
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var bySeverity []GovernanceCountRow
	if err := db.Select("severity AS key, count(*) AS count").Group("severity").Order("count DESC").Scan(&bySeverity).Error; err != nil {
		return 0, nil, err
	}
	return total, bySeverity, nil
}
