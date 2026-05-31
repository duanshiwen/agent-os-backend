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
	ErrSAGEPluginNotFound       = errors.New("sage plugin not found")
	ErrSAGEVersionNotFound      = errors.New("sage plugin version not found")
	ErrSAGEInstallationNotFound = errors.New("sage plugin installation not found")
	ErrSAGEInvalid              = errors.New("sage request invalid")
	ErrSAGEForbidden            = errors.New("sage operation forbidden")
)

const SensitiveOperationSAGEHighRiskGrant = "sage.permission.high_risk_grant"

type SAGEPluginService struct {
	repo                  *repository.SAGEPluginRepo
	validator             *SAGEManifestValidator
	syncSvc               *SyncService
	auditSvc              *AuditService
	sensitiveOperationSvc *SensitiveOperationService
	objectSvc             *ObjectService
	governanceEnforcer    *GovernanceEnforcer
	governanceScanner     *GovernanceScanner
}

func NewSAGEPluginService(repo *repository.SAGEPluginRepo) *SAGEPluginService {
	return &SAGEPluginService{repo: repo, validator: NewSAGEManifestValidator()}
}
func (s *SAGEPluginService) SetSyncService(syncSvc *SyncService)    { s.syncSvc = syncSvc }
func (s *SAGEPluginService) SetAuditService(auditSvc *AuditService) { s.auditSvc = auditSvc }
func (s *SAGEPluginService) SetSensitiveOperationService(svc *SensitiveOperationService) {
	s.sensitiveOperationSvc = svc
}
func (s *SAGEPluginService) SetObjectService(objectSvc *ObjectService) { s.objectSvc = objectSvc }
func (s *SAGEPluginService) SetGovernanceEnforcer(enforcer *GovernanceEnforcer) {
	s.governanceEnforcer = enforcer
}
func (s *SAGEPluginService) SetGovernanceScanner(scanner *GovernanceScanner) {
	s.governanceScanner = scanner
}

type CreateSAGEPluginInput struct {
	PluginKey    string     `json:"plugin_key"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Category     string     `json:"category"`
	IconObjectID *uuid.UUID `json:"icon_object_id"`
	HomepageURL  string     `json:"homepage_url"`
	ManifestURL  string     `json:"manifest_url"`
}
type SubmitSAGEPluginVersionInput struct {
	Version         string         `json:"version"`
	Manifest        map[string]any `json:"manifest"`
	PackageObjectID *uuid.UUID     `json:"package_object_id"`
}
type ReviewSAGEPluginInput struct {
	Decision         string   `json:"decision"`
	Reason           string   `json:"reason"`
	SecurityFindings []string `json:"security_findings"`
	PrivacyFindings  []string `json:"privacy_findings"`
	PolicyFindings   []string `json:"policy_findings"`
}
type InstallSAGEPluginInput struct {
	TrackMode string `json:"track_mode"`
}
type GrantSAGEPermissionInput struct {
	PermissionKey     string         `json:"permission_key"`
	GrantScope        map[string]any `json:"grant_scope"`
	ConfirmationToken string         `json:"confirmation_token"`
	ApprovalToken     string         `json:"approval_token"`
	ExpiresAt         *time.Time     `json:"expires_at"`
}
type CreateSAGEInvocationInput struct {
	PluginKey       string         `json:"plugin_key"`
	InstallationID  *uuid.UUID     `json:"installation_id"`
	ClientRequestID string         `json:"client_request_id"`
	UserIntent      string         `json:"user_intent"`
	FlowID          string         `json:"flow_id"`
	FlowHash        string         `json:"flow_hash"`
	RiskLevel       string         `json:"risk_level"`
	PermissionsUsed []string       `json:"permissions_used"`
	PolicyDecision  map[string]any `json:"policy_decision"`
}
type SubmitSAGEExecutionReportInput struct {
	ClientReportID    string         `json:"client_report_id"`
	Status            string         `json:"status"`
	FlowID            string         `json:"flow_id"`
	StepsCompleted    []string       `json:"steps_completed"`
	StepSummaries     map[string]any `json:"step_summaries"`
	Errors            []string       `json:"errors"`
	UserConfirmations []string       `json:"user_confirmations"`
	PluginCallbacks   []string       `json:"plugin_callbacks"`
	TokensUsed        int            `json:"tokens_used"`
	Metering          map[string]any `json:"metering"`
}

type SAGECatalogPluginAsset struct {
	ObjectID    uuid.UUID `json:"object_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	ContentHash string    `json:"content_hash"`
	ContentSize int64     `json:"content_size"`
}

type SAGECatalogPlugin struct {
	model.SAGEPlugin
	IconAsset    *SAGECatalogPluginAsset `json:"icon_asset,omitempty"`
	PackageAsset *SAGECatalogPluginAsset `json:"package_asset,omitempty"`
}

type SAGECatalogPage struct {
	Items         []SAGECatalogPlugin `json:"items"`
	Limit, Offset int                 `json:"limit"`
	Total         int64               `json:"total"`
}
type SAGEPolicyBundle struct {
	PluginKey           string            `json:"plugin_key"`
	Version             string            `json:"version"`
	PolicyBundleVersion string            `json:"policy_bundle_version"`
	ManifestSnapshot    datatypes.JSONMap `json:"manifest_snapshot"`
	GrantedPermissions  []map[string]any  `json:"granted_permissions"`
	DeniedPermissions   []string          `json:"denied_permissions"`
	RuntimeGuards       []map[string]any  `json:"runtime_guards"`
	Reporting           map[string]any    `json:"reporting"`
}
type SAGEDeveloperMetrics struct {
	PluginKey        string  `json:"plugin_key"`
	Period           string  `json:"period"`
	Invocations      int64   `json:"invocations"`
	Completed        int64   `json:"completed"`
	Failed           int64   `json:"failed"`
	SuccessRate      float64 `json:"success_rate"`
	TokensUsed       int64   `json:"tokens_used"`
	EstimatedRevenue int64   `json:"estimated_revenue"`
}

func (s *SAGEPluginService) CreatePlugin(developerID uuid.UUID, input CreateSAGEPluginInput) (*model.SAGEPlugin, error) {
	pluginKey := strings.TrimSpace(input.PluginKey)
	name := strings.TrimSpace(input.Name)
	if pluginKey == "" || name == "" {
		return nil, fmt.Errorf("%w: plugin_key and name are required", ErrSAGEInvalid)
	}
	if input.IconObjectID != nil {
		icon, err := s.requireActiveOwnedAsset(developerID, *input.IconObjectID, "icon", []string{"image/"})
		if err != nil {
			return nil, err
		}
		input.IconObjectID = &icon.ID
	}
	plugin := &model.SAGEPlugin{PluginKey: pluginKey, DeveloperID: developerID, Name: name, Description: strings.TrimSpace(input.Description), Category: strings.TrimSpace(input.Category), IconObjectID: input.IconObjectID, HomepageURL: strings.TrimSpace(input.HomepageURL), ManifestURL: strings.TrimSpace(input.ManifestURL), Status: repository.SAGEPluginStatusDraft, ReviewStatus: repository.SAGEReviewStatusPending, Visibility: "private", RiskLevel: "unknown", TrustLevel: "unverified"}
	if err := s.repo.CreatePlugin(plugin); err != nil {
		return nil, err
	}
	s.recordAudit(&developerID, "", AuditActionSAGEPluginCreated, "sage_plugin", plugin.ID.String(), AuditOutcomeSuccess, map[string]any{"plugin_key": plugin.PluginKey})
	return plugin, nil
}

func (s *SAGEPluginService) SubmitVersion(developerID, pluginID uuid.UUID, input SubmitSAGEPluginVersionInput) (*model.SAGEPluginVersion, *SAGEManifestValidationResult, error) {
	plugin, err := s.repo.GetPlugin(pluginID)
	if err != nil {
		return nil, nil, ErrSAGEPluginNotFound
	}
	if plugin.DeveloperID != developerID {
		return nil, nil, ErrSAGEForbidden
	}
	result, err := s.validator.Validate(input.Manifest)
	if err != nil {
		return nil, nil, err
	}
	version := strings.TrimSpace(input.Version)
	if version == "" {
		version = result.Manifest.Version
	}
	if version == "" {
		return nil, nil, fmt.Errorf("%w: version is required", ErrSAGEInvalid)
	}
	if input.PackageObjectID != nil {
		pkg, err := s.requireActiveOwnedAsset(developerID, *input.PackageObjectID, "package", []string{"application/zip", "application/gzip", "application/x-tar", "application/octet-stream"})
		if err != nil {
			return nil, nil, err
		}
		input.PackageObjectID = &pkg.ID
	}
	now := time.Now().UTC()
	status := repository.SAGEVersionStatusSubmitted
	validationStatus := repository.SAGEValidationStatusValid
	if !result.Valid {
		status = repository.SAGEVersionStatusDraft
		validationStatus = repository.SAGEValidationStatusInvalid
	}
	v := &model.SAGEPluginVersion{PluginID: plugin.ID, Version: version, SAGEVersion: result.Manifest.SAGEVersion, ManifestHash: result.ManifestHash, ManifestSnapshot: datatypes.JSONMap(result.Snapshot), PackageObjectID: input.PackageObjectID, ValidationStatus: validationStatus, ValidationErrors: datatypes.JSONSlice[string](result.Errors), ValidationWarnings: datatypes.JSONSlice[string](result.Warnings), RiskSummary: datatypes.JSONMap(result.RiskSummary), PermissionSummary: datatypes.JSONMap(result.PermissionSummary), Status: status}
	if result.Valid {
		v.SubmittedAt = &now
	}
	err = s.repo.Transaction(func(tx *repository.SAGEPluginRepo) error {
		if err := tx.CreateVersion(v); err != nil {
			return err
		}
		plugin.LatestVersionID = &v.ID
		plugin.ManifestURL = result.Manifest.Endpoints.Manifest
		plugin.FlowURL = result.Manifest.Endpoints.Flow
		plugin.CallbackURL = result.Manifest.Endpoints.Callback
		plugin.HealthURL = result.Manifest.Endpoints.Health
		plugin.RiskLevel = fmt.Sprint(result.RiskSummary["highest_risk"])
		if result.Valid {
			plugin.Status = repository.SAGEPluginStatusSubmitted
			plugin.ReviewStatus = repository.SAGEReviewStatusPending
		}
		return tx.UpdatePlugin(plugin)
	})
	if err != nil {
		return nil, nil, err
	}
	if s.governanceScanner != nil {
		findings := s.governanceScanner.ScanSAGEManifest(plugin.ID.String(), v.ID.String(), result.Snapshot)
		if err := s.governanceScanner.PersistFindings(GovernanceSubjectSAGEPlugin, plugin.ID.String(), findings); err != nil {
			return nil, nil, err
		}
	}
	s.recordAudit(&developerID, "", AuditActionSAGEPluginVersionSubmitted, "sage_plugin_version", v.ID.String(), AuditOutcomeSuccess, map[string]any{"plugin_id": plugin.ID.String(), "valid": result.Valid})
	return v, result, nil
}

func (s *SAGEPluginService) GetValidation(developerID, pluginID, versionID uuid.UUID) (*model.SAGEPluginVersion, error) {
	plugin, err := s.repo.GetPlugin(pluginID)
	if err != nil {
		return nil, ErrSAGEPluginNotFound
	}
	if plugin.DeveloperID != developerID {
		return nil, ErrSAGEForbidden
	}
	v, err := s.repo.GetVersion(versionID)
	if err != nil || v.PluginID != pluginID {
		return nil, ErrSAGEVersionNotFound
	}
	return v, nil
}

func (s *SAGEPluginService) ListReviewQueue(status string, limit, offset int) ([]model.SAGEPluginVersion, error) {
	return s.repo.ListPluginsForReview(status, limit, offset)
}

func (s *SAGEPluginService) ReviewPlugin(reviewerID, pluginID, versionID uuid.UUID, input ReviewSAGEPluginInput) (*model.SAGEPluginReview, error) {
	plugin, err := s.repo.GetPlugin(pluginID)
	if err != nil {
		return nil, ErrSAGEPluginNotFound
	}
	version, err := s.repo.GetVersion(versionID)
	if err != nil || version.PluginID != pluginID {
		return nil, ErrSAGEVersionNotFound
	}
	decision := strings.TrimSpace(input.Decision)
	if decision == "" {
		return nil, fmt.Errorf("%w: decision is required", ErrSAGEInvalid)
	}
	review := &model.SAGEPluginReview{PluginID: pluginID, VersionID: versionID, ReviewerID: reviewerID, Decision: decision, Reason: strings.TrimSpace(input.Reason), SecurityFindings: datatypes.JSONSlice[string](input.SecurityFindings), PrivacyFindings: datatypes.JSONSlice[string](input.PrivacyFindings), PolicyFindings: datatypes.JSONSlice[string](input.PolicyFindings)}
	now := time.Now().UTC()
	err = s.repo.Transaction(func(tx *repository.SAGEPluginRepo) error {
		if err := tx.CreateReview(review); err != nil {
			return err
		}
		switch decision {
		case "approved":
			plugin.Status = repository.SAGEPluginStatusPublished
			plugin.ReviewStatus = repository.SAGEReviewStatusApproved
			plugin.Visibility = "public"
			plugin.ApprovedVersionID = &version.ID
			version.Status = repository.SAGEVersionStatusApproved
			version.ApprovedAt = &now
		case "rejected", "request_changes":
			plugin.ReviewStatus = decision
			version.Status = repository.SAGEVersionStatusRejected
			version.RejectedAt = &now
		case "takedown", "suspended":
			plugin.Status = repository.SAGEPluginStatusSuspended
			plugin.ReviewStatus = repository.SAGEReviewStatusTakedown
		}
		if err := tx.UpdatePlugin(plugin); err != nil {
			return err
		}
		return tx.UpdateVersion(version)
	})
	if err != nil {
		return nil, err
	}
	s.recordAudit(&reviewerID, "", AuditActionSAGEPluginReviewed, "sage_plugin", pluginID.String(), AuditOutcomeSuccess, map[string]any{"version_id": versionID.String(), "decision": decision})
	return review, nil
}

func (s *SAGEPluginService) SuspendPlugin(actorID, pluginID uuid.UUID, reason string) (*model.SAGEPlugin, error) {
	plugin, err := s.repo.GetPlugin(pluginID)
	if err != nil {
		return nil, ErrSAGEPluginNotFound
	}
	plugin.Status = repository.SAGEPluginStatusSuspended
	plugin.ReviewStatus = repository.SAGEReviewStatusTakedown
	if err := s.repo.UpdatePlugin(plugin); err != nil {
		return nil, err
	}
	s.recordAudit(&actorID, "", AuditActionSAGEPluginSuspended, "sage_plugin", pluginID.String(), AuditOutcomeSuccess, map[string]any{"reason": reason})
	return plugin, nil
}
func (s *SAGEPluginService) SearchCatalog(q, category string, limit, offset int) (*SAGECatalogPage, error) {
	items, err := s.repo.SearchPublishedPlugins(q, category, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.repo.CountPublishedPlugins(q, category)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	catalogItems := make([]SAGECatalogPlugin, 0, len(items))
	for _, item := range items {
		catalogItem, err := s.enrichCatalogPlugin(&item)
		if err != nil {
			return nil, err
		}
		catalogItems = append(catalogItems, *catalogItem)
	}
	return &SAGECatalogPage{Items: catalogItems, Limit: limit, Offset: offset, Total: total}, nil
}
func (s *SAGEPluginService) GetCatalogPlugin(pluginKey string) (*SAGECatalogPlugin, error) {
	p, err := s.repo.GetPublishedPluginByKey(pluginKey)
	if err != nil {
		return nil, ErrSAGEPluginNotFound
	}
	return s.enrichCatalogPlugin(p)
}

func (s *SAGEPluginService) InstallPlugin(userID uuid.UUID, deviceID, pluginKey string, input InstallSAGEPluginInput) (*model.SAGEPluginInstallation, error) {
	plugin, err := s.repo.GetPublishedPluginByKey(pluginKey)
	if err != nil {
		return nil, ErrSAGEPluginNotFound
	}
	if plugin.ApprovedVersionID == nil {
		return nil, fmt.Errorf("%w: approved version required", ErrSAGEInvalid)
	}
	if existing, err := s.repo.GetInstallationByUserPlugin(userID, plugin.ID); err == nil {
		if existing.Status != repository.SAGEInstallationStatusActive {
			existing.Status = repository.SAGEInstallationStatusActive
			existing.DisabledAt = nil
			if err := s.repo.UpdateInstallation(existing); err != nil {
				return nil, err
			}
			s.recordPluginSync(userID, deviceID, existing.ID.String(), SyncOperationEnabled, map[string]any{
				"plugin_id":       plugin.ID.String(),
				"plugin_key":      plugin.PluginKey,
				"version_id":      existing.VersionID.String(),
				"installation_id": existing.ID.String(),
				"status":          existing.Status,
			})
		}
		return existing, nil
	}
	track := strings.TrimSpace(input.TrackMode)
	if track == "" {
		track = "latest_approved"
	}
	installation := &model.SAGEPluginInstallation{UserID: userID, PluginID: plugin.ID, VersionID: *plugin.ApprovedVersionID, Status: repository.SAGEInstallationStatusActive, InstallSource: "catalog", TrackMode: track, InstalledAt: time.Now().UTC()}
	if err := s.repo.CreateInstallation(installation); err != nil {
		return nil, err
	}
	s.recordPluginSync(userID, deviceID, installation.ID.String(), SyncOperationInstalled, map[string]any{"plugin_id": plugin.ID.String(), "plugin_key": plugin.PluginKey, "version_id": installation.VersionID.String(), "installation_id": installation.ID.String(), "status": installation.Status, "track_mode": installation.TrackMode})
	s.recordAudit(&userID, deviceID, AuditActionSAGEPluginInstalled, "sage_plugin_installation", installation.ID.String(), AuditOutcomeSuccess, nil)
	return installation, nil
}

func (s *SAGEPluginService) SetInstallationStatus(userID uuid.UUID, deviceID string, installationID uuid.UUID, status string) (*model.SAGEPluginInstallation, error) {
	inst, err := s.repo.GetInstallation(installationID)
	if err != nil || inst.UserID != userID {
		return nil, ErrSAGEInstallationNotFound
	}
	inst.Status = status
	now := time.Now().UTC()
	if status == repository.SAGEInstallationStatusDisabled || status == repository.SAGEInstallationStatusUninstalled {
		inst.DisabledAt = &now
	} else {
		inst.DisabledAt = nil
	}
	if err := s.repo.UpdateInstallation(inst); err != nil {
		return nil, err
	}
	operation := SyncOperationEnabled
	switch status {
	case repository.SAGEInstallationStatusDisabled:
		operation = SyncOperationDisabled
	case repository.SAGEInstallationStatusUninstalled:
		operation = SyncOperationUninstalled
	}
	s.recordPluginSync(userID, deviceID, inst.ID.String(), operation, map[string]any{"installation_id": inst.ID.String(), "plugin_id": inst.PluginID.String(), "version_id": inst.VersionID.String(), "status": status})
	return inst, nil
}
func (s *SAGEPluginService) ListInstallations(userID uuid.UUID) ([]model.SAGEPluginInstallation, error) {
	return s.repo.ListInstallations(userID)
}

func (s *SAGEPluginService) GrantPermission(userID uuid.UUID, deviceID string, installationID uuid.UUID, input GrantSAGEPermissionInput) (*model.SAGEPluginPermissionGrant, error) {
	inst, err := s.repo.GetInstallation(installationID)
	if err != nil || inst.UserID != userID {
		return nil, ErrSAGEInstallationNotFound
	}
	key := strings.TrimSpace(input.PermissionKey)
	if key == "" {
		return nil, fmt.Errorf("%w: permission_key is required", ErrSAGEInvalid)
	}
	risk := knownSAGEPermissions[key]
	if risk == "" {
		risk = SAGERiskMedium
	}
	if _, err := s.enforceGovernance(GovernanceEnforcementInput{ActorUserID: &userID, ActorDeviceID: deviceID, SubjectType: GovernanceSubjectSAGEPermissionGrant, SubjectID: installationID.String(), CapabilityKey: key, RiskLevel: risk, Context: map[string]any{"installation_id": installationID.String(), "plugin_id": inst.PluginID.String(), "permission_key": key, "grant_scope": input.GrantScope}, ApprovalToken: input.ApprovalToken, ApprovalConsumedBy: deviceID}); err != nil {
		return nil, err
	}
	requires := riskRank(risk) >= riskRank(SAGERiskHigh)
	if requires {
		if s.sensitiveOperationSvc == nil {
			return nil, fmt.Errorf("sensitive operation service unavailable")
		}
		if err := s.sensitiveOperationSvc.ConsumeConfirmation(userID, input.ConfirmationToken, SensitiveOperationSAGEHighRiskGrant, "sage_permission_grant"); err != nil {
			return nil, err
		}
	}
	grantScope := datatypes.JSONMap(input.GrantScope)
	if grantScope == nil {
		grantScope = datatypes.JSONMap{}
	}
	grant, err := s.repo.GetGrantByInstallationPermission(installationID, key)
	if err == nil {
		grant.Status = repository.SAGEGrantStatusActive
		grant.RiskLevel = risk
		grant.GrantScope = grantScope
		grant.RequiresConfirmation = requires
		grant.ExpiresAt = input.ExpiresAt
		grant.RevokedAt = nil
		if err := s.repo.UpdateGrant(grant); err != nil {
			return nil, err
		}
	} else {
		grant = &model.SAGEPluginPermissionGrant{InstallationID: installationID, UserID: userID, PluginID: inst.PluginID, PermissionKey: key, RiskLevel: risk, Status: repository.SAGEGrantStatusActive, GrantScope: grantScope, RequiresConfirmation: requires, ExpiresAt: input.ExpiresAt}
		if err := s.repo.CreateGrant(grant); err != nil {
			return nil, err
		}
	}
	s.recordPluginSync(userID, deviceID, installationID.String(), SyncOperationPermissionGranted, map[string]any{"installation_id": installationID.String(), "grant_id": grant.ID.String(), "permission_key": key, "plugin_id": inst.PluginID.String(), "risk_level": risk, "requires_confirmation": requires, "status": grant.Status})
	return grant, nil
}

func (s *SAGEPluginService) RevokeGrant(userID uuid.UUID, deviceID string, grantID uuid.UUID) (*model.SAGEPluginPermissionGrant, error) {
	grant, err := s.repo.GetGrant(grantID)
	if err != nil || grant.UserID != userID {
		return nil, ErrSAGEForbidden
	}
	now := time.Now().UTC()
	grant.Status = repository.SAGEGrantStatusRevoked
	grant.RevokedAt = &now
	if err := s.repo.UpdateGrant(grant); err != nil {
		return nil, err
	}
	s.recordPluginSync(userID, deviceID, grant.InstallationID.String(), SyncOperationPermissionRevoked, map[string]any{"installation_id": grant.InstallationID.String(), "grant_id": grant.ID.String(), "permission_key": grant.PermissionKey, "plugin_id": grant.PluginID.String(), "status": grant.Status})
	return grant, nil
}

func (s *SAGEPluginService) PolicyBundle(userID uuid.UUID, installationID uuid.UUID) (*SAGEPolicyBundle, error) {
	inst, err := s.repo.GetInstallation(installationID)
	if err != nil || inst.UserID != userID {
		return nil, ErrSAGEInstallationNotFound
	}
	if inst.Status != repository.SAGEInstallationStatusActive {
		return nil, ErrSAGEForbidden
	}
	plugin, _ := s.repo.GetPlugin(inst.PluginID)
	version, _ := s.repo.GetVersion(inst.VersionID)
	grants, _ := s.repo.ListActiveGrants(inst.ID)
	granted := []map[string]any{}
	grantedSet := map[string]bool{}
	for _, g := range grants {
		grantedSet[g.PermissionKey] = true
		granted = append(granted, map[string]any{"key": g.PermissionKey, "risk": g.RiskLevel, "scope": g.GrantScope})
	}
	denied := []string{}
	guards := []map[string]any{}
	if perms, ok := version.PermissionSummary["permissions"].([]any); ok {
		for _, p := range perms {
			key := fmt.Sprint(p)
			if !grantedSet[key] {
				denied = append(denied, key)
			}
			if riskRank(knownSAGEPermissions[key]) >= riskRank(SAGERiskHigh) {
				guards = append(guards, map[string]any{"id": "guard-" + strings.ReplaceAll(key, ".", "-"), "match": map[string]any{"permission": key}, "decision": "require_user_confirmation"})
			}
		}
	}
	return &SAGEPolicyBundle{PluginKey: plugin.PluginKey, Version: version.Version, PolicyBundleVersion: time.Now().UTC().Format("2006-01-02") + ".1", ManifestSnapshot: version.ManifestSnapshot, GrantedPermissions: granted, DeniedPermissions: denied, RuntimeGuards: guards, Reporting: map[string]any{"required": true, "endpoint": "/api/v1/sage/invocations/{id}/reports"}}, nil
}

func (s *SAGEPluginService) CreateInvocation(userID uuid.UUID, deviceID string, input CreateSAGEInvocationInput) (*model.SAGEPluginInvocation, error) {
	plugin, err := s.repo.GetPublishedPluginByKey(input.PluginKey)
	if err != nil {
		return nil, ErrSAGEPluginNotFound
	}
	inst, err := s.repo.GetInstallationByUserPlugin(userID, plugin.ID)
	if err != nil || inst.Status != repository.SAGEInstallationStatusActive {
		return nil, ErrSAGEForbidden
	}
	clientID := strings.TrimSpace(input.ClientRequestID)
	if clientID == "" {
		return nil, fmt.Errorf("%w: client_request_id is required", ErrSAGEInvalid)
	}
	if existing, err := s.repo.GetInvocationByClientRequest(userID, plugin.ID, clientID); err == nil {
		return existing, nil
	}
	now := time.Now().UTC()
	risk := strings.TrimSpace(input.RiskLevel)
	if risk == "" {
		risk = plugin.RiskLevel
	}
	capabilityKey := "sage.invocation.create"
	if len(input.PermissionsUsed) > 0 && strings.TrimSpace(input.PermissionsUsed[0]) != "" {
		capabilityKey = strings.TrimSpace(input.PermissionsUsed[0])
	}
	governanceResult, err := s.enforceGovernance(GovernanceEnforcementInput{ActorUserID: &userID, ActorDeviceID: deviceID, SubjectType: GovernanceSubjectSAGEInvocation, SubjectID: plugin.ID.String(), CapabilityKey: capabilityKey, RiskLevel: risk, Context: map[string]any{"plugin_key": plugin.PluginKey, "installation_id": inst.ID.String(), "client_request_id": clientID, "flow_id": input.FlowID, "permissions_used": input.PermissionsUsed}})
	if err != nil {
		return nil, err
	}
	policyDecision := datatypes.JSONMap(input.PolicyDecision)
	if policyDecision == nil {
		policyDecision = datatypes.JSONMap{}
	}
	if governanceResult != nil {
		policyDecision["mode"] = governanceResult.Mode
		policyDecision["would_have_blocked"] = governanceResult.WouldHaveBlocked
		if governanceResult.Decision != nil {
			policyDecision["governance_decision_id"] = governanceResult.Decision.ID.String()
			policyDecision["decision"] = governanceResult.Decision.Decision
		}
	}
	inv := &model.SAGEPluginInvocation{UserID: userID, DeviceID: deviceID, PluginID: plugin.ID, InstallationID: &inst.ID, ClientRequestID: clientID, UserIntent: strings.TrimSpace(input.UserIntent), FlowID: strings.TrimSpace(input.FlowID), FlowHash: strings.TrimSpace(input.FlowHash), Status: repository.SAGEInvocationStatusCreated, RiskLevel: risk, PolicyDecision: policyDecision, PermissionsUsed: datatypes.JSONSlice[string](input.PermissionsUsed), StartedAt: &now}
	if inv.PolicyDecision == nil {
		inv.PolicyDecision = datatypes.JSONMap{}
	}
	if err := s.repo.CreateInvocation(inv); err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *SAGEPluginService) enforceGovernance(input GovernanceEnforcementInput) (*GovernanceEnforcementResult, error) {
	if s.governanceEnforcer == nil {
		return nil, nil
	}
	return s.governanceEnforcer.Enforce(input)
}

func (s *SAGEPluginService) SubmitReport(userID uuid.UUID, invocationID uuid.UUID, input SubmitSAGEExecutionReportInput) (*model.SAGEPluginExecutionReport, error) {
	inv, err := s.repo.GetInvocation(invocationID)
	if err != nil || inv.UserID != userID {
		return nil, ErrSAGEForbidden
	}
	clientID := strings.TrimSpace(input.ClientReportID)
	if clientID == "" {
		return nil, fmt.Errorf("%w: client_report_id is required", ErrSAGEInvalid)
	}
	if existing, err := s.repo.GetReportByClientID(invocationID, clientID); err == nil {
		return existing, nil
	}
	report := &model.SAGEPluginExecutionReport{InvocationID: invocationID, ClientReportID: clientID, Status: strings.TrimSpace(input.Status), FlowID: strings.TrimSpace(input.FlowID), StepsCompleted: datatypes.JSONSlice[string](input.StepsCompleted), StepSummaries: datatypes.JSONMap(input.StepSummaries), Errors: datatypes.JSONSlice[string](input.Errors), UserConfirmations: datatypes.JSONSlice[string](input.UserConfirmations), PluginCallbacks: datatypes.JSONSlice[string](input.PluginCallbacks), TokensUsed: input.TokensUsed, Metering: datatypes.JSONMap(input.Metering)}
	if report.Status == "" {
		report.Status = "completed"
	}
	if report.StepSummaries == nil {
		report.StepSummaries = datatypes.JSONMap{}
	}
	if report.Metering == nil {
		report.Metering = datatypes.JSONMap{}
	}
	now := time.Now().UTC()
	if report.Status == "completed" {
		inv.Status = repository.SAGEInvocationStatusCompleted
		inv.CompletedAt = &now
	} else if report.Status == "failed" {
		inv.Status = repository.SAGEInvocationStatusFailed
		inv.FailedAt = &now
	}
	err = s.repo.Transaction(func(tx *repository.SAGEPluginRepo) error {
		if err := tx.CreateReport(report); err != nil {
			return err
		}
		if err := tx.UpdateInvocation(inv); err != nil {
			return err
		}
		plugin, _ := tx.GetPlugin(inv.PluginID)
		ledger := &model.SAGEPluginUsageLedger{DeveloperID: plugin.DeveloperID, PluginID: inv.PluginID, UserID: inv.UserID, InvocationID: inv.ID, ReportID: report.ID, EventType: "plugin_invocation." + report.Status, Quantity: 1, Currency: "CNY", Metadata: datatypes.JSONMap{"tokens_used": input.TokensUsed}}
		return tx.CreateUsageLedger(ledger)
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *SAGEPluginService) DeveloperMetrics(developerID uuid.UUID, pluginID uuid.UUID) (*SAGEDeveloperMetrics, error) {
	plugin, err := s.repo.GetPlugin(pluginID)
	if err != nil {
		return nil, ErrSAGEPluginNotFound
	}
	if plugin.DeveloperID != developerID {
		return nil, ErrSAGEForbidden
	}
	inv, completed, tokens, err := s.repo.DeveloperMetrics(developerID, pluginID)
	if err != nil {
		return nil, err
	}
	failed := inv - completed
	rate := 0.0
	if inv > 0 {
		rate = float64(completed) / float64(inv)
	}
	return &SAGEDeveloperMetrics{PluginKey: plugin.PluginKey, Period: "all_time", Invocations: inv, Completed: completed, Failed: failed, SuccessRate: rate, TokensUsed: tokens}, nil
}

func (s *SAGEPluginService) requireActiveOwnedAsset(ownerID uuid.UUID, objectID uuid.UUID, assetKind string, allowedContentTypes []string) (*model.ObjectRecord, error) {
	if s.objectSvc == nil {
		return nil, fmt.Errorf("%w: object service unavailable", ErrSAGEInvalid)
	}
	record, err := s.objectSvc.RequireActiveOwnedObject(ownerID, objectID)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.TrimSpace(record.Scope), "sage-plugin-") {
		return nil, fmt.Errorf("%w: %s object scope must start with sage-plugin-", ErrSAGEInvalid, assetKind)
	}
	for _, allowed := range allowedContentTypes {
		if strings.HasSuffix(allowed, "/") {
			if strings.HasPrefix(record.ContentType, allowed) {
				return record, nil
			}
			continue
		}
		if record.ContentType == allowed {
			return record, nil
		}
	}
	return nil, fmt.Errorf("%w: unsupported %s content type %s", ErrSAGEInvalid, assetKind, record.ContentType)
}

func (s *SAGEPluginService) enrichCatalogPlugin(plugin *model.SAGEPlugin) (*SAGECatalogPlugin, error) {
	item := &SAGECatalogPlugin{SAGEPlugin: *plugin}
	if s.objectSvc == nil {
		return item, nil
	}
	if plugin.IconObjectID != nil {
		record, err := s.objectSvc.RequireActiveOwnedObject(plugin.DeveloperID, *plugin.IconObjectID)
		if err == nil {
			item.IconAsset = catalogAssetFromObject(record)
		}
	}
	if plugin.ApprovedVersionID != nil {
		version, err := s.repo.GetVersion(*plugin.ApprovedVersionID)
		if err == nil && version.PackageObjectID != nil {
			record, err := s.objectSvc.RequireActiveOwnedObject(plugin.DeveloperID, *version.PackageObjectID)
			if err == nil {
				item.PackageAsset = catalogAssetFromObject(record)
			}
		}
	}
	return item, nil
}

func catalogAssetFromObject(record *model.ObjectRecord) *SAGECatalogPluginAsset {
	return &SAGECatalogPluginAsset{ObjectID: record.ID, Filename: record.Filename, ContentType: record.ContentType, ContentHash: record.ContentHash, ContentSize: record.ContentSize}
}

func (s *SAGEPluginService) recordPluginSync(userID uuid.UUID, deviceID, installationID, operation string, payload map[string]any) {
	if s.syncSvc == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["object_id"] = installationID
	payload["object_type"] = SyncObjectPlugin
	_, _ = s.syncSvc.RecordEnvelope(SyncEnvelope{UserID: userID, SourceDeviceID: deviceID, ObjectType: SyncObjectPlugin, ObjectID: installationID, Operation: operation, ClientEventID: "", Payload: datatypes.JSONMap(payload)})
}
func (s *SAGEPluginService) recordAudit(actorID *uuid.UUID, actorDeviceID, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	if s.auditSvc == nil {
		return
	}
	_, _ = s.auditSvc.Record(RecordAuditEventInput{ActorUserID: actorID, ActorDeviceID: actorDeviceID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, Metadata: metadata})
}
func isGormNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
