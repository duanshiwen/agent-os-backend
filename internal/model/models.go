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
	PairedAt     time.Time  `json:"paired_at"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
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
	OwnerID          uuid.UUID `gorm:"type:uuid;index;not null" json:"owner_id"`
	Name             string    `gorm:"not null" json:"name"`
	Description      string    `json:"description"`
	Status           string    `gorm:"index;not null;default:draft" json:"status"`
	PricingModel     string    `json:"pricing_model"`
	MonthlyPrice     int64     `json:"monthly_price"`
	IsFree           bool      `json:"is_free"`
	PlatformMinPrice int64     `json:"platform_min_price"`
	PlatformMaxPrice int64     `json:"platform_max_price"`
}
type KBSnapshot struct {
	Base
	CollectionID      uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_snapshot_collection_version" json:"collection_id"`
	Version           int       `gorm:"not null;uniqueIndex:idx_kb_snapshot_collection_version" json:"version"`
	EntryCount        int       `json:"entry_count"`
	TotalTokens       int       `json:"total_tokens"`
	PublishedAt       time.Time `gorm:"index" json:"published_at"`
	Checksum          string    `gorm:"index" json:"checksum"`
	ManifestObjectURI string    `json:"manifest_object_uri"`
	ArchiveObjectURI  string    `json:"archive_object_uri"`
	ContentHash       string    `gorm:"index" json:"content_hash"`
	ContentSize       int64     `json:"content_size"`
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
	UserID        uuid.UUID  `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_subscription_user_collection" json:"user_id"`
	CollectionID  uuid.UUID  `gorm:"type:uuid;index;not null;uniqueIndex:idx_kb_subscription_user_collection" json:"collection_id"`
	SnapshotID    uuid.UUID  `gorm:"type:uuid;index" json:"snapshot_id"`
	TrackMode     string     `gorm:"index;not null;default:latest" json:"track_mode"`
	PinnedVersion *int       `json:"pinned_version"`
	Status        string     `gorm:"index;not null;default:active" json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
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

func AllModels() []any {
	return []any{&User{}, &Device{}, &DevicePairingSession{}, &AuthChallenge{}, &AdmissionRequest{}, &ServerAdmission{}, &Conversation{}, &ConversationParticipant{}, &Message{}, &OfflineMessage{}, &SyncEvent{}, &SyncCursor{}, &SyncSequence{}, &UserSkillSetting{}, &UserAgentSetting{}, &UserServerConnection{}, &UserKnowledgeEntry{}, &ObjectRecord{}, &KBCollection{}, &KBSnapshot{}, &KBSnapshotEntry{}, &KBSubscription{}, &KBUsageRecord{}, &KBSearchDocument{}, &KBEmbeddingJob{}, &KBSearchEmbedding{}, &Plugin{}, &PluginVersion{}, &PluginUsageRecord{}, &BillingAccount{}, &BillingTransaction{}, &ContributorEarning{}}
}
