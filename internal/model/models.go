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
	PubKeyEd25519 string `gorm:"uniqueIndex;not null" json:"pubkey_ed25519"`
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
	DeviceID     string     `gorm:"index;not null" json:"device_id"`
	DeviceName   string     `json:"device_name"`
	DevicePubKey string     `json:"device_pubkey"`
	PairedAt     time.Time  `json:"paired_at"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
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
	UserPubKey string `gorm:"index;not null" json:"user_pubkey"`
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
	UserID    uuid.UUID         `gorm:"type:uuid;index" json:"user_id"`
	DeviceID  string            `gorm:"index" json:"device_id"`
	EventType string            `gorm:"index" json:"event_type"`
	Payload   datatypes.JSONMap `gorm:"type:jsonb" json:"payload"`
	Timestamp time.Time         `gorm:"index" json:"timestamp"`
	Sequence  uint64            `gorm:"index" json:"sequence"`
}
type SyncCursor struct {
	UserID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	DeviceID           string    `gorm:"primaryKey" json:"device_id"`
	LastSyncedSequence uint64    `json:"last_synced_sequence"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type KBCollection struct {
	Base
	OwnerID          uuid.UUID `gorm:"type:uuid;index" json:"owner_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	PricingModel     string    `json:"pricing_model"`
	MonthlyPrice     int64     `json:"monthly_price"`
	IsFree           bool      `json:"is_free"`
	PlatformMinPrice int64     `json:"platform_min_price"`
	PlatformMaxPrice int64     `json:"platform_max_price"`
}
type KBSnapshot struct {
	Base
	CollectionID uuid.UUID `gorm:"type:uuid;index" json:"collection_id"`
	Version      int       `json:"version"`
	EntryCount   int       `json:"entry_count"`
	TotalTokens  int       `json:"total_tokens"`
	PublishedAt  time.Time `json:"published_at"`
	Checksum     string    `json:"checksum"`
}
type KBSnapshotEntry struct {
	Base
	SnapshotID    uuid.UUID                   `gorm:"type:uuid;index" json:"snapshot_id"`
	EntryID       string                      `gorm:"index" json:"entry_id"`
	Title         string                      `json:"title"`
	Summary       string                      `json:"summary"`
	Tags          datatypes.JSONSlice[string] `gorm:"type:jsonb" json:"tags"`
	Metadata      datatypes.JSONMap           `gorm:"type:jsonb" json:"metadata"`
	ContentPath   string                      `json:"content_path"`
	EmbeddingPath string                      `json:"embedding_path"`
	Tokens        int                         `json:"tokens"`
}
type KBSubscription struct {
	Base
	UserID        uuid.UUID  `gorm:"type:uuid;index" json:"user_id"`
	CollectionID  uuid.UUID  `gorm:"type:uuid;index" json:"collection_id"`
	TrackMode     string     `json:"track_mode"`
	PinnedVersion *int       `json:"pinned_version"`
	StartedAt     time.Time  `json:"started_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
}
type KBUsageRecord struct {
	Base
	UserID        uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	CollectionID  uuid.UUID `gorm:"type:uuid;index" json:"collection_id"`
	SnapshotID    uuid.UUID `gorm:"type:uuid;index" json:"snapshot_id"`
	OperationType string    `json:"operation_type"`
	TokensUsed    int       `json:"tokens_used"`
	BilledAt      time.Time `json:"billed_at"`
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
	return []any{&User{}, &Device{}, &AuthChallenge{}, &AdmissionRequest{}, &ServerAdmission{}, &Conversation{}, &ConversationParticipant{}, &Message{}, &OfflineMessage{}, &SyncEvent{}, &SyncCursor{}, &KBCollection{}, &KBSnapshot{}, &KBSnapshotEntry{}, &KBSubscription{}, &KBUsageRecord{}, &Plugin{}, &PluginVersion{}, &PluginUsageRecord{}, &BillingAccount{}, &BillingTransaction{}, &ContributorEarning{}}
}
