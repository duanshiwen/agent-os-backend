package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type SAGEPlugin struct {
	Base
	PluginKey         string     `gorm:"uniqueIndex;not null" json:"plugin_key"`
	DeveloperID       uuid.UUID  `gorm:"type:uuid;index;not null" json:"developer_id"`
	Name              string     `gorm:"not null" json:"name"`
	Description       string     `gorm:"type:text" json:"description"`
	Category          string     `gorm:"index" json:"category"`
	IconObjectID      *uuid.UUID `gorm:"type:uuid" json:"icon_object_id"`
	HomepageURL       string     `json:"homepage_url"`
	ManifestURL       string     `json:"manifest_url"`
	FlowURL           string     `json:"flow_url"`
	CallbackURL       string     `json:"callback_url"`
	HealthURL         string     `json:"health_url"`
	Status            string     `gorm:"index;not null;default:draft" json:"status"`
	ReviewStatus      string     `gorm:"index;not null;default:pending" json:"review_status"`
	Visibility        string     `gorm:"index;not null;default:private" json:"visibility"`
	RiskLevel         string     `gorm:"index;not null;default:unknown" json:"risk_level"`
	TrustLevel        string     `gorm:"index;not null;default:unverified" json:"trust_level"`
	LatestVersionID   *uuid.UUID `gorm:"type:uuid" json:"latest_version_id"`
	ApprovedVersionID *uuid.UUID `gorm:"type:uuid" json:"approved_version_id"`
}

type SAGEPluginVersion struct {
	Base
	PluginID            uuid.UUID                   `gorm:"type:uuid;index;not null;uniqueIndex:idx_sage_plugin_version" json:"plugin_id"`
	Version             string                      `gorm:"not null;uniqueIndex:idx_sage_plugin_version" json:"version"`
	SAGEVersion         string                      `gorm:"not null;default:1.0" json:"sage_version"`
	ManifestHash        string                      `gorm:"index;not null" json:"manifest_hash"`
	ManifestSnapshot    datatypes.JSONMap           `gorm:"type:jsonb;not null" json:"manifest_snapshot"`
	RawManifestObjectID *uuid.UUID                  `gorm:"type:uuid" json:"raw_manifest_object_id"`
	ValidationStatus    string                      `gorm:"index;not null;default:pending" json:"validation_status"`
	ValidationErrors    datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"validation_errors"`
	ValidationWarnings  datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"validation_warnings"`
	RiskSummary         datatypes.JSONMap           `gorm:"type:jsonb;not null" json:"risk_summary"`
	PermissionSummary   datatypes.JSONMap           `gorm:"type:jsonb;not null" json:"permission_summary"`
	Status              string                      `gorm:"index;not null;default:draft" json:"status"`
	SubmittedAt         *time.Time                  `json:"submitted_at"`
	ApprovedAt          *time.Time                  `json:"approved_at"`
	RejectedAt          *time.Time                  `json:"rejected_at"`
}

type SAGEPluginReview struct {
	Base
	PluginID         uuid.UUID                   `gorm:"type:uuid;index;not null" json:"plugin_id"`
	VersionID        uuid.UUID                   `gorm:"type:uuid;index;not null" json:"version_id"`
	ReviewerID       uuid.UUID                   `gorm:"type:uuid;index;not null" json:"reviewer_id"`
	Decision         string                      `gorm:"index;not null" json:"decision"`
	Reason           string                      `gorm:"type:text" json:"reason"`
	SecurityFindings datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"security_findings"`
	PrivacyFindings  datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"privacy_findings"`
	PolicyFindings   datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"policy_findings"`
}

type SAGEPluginInstallation struct {
	Base
	UserID        uuid.UUID  `gorm:"type:uuid;index;not null;uniqueIndex:idx_sage_install_user_plugin" json:"user_id"`
	PluginID      uuid.UUID  `gorm:"type:uuid;index;not null;uniqueIndex:idx_sage_install_user_plugin" json:"plugin_id"`
	VersionID     uuid.UUID  `gorm:"type:uuid;index;not null" json:"version_id"`
	Status        string     `gorm:"index;not null;default:active" json:"status"`
	InstallSource string     `gorm:"not null;default:catalog" json:"install_source"`
	TrackMode     string     `gorm:"not null;default:latest_approved" json:"track_mode"`
	InstalledAt   time.Time  `gorm:"index;not null" json:"installed_at"`
	DisabledAt    *time.Time `json:"disabled_at"`
}

type SAGEPluginPermissionGrant struct {
	Base
	InstallationID       uuid.UUID         `gorm:"type:uuid;index;not null;uniqueIndex:idx_sage_grant_install_permission" json:"installation_id"`
	UserID               uuid.UUID         `gorm:"type:uuid;index;not null" json:"user_id"`
	PluginID             uuid.UUID         `gorm:"type:uuid;index;not null" json:"plugin_id"`
	PermissionKey        string            `gorm:"index;not null;uniqueIndex:idx_sage_grant_install_permission" json:"permission_key"`
	RiskLevel            string            `gorm:"index;not null;default:low" json:"risk_level"`
	Status               string            `gorm:"index;not null;default:active" json:"status"`
	GrantScope           datatypes.JSONMap `gorm:"type:jsonb;not null" json:"grant_scope"`
	RequiresConfirmation bool              `gorm:"not null;default:false" json:"requires_confirmation"`
	ConfirmationID       *uuid.UUID        `gorm:"type:uuid" json:"confirmation_id"`
	ExpiresAt            *time.Time        `json:"expires_at"`
	RevokedAt            *time.Time        `json:"revoked_at"`
}

type SAGEPluginInvocation struct {
	Base
	UserID          uuid.UUID                   `gorm:"type:uuid;index;not null;uniqueIndex:idx_sage_invocation_client_request" json:"user_id"`
	DeviceID        string                      `gorm:"index;not null" json:"device_id"`
	PluginID        uuid.UUID                   `gorm:"type:uuid;index;not null;uniqueIndex:idx_sage_invocation_client_request" json:"plugin_id"`
	InstallationID  *uuid.UUID                  `gorm:"type:uuid;index" json:"installation_id"`
	ClientRequestID string                      `gorm:"index;not null;uniqueIndex:idx_sage_invocation_client_request" json:"client_request_id"`
	UserIntent      string                      `gorm:"type:text" json:"user_intent"`
	FlowID          string                      `gorm:"index" json:"flow_id"`
	FlowHash        string                      `gorm:"index" json:"flow_hash"`
	Status          string                      `gorm:"index;not null;default:created" json:"status"`
	RiskLevel       string                      `gorm:"index;not null;default:unknown" json:"risk_level"`
	PolicyDecision  datatypes.JSONMap           `gorm:"type:jsonb;not null" json:"policy_decision"`
	PermissionsUsed datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"permissions_used"`
	StartedAt       *time.Time                  `json:"started_at"`
	CompletedAt     *time.Time                  `json:"completed_at"`
	FailedAt        *time.Time                  `json:"failed_at"`
}

type SAGEPluginExecutionReport struct {
	Base
	InvocationID      uuid.UUID                   `gorm:"type:uuid;index;not null;uniqueIndex:idx_sage_report_invocation_client" json:"invocation_id"`
	ClientReportID    string                      `gorm:"index;not null;uniqueIndex:idx_sage_report_invocation_client" json:"client_report_id"`
	Status            string                      `gorm:"index;not null" json:"status"`
	FlowID            string                      `gorm:"index" json:"flow_id"`
	StepsCompleted    datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"steps_completed"`
	StepSummaries     datatypes.JSONMap           `gorm:"type:jsonb;not null" json:"step_summaries"`
	Errors            datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"errors"`
	UserConfirmations datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"user_confirmations"`
	PluginCallbacks   datatypes.JSONSlice[string] `gorm:"type:jsonb;not null" json:"plugin_callbacks"`
	TokensUsed        int                         `gorm:"not null;default:0" json:"tokens_used"`
	Metering          datatypes.JSONMap           `gorm:"type:jsonb;not null" json:"metering"`
}

type SAGEPluginUsageLedger struct {
	Base
	DeveloperID  uuid.UUID         `gorm:"type:uuid;index;not null" json:"developer_id"`
	PluginID     uuid.UUID         `gorm:"type:uuid;index;not null" json:"plugin_id"`
	UserID       uuid.UUID         `gorm:"type:uuid;index;not null" json:"user_id"`
	InvocationID uuid.UUID         `gorm:"type:uuid;index;not null" json:"invocation_id"`
	ReportID     uuid.UUID         `gorm:"type:uuid;index" json:"report_id"`
	EventType    string            `gorm:"index;not null" json:"event_type"`
	Quantity     int64             `gorm:"not null;default:1" json:"quantity"`
	Amount       int64             `gorm:"not null;default:0" json:"amount"`
	Currency     string            `gorm:"not null;default:CNY" json:"currency"`
	Metadata     datatypes.JSONMap `gorm:"type:jsonb;not null" json:"metadata"`
}
