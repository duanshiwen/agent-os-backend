package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Base struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

type User struct {
	Base
	PubKeyEd25519 string `gorm:"column:pubkey_ed25519;uniqueIndex;not null" json:"pubkey_ed25519"`
	PasswordHash  string `json:"-"`
	DisplayName   string `json:"display_name"`
	AvatarURL     string `json:"avatar_url"`
	Status        string `gorm:"default:active" json:"status"`
	Role          string `gorm:"default:user" json:"role"`
	IsAdmin       bool   `gorm:"default:false" json:"is_admin"`
}
type Device struct {
	Base
	UserID       uuid.UUID  `gorm:"type:uuid;index;not null" json:"user_id"`
	DeviceID     string     `gorm:"uniqueIndex;not null" json:"device_id"`
	DeviceName   string     `json:"device_name"`
	DevicePubKey string     `json:"device_pubkey"`
	Status       string     `gorm:"index;not null;default:active" json:"status"`
	PairedAt     time.Time  `json:"paired_at"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	RevokedAt    *time.Time `json:"revoked_at"`
	RevokedBy    *uuid.UUID `gorm:"type:uuid" json:"revoked_by"`
}

type DevicePairingSession struct {
	Base
	UserID            uuid.UUID  `gorm:"type:uuid;index;not null" json:"user_id"`
	PairingTokenHash  string     `gorm:"not null" json:"-"`
	QRPayloadHash     string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt         time.Time  `gorm:"index;not null" json:"expires_at"`
	UsedAt            *time.Time `json:"used_at"`
	CreatedByDeviceID string     `gorm:"index;not null" json:"created_by_device_id"`
	ClaimedByDeviceID string     `gorm:"index" json:"claimed_by_device_id"`
}

type AuthChallenge struct {
	Base
	DeviceID   string     `gorm:"index;not null" json:"device_id"`
	UserPubKey string     `gorm:"index;not null" json:"user_pubkey"`
	Nonce      string     `gorm:"not null" json:"nonce"`
	Challenge  string     `gorm:"not null" json:"challenge"`
	ExpiresAt  time.Time  `json:"expires_at"`
	UsedAt     *time.Time `json:"used_at"`
}
type AdmissionRequest struct {
	Base
	UserPubKey string `gorm:"column:user_pubkey;index;not null" json:"user_pubkey"`
	DeviceID   string `json:"device_id"`
	Status     string `gorm:"default:pending" json:"status"`
	Reason     string `json:"reason"`
}

type ServerAdmission struct {
	Base
	ServerID              string     `gorm:"uniqueIndex;not null" json:"server_id"`
	PolicyType            string     `gorm:"not null" json:"policy_type"`
	InvitationCodeHash    string     `json:"-"`
	AdminApprovalRequired bool       `gorm:"default:false" json:"admin_approval_required"`
	UpdatedBy             *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
}

type Conversation struct {
	Base
	Type      string    `gorm:"index;not null" json:"type"`
	Name      string    `json:"name"`
	CreatedBy uuid.UUID `gorm:"type:uuid" json:"created_by"`
	GeoLat    *float64  `json:"geo_lat"`
	GeoLng    *float64  `json:"geo_lng"`
	GeoRadius *float64  `json:"geo_radius"`
}
type ConversationParticipant struct {
	ConversationID uuid.UUID `gorm:"type:uuid;primaryKey" json:"conversation_id"`
	UserID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	Role           string    `json:"role"`
	JoinedAt       time.Time `json:"joined_at"`
}
type Message struct {
	Base
	ConversationID uuid.UUID         `gorm:"type:uuid;index;not null" json:"conversation_id"`
	SenderID       uuid.UUID         `gorm:"type:uuid;index;not null" json:"sender_id"`
	Type           string            `json:"type"`
	Content        string            `json:"content"`
	Metadata       datatypes.JSONMap `gorm:"type:jsonb" json:"metadata"`
	DeliveredAt    *time.Time        `json:"delivered_at"`
}
type OfflineMessage struct {
	Base
	UserID    uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	DeviceID  string    `gorm:"index" json:"device_id"`
	MessageID uuid.UUID `gorm:"type:uuid;index" json:"message_id"`
	Message   Message   `gorm:"foreignKey:MessageID" json:"message,omitempty"`
	Delivered bool      `json:"delivered"`
}

type SyncEvent struct {
	Base
	UserID         uuid.UUID         `gorm:"type:uuid;index" json:"user_id"`
	DeviceID       string            `gorm:"index" json:"device_id"`
	EventType      string            `gorm:"index" json:"event_type"`
	SchemaVersion  int               `gorm:"default:1;not null" json:"schema_version"`
	ObjectType     string            `gorm:"index;not null" json:"object_type"`
	ObjectID       string            `gorm:"index;not null" json:"object_id"`
	Operation      string            `gorm:"index;not null" json:"operation"`
	SourceDeviceID string            `gorm:"index;not null" json:"source_device_id"`
	ClientEventID  string            `gorm:"index;not null" json:"client_event_id"`
	Payload        datatypes.JSONMap `gorm:"type:jsonb" json:"payload"`
	Timestamp      time.Time         `gorm:"index" json:"timestamp"`
	Sequence       uint64            `gorm:"index" json:"sequence"`
}
type SyncCursor struct {
	UserID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	DeviceID           string    `gorm:"primaryKey" json:"device_id"`
	LastSyncedSequence uint64    `json:"last_synced_sequence"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type SyncSequence struct {
	UserID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	NextSequence uint64    `gorm:"not null;default:1" json:"next_sequence"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UserSkillSetting struct {
	Base
	UserID            uuid.UUID         `gorm:"type:uuid;index;not null;uniqueIndex:idx_user_skill_setting" json:"user_id"`
	SkillID           string            `gorm:"not null;uniqueIndex:idx_user_skill_setting" json:"skill_id"`
	Enabled           bool              `gorm:"default:false" json:"enabled"`
	Config            datatypes.JSONMap `gorm:"type:jsonb" json:"config"`
	UpdatedByDeviceID string            `gorm:"index" json:"updated_by_device_id"`
}

type UserAgentSetting struct {
	Base
	UserID            uuid.UUID         `gorm:"type:uuid;index;not null;uniqueIndex:idx_user_agent_setting" json:"user_id"`
	AgentID           string            `gorm:"not null;uniqueIndex:idx_user_agent_setting" json:"agent_id"`
	DisplayName       string            `json:"display_name"`
	Config            datatypes.JSONMap `gorm:"type:jsonb" json:"config"`
	UpdatedByDeviceID string            `gorm:"index" json:"updated_by_device_id"`
}

type UserServerConnection struct {
	Base
	UserID            uuid.UUID         `gorm:"type:uuid;index;not null;uniqueIndex:idx_user_server_connection" json:"user_id"`
	ServerID          string            `gorm:"not null;uniqueIndex:idx_user_server_connection" json:"server_id"`
	Name              string            `json:"name"`
	BaseURL           string            `json:"base_url"`
	Status            string            `gorm:"default:active" json:"status"`
	Config            datatypes.JSONMap `gorm:"type:jsonb" json:"config"`
	UpdatedByDeviceID string            `gorm:"index" json:"updated_by_device_id"`
}

type UserKnowledgeEntry struct {
	Base
	UserID            uuid.UUID                   `gorm:"type:uuid;index;not null;uniqueIndex:idx_user_knowledge_entry" json:"user_id"`
	EntryID           string                      `gorm:"not null;uniqueIndex:idx_user_knowledge_entry" json:"entry_id"`
	Title             string                      `gorm:"not null" json:"title"`
	ContentMarkdown   string                      `gorm:"type:text;not null" json:"content_markdown"`
	Summary           string                      `json:"summary"`
	Tags              datatypes.JSONSlice[string] `gorm:"type:jsonb" json:"tags"`
	Metadata          datatypes.JSONMap           `gorm:"type:jsonb" json:"metadata"`
	SourceURI         string                      `json:"source_uri"`
	Status            string                      `gorm:"default:active;index" json:"status"`
	Version           uint64                      `gorm:"not null;default:1" json:"version"`
	ContentHash       string                      `gorm:"index" json:"content_hash"`
	DeletedAt         *time.Time                  `json:"deleted_at"`
	UpdatedByDeviceID string                      `gorm:"index" json:"updated_by_device_id"`
}

type ObjectRecord struct {
	Base
	OwnerID     uuid.UUID  `gorm:"type:uuid;index;not null" json:"owner_id"`
	Scope       string     `gorm:"index;not null" json:"scope"`
	Bucket      string     `gorm:"not null" json:"bucket"`
	ObjectKey   string     `gorm:"uniqueIndex;not null" json:"object_key"`
	ObjectURI   string     `gorm:"uniqueIndex;not null" json:"object_uri"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	ContentHash string     `gorm:"index;not null" json:"content_hash"`
	ContentSize int64      `json:"content_size"`
	Status      string     `gorm:"index;not null;default:pending" json:"status"`
	RefCount    int        `gorm:"not null;default:0" json:"ref_count"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CompletedAt *time.Time `json:"completed_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at" json:"deleted_at"`
}

type KBCollection struct {
	Base
	OwnerID              uuid.UUID         `gorm:"type:uuid;index;not null" json:"owner_id"`
	Name                 string            `gorm:"not null" json:"name"`
	Description          string            `json:"description"`
	Status               string            `gorm:"index;not null;default:draft" json:"status"`
	ReviewStatus         string            `gorm:"index;not null;default:pending" json:"review_status"`
	ReviewReason         string            `json:"review_reason"`
	ReviewedBy           *uuid.UUID        `gorm:"type:uuid;index" json:"reviewed_by"`
	ReviewedAt           *time.Time        `json:"reviewed_at"`
	TakedownReason       string            `json:"takedown_reason"`
	TakedownAt           *time.Time        `json:"takedown_at"`
	SourceDeclaration    string            `gorm:"type:text" json:"source_declaration"`
	CopyrightDeclaration string            `gorm:"type:text" json:"copyright_declaration"`
	ModerationMetadata   datatypes.JSONMap `gorm:"type:jsonb" json:"moderation_metadata"`
	PricingModel         string            `json:"pricing_model"`
	MonthlyPrice         int64             `json:"monthly_price"`
	IsFree               bool              `json:"is_free"`
	PlatformMinPrice     int64             `json:"platform_min_price"`
	PlatformMaxPrice     int64             `json:"platform_max_price"`
	EntitlementMode      string            `gorm:"index;not null;default:free" json:"entitlement_mode"`
	TrialDays            int               `gorm:"not null;default:0" json:"trial_days"`
	BillingInterval      string            `gorm:"not null;default:month" json:"billing_interval"`
	Currency             string            `gorm:"not null;default:CNY" json:"currency"`
}
type KBSnapshot struct {
	Base
	CollectionID      uuid.UUID  `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_snapshot_collection_version" json:"collection_id"`
	Version           int        `gorm:"not null;uniqueIndex:idx_kb_snapshot_collection_version" json:"version"`
	Status            string     `gorm:"index;not null;default:active" json:"status"`
	EntryCount        int        `json:"entry_count"`
	TotalTokens       int        `json:"total_tokens"`
	PublishedAt       time.Time  `gorm:"index" json:"published_at"`
	ArchivedAt        *time.Time `json:"archived_at"`
	Checksum          string     `gorm:"index" json:"checksum"`
	ManifestObjectURI string     `json:"manifest_object_uri"`
	ArchiveObjectURI  string     `json:"archive_object_uri"`
	ContentHash       string     `gorm:"index" json:"content_hash"`
	ContentSize       int64      `json:"content_size"`
}
type KBSnapshotEntry struct {
	Base
	SnapshotID       uuid.UUID                   `gorm:"type:uuid;index;not null" json:"snapshot_id"`
	EntryID          string                      `gorm:"index;not null" json:"entry_id"`
	Title            string                      `json:"title"`
	Summary          string                      `json:"summary"`
	Tags             datatypes.JSONSlice[string] `gorm:"type:jsonb" json:"tags"`
	Metadata         datatypes.JSONMap           `gorm:"type:jsonb" json:"metadata"`
	ContentObjectURI string                      `json:"content_object_uri"`
	EmbeddingPath    string                      `json:"embedding_path"`
	Tokens           int                         `json:"tokens"`
}
type KBSubscription struct {
	Base
	UserID             uuid.UUID  `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_subscription_user_collection" json:"user_id"`
	CollectionID       uuid.UUID  `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_subscription_user_collection" json:"collection_id"`
	SnapshotID         uuid.UUID  `gorm:"type:uuid;index" json:"snapshot_id"`
	TrackMode          string     `gorm:"index;not null;default:latest" json:"track_mode"`
	PinnedVersion      *int       `json:"pinned_version"`
	Status             string     `gorm:"index;not null;default:active" json:"status"`
	StartedAt          time.Time  `json:"started_at"`
	ExpiresAt          *time.Time `json:"expires_at"`
	EntitlementType    string     `gorm:"index;not null;default:free" json:"entitlement_type"`
	RenewalStatus      string     `gorm:"index;not null;default:none" json:"renewal_status"`
	CurrentPeriodStart *time.Time `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time `gorm:"index" json:"current_period_end"`
	GrantedBy          *uuid.UUID `gorm:"type:uuid;index" json:"granted_by"`
	GrantReason        string     `gorm:"type:text" json:"grant_reason"`
}
type KBModerationReport struct {
	Base
	CollectionID uuid.UUID  `gorm:"type:uuid;index;not null" json:"collection_id"`
	ReporterID   uuid.UUID  `gorm:"type:uuid;index;not null" json:"reporter_id"`
	Reason       string     `gorm:"index;not null" json:"reason"`
	Detail       string     `gorm:"type:text" json:"detail"`
	Status       string     `gorm:"index;not null;default:open" json:"status"`
	ResolvedBy   *uuid.UUID `gorm:"type:uuid;index" json:"resolved_by"`
	ResolvedAt   *time.Time `json:"resolved_at"`
	Resolution   string     `gorm:"type:text" json:"resolution"`
}

type KBUsageRecord struct {
	Base
	UserID          uuid.UUID  `gorm:"type:uuid;index" json:"user_id"`
	CollectionID    uuid.UUID  `gorm:"type:uuid;index;not null" json:"collection_id"`
	SnapshotID      uuid.UUID  `gorm:"type:uuid;index;not null" json:"snapshot_id"`
	SnapshotEntryID *uuid.UUID `gorm:"type:uuid;index" json:"snapshot_entry_id"`
	OperationType   string     `gorm:"index;not null" json:"operation_type"`
	TokensUsed      int        `json:"tokens_used"`
	UnitPrice       int64      `json:"unit_price"`
	Amount          int64      `json:"amount"`
	Currency        string     `gorm:"default:CNY" json:"currency"`
	BilledAt        time.Time  `gorm:"index" json:"billed_at"`
}

type KBSearchDocument struct {
	Base
	CollectionID    uuid.UUID                   `gorm:"type:uuid;index;not null" json:"collection_id"`
	SnapshotID      uuid.UUID                   `gorm:"type:uuid;index;not null" json:"snapshot_id"`
	SnapshotEntryID uuid.UUID                   `gorm:"type:uuid;index;not null" json:"snapshot_entry_id"`
	EntryID         string                      `gorm:"index;not null" json:"entry_id"`
	Title           string                      `gorm:"index" json:"title"`
	Summary         string                      `json:"summary"`
	Tags            datatypes.JSONSlice[string] `gorm:"type:jsonb" json:"tags"`
	Metadata        datatypes.JSONMap           `gorm:"type:jsonb" json:"metadata"`
	ContentText     string                      `gorm:"type:text" json:"content_text"`
	ContentHash     string                      `gorm:"index" json:"content_hash"`
	Tokens          int                         `json:"tokens"`
	Status          string                      `gorm:"index;not null;default:active" json:"status"`
	IndexedAt       time.Time                   `gorm:"index" json:"indexed_at"`
}

type KBEmbeddingJob struct {
	Base
	SearchDocumentID uuid.UUID  `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_embedding_job_doc_provider_model_hash" json:"search_document_id"`
	CollectionID     uuid.UUID  `gorm:"type:uuid;index;not null" json:"collection_id"`
	SnapshotID       uuid.UUID  `gorm:"type:uuid;index;not null" json:"snapshot_id"`
	SnapshotEntryID  uuid.UUID  `gorm:"type:uuid;index;not null" json:"snapshot_entry_id"`
	Provider         string     `gorm:"not null;uniqueIndex:idx_kb_embedding_job_doc_provider_model_hash" json:"provider"`
	Model            string     `gorm:"not null;uniqueIndex:idx_kb_embedding_job_doc_provider_model_hash" json:"model"`
	Dimensions       int        `gorm:"not null;default:1024" json:"dimensions"`
	ContentHash      string     `gorm:"index;not null;uniqueIndex:idx_kb_embedding_job_doc_provider_model_hash" json:"content_hash"`
	Status           string     `gorm:"index;not null;default:pending" json:"status"`
	Priority         int        `gorm:"index;not null;default:100" json:"priority"`
	Attempts         int        `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts      int        `gorm:"not null;default:5" json:"max_attempts"`
	AvailableAt      time.Time  `gorm:"index;not null" json:"available_at"`
	LockedAt         *time.Time `json:"locked_at"`
	LockedBy         string     `gorm:"index" json:"locked_by"`
	LastError        string     `json:"last_error"`
	StartedAt        *time.Time `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	FailedAt         *time.Time `json:"failed_at"`
}

type KBSearchEmbedding struct {
	Base
	SearchDocumentID uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_search_embedding_doc_provider_model_hash" json:"search_document_id"`
	CollectionID     uuid.UUID `gorm:"type:uuid;index;not null" json:"collection_id"`
	SnapshotID       uuid.UUID `gorm:"type:uuid;index;not null" json:"snapshot_id"`
	SnapshotEntryID  uuid.UUID `gorm:"type:uuid;index;not null" json:"snapshot_entry_id"`
	Provider         string    `gorm:"not null;uniqueIndex:idx_kb_search_embedding_doc_provider_model_hash" json:"provider"`
	Model            string    `gorm:"not null;uniqueIndex:idx_kb_search_embedding_doc_provider_model_hash" json:"model"`
	Dimensions       int       `gorm:"not null;default:1024" json:"dimensions"`
	ContentHash      string    `gorm:"index;not null;uniqueIndex:idx_kb_search_embedding_doc_provider_model_hash" json:"content_hash"`
	Embedding        string    `gorm:"type:vector(1024);not null" json:"embedding"`
}

type Plugin struct {
	Base
	DeveloperID   uuid.UUID         `gorm:"type:uuid;index" json:"developer_id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	ScapeManifest datatypes.JSONMap `gorm:"type:jsonb" json:"scape_manifest"`
	Status        string            `gorm:"default:pending" json:"status"`
	Version       string            `json:"version"`
}
type PluginVersion struct {
	Base
	PluginID     uuid.UUID         `gorm:"type:uuid;index" json:"plugin_id"`
	Version      string            `json:"version"`
	ManifestJSON datatypes.JSONMap `gorm:"type:jsonb" json:"manifest_json"`
	Changelog    string            `json:"changelog"`
	PublishedAt  *time.Time        `json:"published_at"`
	ReviewedAt   *time.Time        `json:"reviewed_at"`
	ReviewerID   *uuid.UUID        `gorm:"type:uuid" json:"reviewer_id"`
}
type PluginUsageRecord struct {
	Base
	UserID      uuid.UUID  `gorm:"type:uuid;index" json:"user_id"`
	PluginID    uuid.UUID  `gorm:"type:uuid;index" json:"plugin_id"`
	SessionID   string     `json:"session_id"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	TokensUsed  int        `json:"tokens_used"`
}

type KBBillingPlan struct {
	Base
	CollectionID    uuid.UUID         `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_billing_plan_collection_version" json:"collection_id"`
	Version         int               `gorm:"not null;uniqueIndex:idx_kb_billing_plan_collection_version" json:"version"`
	EntitlementType string            `gorm:"index;not null" json:"entitlement_type"`
	BillingInterval string            `gorm:"not null;default:month" json:"billing_interval"`
	Price           int64             `gorm:"not null;default:0" json:"price"`
	Currency        string            `gorm:"not null;default:CNY" json:"currency"`
	TrialDays       int               `gorm:"not null;default:0" json:"trial_days"`
	Status          string            `gorm:"index;not null;default:active" json:"status"`
	EffectiveAt     time.Time         `gorm:"index;not null" json:"effective_at"`
	Metadata        datatypes.JSONMap `gorm:"type:jsonb" json:"metadata"`
}

type KBInvoice struct {
	Base
	UserID         uuid.UUID         `gorm:"type:uuid;index;not null" json:"user_id"`
	CollectionID   *uuid.UUID        `gorm:"type:uuid;index" json:"collection_id"`
	SubscriptionID *uuid.UUID        `gorm:"type:uuid;index" json:"subscription_id"`
	Status         string            `gorm:"index;not null;default:draft" json:"status"`
	Currency       string            `gorm:"not null;default:CNY" json:"currency"`
	Subtotal       int64             `gorm:"not null;default:0" json:"subtotal"`
	PlatformFee    int64             `gorm:"not null;default:0" json:"platform_fee"`
	Total          int64             `gorm:"not null;default:0" json:"total"`
	PeriodStart    *time.Time        `json:"period_start"`
	PeriodEnd      *time.Time        `json:"period_end"`
	IssuedAt       *time.Time        `json:"issued_at"`
	PaidAt         *time.Time        `json:"paid_at"`
	VoidedAt       *time.Time        `json:"voided_at"`
	Metadata       datatypes.JSONMap `gorm:"type:jsonb" json:"metadata"`
}

type KBInvoiceItem struct {
	Base
	InvoiceID     uuid.UUID         `gorm:"type:uuid;index;not null" json:"invoice_id"`
	UsageRecordID *uuid.UUID        `gorm:"type:uuid;index" json:"usage_record_id"`
	Description   string            `gorm:"not null" json:"description"`
	Quantity      int64             `gorm:"not null;default:1" json:"quantity"`
	UnitPrice     int64             `gorm:"not null;default:0" json:"unit_price"`
	Amount        int64             `gorm:"not null;default:0" json:"amount"`
	Metadata      datatypes.JSONMap `gorm:"type:jsonb" json:"metadata"`
}

type ContributorPayoutPeriod struct {
	Base
	ContributorID uuid.UUID         `gorm:"type:uuid;index;not null;uniqueIndex:idx_contributor_payout_period" json:"contributor_id"`
	Period        string            `gorm:"index;not null;uniqueIndex:idx_contributor_payout_period" json:"period"`
	Status        string            `gorm:"index;not null;default:open" json:"status"`
	GrossAmount   int64             `gorm:"not null;default:0" json:"gross_amount"`
	PlatformFee   int64             `gorm:"not null;default:0" json:"platform_fee"`
	NetAmount     int64             `gorm:"not null;default:0" json:"net_amount"`
	PaidAt        *time.Time        `json:"paid_at"`
	Metadata      datatypes.JSONMap `gorm:"type:jsonb" json:"metadata"`
}

type KBRefund struct {
	Base
	InvoiceID   uuid.UUID         `gorm:"type:uuid;index;not null" json:"invoice_id"`
	UserID      uuid.UUID         `gorm:"type:uuid;index;not null" json:"user_id"`
	Amount      int64             `gorm:"not null" json:"amount"`
	Reason      string            `gorm:"type:text;not null" json:"reason"`
	Status      string            `gorm:"index;not null;default:pending" json:"status"`
	ProcessedBy *uuid.UUID        `gorm:"type:uuid;index" json:"processed_by"`
	ProcessedAt *time.Time        `json:"processed_at"`
	Metadata    datatypes.JSONMap `gorm:"type:jsonb" json:"metadata"`
}

type KBBillingDispute struct {
	Base
	InvoiceID  uuid.UUID         `gorm:"type:uuid;index;not null" json:"invoice_id"`
	UserID     uuid.UUID         `gorm:"type:uuid;index;not null" json:"user_id"`
	Reason     string            `gorm:"type:text;not null" json:"reason"`
	Detail     string            `gorm:"type:text" json:"detail"`
	Status     string            `gorm:"index;not null;default:open" json:"status"`
	ResolvedBy *uuid.UUID        `gorm:"type:uuid;index" json:"resolved_by"`
	ResolvedAt *time.Time        `json:"resolved_at"`
	Resolution string            `gorm:"type:text" json:"resolution"`
	Metadata   datatypes.JSONMap `gorm:"type:jsonb" json:"metadata"`
}

type BillingAccount struct {
	Base
	UserID   uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"user_id"`
	Balance  int64     `json:"balance"`
	Currency string    `gorm:"default:CNY" json:"currency"`
}
type BillingTransaction struct {
	Base
	UserID      uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	Type        string    `json:"type"`
	Amount      int64     `json:"amount"`
	Description string    `json:"description"`
	RelatedID   string    `json:"related_id"`
}
type ContributorEarning struct {
	Base
	ContributorID uuid.UUID `gorm:"type:uuid;index" json:"contributor_id"`
	CollectionID  uuid.UUID `gorm:"type:uuid;index" json:"collection_id"`
	Period        string    `json:"period"`
	GrossAmount   int64     `json:"gross_amount"`
	PlatformFee   int64     `json:"platform_fee"`
	NetAmount     int64     `json:"net_amount"`
}

type SensitiveOperationConfirmation struct {
	Base
	UserID         uuid.UUID  `gorm:"type:uuid;index;not null" json:"user_id"`
	DeviceID       string     `gorm:"index;not null" json:"device_id"`
	Operation      string     `gorm:"index;not null" json:"operation"`
	TokenHash      string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt      time.Time  `gorm:"index;not null" json:"expires_at"`
	UsedAt         *time.Time `json:"used_at"`
	ConsumedBy     string     `gorm:"index" json:"consumed_by"`
	RemoteMetadata string     `json:"remote_metadata"`
}

type AuditEvent struct {
	Base
	ActorUserID   *uuid.UUID        `gorm:"type:uuid;index" json:"actor_user_id"`
	ActorDeviceID string            `gorm:"index" json:"actor_device_id"`
	Action        string            `gorm:"index;not null" json:"action"`
	ResourceType  string            `gorm:"index;not null" json:"resource_type"`
	ResourceID    string            `gorm:"index" json:"resource_id"`
	Outcome       string            `gorm:"index;not null;default:success" json:"outcome"`
	IPAddress     string            `json:"ip_address"`
	UserAgent     string            `json:"user_agent"`
	Metadata      datatypes.JSONMap `gorm:"type:jsonb" json:"metadata"`
	OccurredAt    time.Time         `gorm:"index;not null" json:"occurred_at"`
}

func AllModels() []any {
	return []any{&User{}, &Device{}, &DevicePairingSession{}, &AuthChallenge{}, &AdmissionRequest{}, &ServerAdmission{}, &Conversation{}, &ConversationParticipant{}, &Message{}, &OfflineMessage{}, &SyncEvent{}, &SyncCursor{}, &SyncSequence{}, &UserSkillSetting{}, &UserAgentSetting{}, &UserServerConnection{}, &UserKnowledgeEntry{}, &ObjectRecord{}, &KBCollection{}, &KBSnapshot{}, &KBSnapshotEntry{}, &KBSubscription{}, &KBModerationReport{}, &KBUsageRecord{}, &KBSearchDocument{}, &KBEmbeddingJob{}, &KBSearchEmbedding{}, &Plugin{}, &PluginVersion{}, &PluginUsageRecord{}, &KBBillingPlan{}, &KBInvoice{}, &KBInvoiceItem{}, &ContributorPayoutPeriod{}, &KBRefund{}, &KBBillingDispute{}, &BillingAccount{}, &BillingTransaction{}, &ContributorEarning{}, &SensitiveOperationConfirmation{}, &AuditEvent{}}
}
