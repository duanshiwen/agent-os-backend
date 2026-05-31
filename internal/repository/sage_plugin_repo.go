package repository

import (
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	SAGEPluginStatusDraft     = "draft"
	SAGEPluginStatusSubmitted = "submitted"
	SAGEPluginStatusPublished = "published"
	SAGEPluginStatusSuspended = "suspended"
	SAGEPluginStatusArchived  = "archived"
	SAGEPluginStatusRemoved   = "removed"

	SAGEReviewStatusPending        = "pending"
	SAGEReviewStatusUnderReview    = "under_review"
	SAGEReviewStatusApproved       = "approved"
	SAGEReviewStatusRejected       = "rejected"
	SAGEReviewStatusRequestChanges = "request_changes"
	SAGEReviewStatusTakedown       = "takedown"

	SAGEVersionStatusDraft     = "draft"
	SAGEVersionStatusSubmitted = "submitted"
	SAGEVersionStatusApproved  = "approved"
	SAGEVersionStatusRejected  = "rejected"
	SAGEVersionStatusArchived  = "archived"

	SAGEValidationStatusValid   = "valid"
	SAGEValidationStatusInvalid = "invalid"

	SAGEInstallationStatusActive      = "active"
	SAGEInstallationStatusDisabled    = "disabled"
	SAGEInstallationStatusUninstalled = "uninstalled"

	SAGEGrantStatusActive  = "active"
	SAGEGrantStatusRevoked = "revoked"

	SAGEInvocationStatusCreated   = "created"
	SAGEInvocationStatusRunning   = "running"
	SAGEInvocationStatusCompleted = "completed"
	SAGEInvocationStatusFailed    = "failed"
)

type SAGEPluginRepo struct{ db *gorm.DB }

func NewSAGEPluginRepo(db *gorm.DB) *SAGEPluginRepo { return &SAGEPluginRepo{db: db} }

func (r *SAGEPluginRepo) Transaction(fn func(txRepo *SAGEPluginRepo) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error { return fn(&SAGEPluginRepo{db: tx}) })
}

func (r *SAGEPluginRepo) CreatePlugin(plugin *model.SAGEPlugin) error {
	return r.db.Create(plugin).Error
}
func (r *SAGEPluginRepo) UpdatePlugin(plugin *model.SAGEPlugin) error { return r.db.Save(plugin).Error }
func (r *SAGEPluginRepo) CreateVersion(version *model.SAGEPluginVersion) error {
	return r.db.Create(version).Error
}
func (r *SAGEPluginRepo) UpdateVersion(version *model.SAGEPluginVersion) error {
	return r.db.Save(version).Error
}
func (r *SAGEPluginRepo) CreateReview(review *model.SAGEPluginReview) error {
	return r.db.Create(review).Error
}
func (r *SAGEPluginRepo) CreateGrant(grant *model.SAGEPluginPermissionGrant) error {
	return r.db.Create(grant).Error
}
func (r *SAGEPluginRepo) UpdateGrant(grant *model.SAGEPluginPermissionGrant) error {
	return r.db.Save(grant).Error
}
func (r *SAGEPluginRepo) CreateReport(report *model.SAGEPluginExecutionReport) error {
	return r.db.Create(report).Error
}
func (r *SAGEPluginRepo) CreateUsageLedger(ledger *model.SAGEPluginUsageLedger) error {
	return r.db.Create(ledger).Error
}

func (r *SAGEPluginRepo) GetPlugin(id uuid.UUID) (*model.SAGEPlugin, error) {
	var plugin model.SAGEPlugin
	err := r.db.First(&plugin, "id = ?", id).Error
	return &plugin, err
}

func (r *SAGEPluginRepo) GetPluginByKey(pluginKey string) (*model.SAGEPlugin, error) {
	var plugin model.SAGEPlugin
	err := r.db.First(&plugin, "plugin_key = ?", strings.TrimSpace(pluginKey)).Error
	return &plugin, err
}

func (r *SAGEPluginRepo) GetVersion(id uuid.UUID) (*model.SAGEPluginVersion, error) {
	var version model.SAGEPluginVersion
	err := r.db.First(&version, "id = ?", id).Error
	return &version, err
}

func (r *SAGEPluginRepo) GetVersionByPluginAndVersion(pluginID uuid.UUID, version string) (*model.SAGEPluginVersion, error) {
	var v model.SAGEPluginVersion
	err := r.db.First(&v, "plugin_id = ? AND version = ?", pluginID, strings.TrimSpace(version)).Error
	return &v, err
}

func (r *SAGEPluginRepo) ListVersionsForPlugin(pluginID uuid.UUID) ([]model.SAGEPluginVersion, error) {
	var versions []model.SAGEPluginVersion
	err := r.db.Where("plugin_id = ?", pluginID).Order("created_at DESC").Find(&versions).Error
	return versions, err
}

func (r *SAGEPluginRepo) ListPluginsForReview(status string, limit, offset int) ([]model.SAGEPluginVersion, error) {
	var versions []model.SAGEPluginVersion
	db := r.db.Model(&model.SAGEPluginVersion{})
	if strings.TrimSpace(status) != "" {
		db = db.Where("status = ?", strings.TrimSpace(status))
	} else {
		db = db.Where("status = ?", SAGEVersionStatusSubmitted)
	}
	if offset < 0 {
		offset = 0
	}
	err := db.Order("updated_at DESC").Limit(normalizeLimit(limit)).Offset(offset).Find(&versions).Error
	return versions, err
}

func (r *SAGEPluginRepo) SearchPublishedPlugins(q, category string, limit, offset int) ([]model.SAGEPlugin, error) {
	var plugins []model.SAGEPlugin
	db := r.db.Model(&model.SAGEPlugin{}).Where("status = ? AND review_status = ? AND visibility = ?", SAGEPluginStatusPublished, SAGEReviewStatusApproved, "public")
	if strings.TrimSpace(category) != "" {
		db = db.Where("category = ?", strings.TrimSpace(category))
	}
	if strings.TrimSpace(q) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
		db = db.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(plugin_key) LIKE ?", like, like, like)
	}
	if offset < 0 {
		offset = 0
	}
	err := db.Order("updated_at DESC").Limit(normalizeLimit(limit)).Offset(offset).Find(&plugins).Error
	return plugins, err
}

func (r *SAGEPluginRepo) CountPublishedPlugins(q, category string) (int64, error) {
	var count int64
	db := r.db.Model(&model.SAGEPlugin{}).Where("status = ? AND review_status = ? AND visibility = ?", SAGEPluginStatusPublished, SAGEReviewStatusApproved, "public")
	if strings.TrimSpace(category) != "" {
		db = db.Where("category = ?", strings.TrimSpace(category))
	}
	if strings.TrimSpace(q) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
		db = db.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(plugin_key) LIKE ?", like, like, like)
	}
	return count, db.Count(&count).Error
}

func (r *SAGEPluginRepo) GetPublishedPluginByKey(pluginKey string) (*model.SAGEPlugin, error) {
	var plugin model.SAGEPlugin
	err := r.db.First(&plugin, "plugin_key = ? AND status = ? AND review_status = ?", strings.TrimSpace(pluginKey), SAGEPluginStatusPublished, SAGEReviewStatusApproved).Error
	return &plugin, err
}

func (r *SAGEPluginRepo) CreateInstallation(installation *model.SAGEPluginInstallation) error {
	return r.db.Create(installation).Error
}
func (r *SAGEPluginRepo) UpdateInstallation(installation *model.SAGEPluginInstallation) error {
	return r.db.Save(installation).Error
}

func (r *SAGEPluginRepo) GetInstallation(id uuid.UUID) (*model.SAGEPluginInstallation, error) {
	var installation model.SAGEPluginInstallation
	err := r.db.First(&installation, "id = ?", id).Error
	return &installation, err
}

func (r *SAGEPluginRepo) GetInstallationByUserPlugin(userID, pluginID uuid.UUID) (*model.SAGEPluginInstallation, error) {
	var installation model.SAGEPluginInstallation
	err := r.db.First(&installation, "user_id = ? AND plugin_id = ?", userID, pluginID).Error
	return &installation, err
}

func (r *SAGEPluginRepo) ListInstallations(userID uuid.UUID) ([]model.SAGEPluginInstallation, error) {
	var installations []model.SAGEPluginInstallation
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&installations).Error
	return installations, err
}

func (r *SAGEPluginRepo) ListActiveGrants(installationID uuid.UUID) ([]model.SAGEPluginPermissionGrant, error) {
	var grants []model.SAGEPluginPermissionGrant
	now := time.Now().UTC()
	err := r.db.Where("installation_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", installationID, SAGEGrantStatusActive, now).Order("created_at ASC").Find(&grants).Error
	return grants, err
}

func (r *SAGEPluginRepo) GetGrant(id uuid.UUID) (*model.SAGEPluginPermissionGrant, error) {
	var grant model.SAGEPluginPermissionGrant
	err := r.db.First(&grant, "id = ?", id).Error
	return &grant, err
}

func (r *SAGEPluginRepo) GetGrantByInstallationPermission(installationID uuid.UUID, permissionKey string) (*model.SAGEPluginPermissionGrant, error) {
	var grant model.SAGEPluginPermissionGrant
	err := r.db.First(&grant, "installation_id = ? AND permission_key = ?", installationID, strings.TrimSpace(permissionKey)).Error
	return &grant, err
}

func (r *SAGEPluginRepo) CreateInvocation(invocation *model.SAGEPluginInvocation) error {
	return r.db.Create(invocation).Error
}
func (r *SAGEPluginRepo) UpdateInvocation(invocation *model.SAGEPluginInvocation) error {
	return r.db.Save(invocation).Error
}

func (r *SAGEPluginRepo) GetInvocation(id uuid.UUID) (*model.SAGEPluginInvocation, error) {
	var invocation model.SAGEPluginInvocation
	err := r.db.First(&invocation, "id = ?", id).Error
	return &invocation, err
}

func (r *SAGEPluginRepo) GetInvocationByClientRequest(userID, pluginID uuid.UUID, clientRequestID string) (*model.SAGEPluginInvocation, error) {
	var invocation model.SAGEPluginInvocation
	err := r.db.First(&invocation, "user_id = ? AND plugin_id = ? AND client_request_id = ?", userID, pluginID, clientRequestID).Error
	return &invocation, err
}

func (r *SAGEPluginRepo) GetReportByClientID(invocationID uuid.UUID, clientReportID string) (*model.SAGEPluginExecutionReport, error) {
	var report model.SAGEPluginExecutionReport
	err := r.db.First(&report, "invocation_id = ? AND client_report_id = ?", invocationID, clientReportID).Error
	return &report, err
}

func (r *SAGEPluginRepo) DeveloperMetrics(developerID, pluginID uuid.UUID) (int64, int64, int64, error) {
	var invocations, completed int64
	if err := r.db.Model(&model.SAGEPluginInvocation{}).Where("plugin_id = ?", pluginID).Count(&invocations).Error; err != nil {
		return 0, 0, 0, err
	}
	if err := r.db.Model(&model.SAGEPluginInvocation{}).Where("plugin_id = ? AND status = ?", pluginID, SAGEInvocationStatusCompleted).Count(&completed).Error; err != nil {
		return 0, 0, 0, err
	}
	var tokens int64
	if err := r.db.Model(&model.SAGEPluginExecutionReport{}).Joins("JOIN sage_plugin_invocations ON sage_plugin_invocations.id = sage_plugin_execution_reports.invocation_id").Where("sage_plugin_invocations.plugin_id = ?", pluginID).Select("COALESCE(SUM(sage_plugin_execution_reports.tokens_used), 0)").Scan(&tokens).Error; err != nil {
		return 0, 0, 0, err
	}
	_ = developerID
	return invocations, completed, tokens, nil
}
