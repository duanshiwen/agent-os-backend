package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	AuditOutcomeSuccess = "success"
	AuditOutcomeFailure = "failure"
	AuditOutcomeDenied  = "denied"
)

const (
	AuditGenesisHash              = "GENESIS"
	AuditHashAlgorithmSHA256      = "sha256"
	AuditChainBreakHashMismatch   = "hash_mismatch"
	AuditChainBreakLinkMismatch   = "link_mismatch"
	AuditChainBreakSequenceGap    = "sequence_gap"
	AuditChainBreakMissingHash    = "missing_hash"
	AuditChainBreakUnsupportedAlg = "unsupported_hash_algorithm"
)

const (
	AuditActionAdmissionPolicyUpdated        = "admission.policy.updated"
	AuditActionAdmissionInvitationCodeSet    = "admission.invitation_code.set"
	AuditActionAdmissionRequestApproved      = "admission.request.approved"
	AuditActionAdmissionRequestRejected      = "admission.request.rejected"
	AuditActionDevicePairingStarted          = "device.pairing.started"
	AuditActionDevicePairingClaimed          = "device.pairing.claimed"
	AuditActionDeviceRenamed                 = "device.renamed"
	AuditActionDeviceRevoked                 = "device.revoked"
	AuditActionPasswordSet                   = "identity.password.set"
	AuditActionPasswordChanged               = "identity.password.changed"
	AuditActionSensitiveConfirmationIssued   = "sensitive_operation.confirmation.issued"
	AuditActionSensitiveConfirmationConsumed = "sensitive_operation.confirmation.consumed"
	AuditActionKBCollectionReviewed          = "kb.collection.reviewed"
	AuditActionKBCollectionReported          = "kb.collection.reported"
	AuditActionKBSnapshotArchived            = "kb.snapshot.archived"
	AuditActionKBSnapshotRestored            = "kb.snapshot.restored"
	AuditActionKBSubscriptionsExpired        = "kb.subscriptions.expired"
	AuditActionKBBillingPlanUpdated          = "kb.billing_plan.updated"
	AuditActionKBInvoiceIssued               = "kb.invoice.issued"
	AuditActionKBInvoicePaid                 = "kb.invoice.paid"
	AuditActionKBRefundRequested             = "kb.refund.requested"
	AuditActionKBRefundResolved              = "kb.refund.resolved"
	AuditActionKBBillingDisputeOpened        = "kb.billing_dispute.opened"
	AuditActionKBBillingDisputeResolved      = "kb.billing_dispute.resolved"
	AuditActionKBPayoutPaid                  = "kb.payout.paid"
	AuditActionSAGEPluginCreated             = "sage.plugin.created"
	AuditActionSAGEPluginVersionSubmitted    = "sage.plugin.version.submitted"
	AuditActionSAGEPluginReviewed            = "sage.plugin.reviewed"
	AuditActionSAGEPluginSuspended           = "sage.plugin.suspended"
	AuditActionSAGEPluginInstalled           = "sage.plugin.installed"
	AuditActionSkillCreated                  = "skill.created"
	AuditActionSkillVersionPublished         = "skill.version.published"
	AuditActionSkillInstalled                = "skill.installed"
	AuditActionSkillTakedown                 = "skill.takedown"
	AuditActionSkillPublisherRestricted      = "skill.publisher.restricted"
	AuditActionSkillPublisherRestrictionLift = "skill.publisher.restriction_lifted"
)

type AuditService struct {
	repo *repository.AuditRepo
}

type RecordAuditEventInput struct {
	ActorUserID   *uuid.UUID
	ActorDeviceID string
	Action        string
	ResourceType  string
	ResourceID    string
	Outcome       string
	IPAddress     string
	UserAgent     string
	Metadata      map[string]any
}

type ListAuditEventsInput struct {
	ActorUserID  *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      string
	Limit        int
	Offset       int
}

type AuditEventsPage struct {
	Items  []model.AuditEvent `json:"items"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
	Total  int64              `json:"total"`
}

type VerifyAuditHashChainInput struct {
	Limit int
}

type AuditHashChainVerification struct {
	Valid        bool                  `json:"valid"`
	Checked      int                   `json:"checked"`
	HeadHash     string                `json:"head_hash"`
	HeadSequence int64                 `json:"head_sequence"`
	Breaks       []AuditHashChainBreak `json:"breaks"`
}

type AuditHashChainBreak struct {
	Sequence int64     `json:"sequence"`
	EventID  uuid.UUID `json:"event_id"`
	Reason   string    `json:"reason"`
	Expected string    `json:"expected"`
	Actual   string    `json:"actual"`
}

func NewAuditService(repo *repository.AuditRepo) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) Record(input RecordAuditEventInput) (*model.AuditEvent, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	action := strings.TrimSpace(input.Action)
	resourceType := strings.TrimSpace(input.ResourceType)
	if action == "" || resourceType == "" {
		return nil, nil
	}
	outcome := strings.TrimSpace(input.Outcome)
	if outcome == "" {
		outcome = AuditOutcomeSuccess
	}
	event := &model.AuditEvent{
		ActorUserID:   input.ActorUserID,
		ActorDeviceID: strings.TrimSpace(input.ActorDeviceID),
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    strings.TrimSpace(input.ResourceID),
		Outcome:       outcome,
		IPAddress:     strings.TrimSpace(input.IPAddress),
		UserAgent:     strings.TrimSpace(input.UserAgent),
		OccurredAt:    time.Now().UTC(),
	}
	if input.Metadata != nil {
		event.Metadata = datatypes.JSONMap(input.Metadata)
	}
	if event.Metadata == nil {
		event.Metadata = datatypes.JSONMap{}
	}
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if err := s.repo.AppendHashChained(event, func(previous *model.AuditEvent, nextSequence int64) error {
		event.Sequence = nextSequence
		event.HashAlgorithm = AuditHashAlgorithmSHA256
		if previous == nil {
			event.PreviousHash = AuditGenesisHash
		} else {
			event.PreviousHash = previous.EventHash
		}
		hash, err := computeAuditEventHash(event)
		if err != nil {
			return err
		}
		event.EventHash = hash
		return nil
	}); err != nil {
		return nil, err
	}
	return event, nil
}

func (s *AuditService) List(input ListAuditEventsInput) (*AuditEventsPage, error) {
	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if input.Offset < 0 {
		input.Offset = 0
	}
	query := repository.AuditEventsQuery{ActorUserID: input.ActorUserID, Action: input.Action, ResourceType: input.ResourceType, ResourceID: input.ResourceID, Outcome: input.Outcome, Limit: limit, Offset: input.Offset}
	events, err := s.repo.List(query)
	if err != nil {
		return nil, err
	}
	total, err := s.repo.Count(query)
	if err != nil {
		return nil, err
	}
	return &AuditEventsPage{Items: events, Limit: limit, Offset: input.Offset, Total: total}, nil
}

func (s *AuditService) VerifyHashChain(input VerifyAuditHashChainInput) (*AuditHashChainVerification, error) {
	if s == nil || s.repo == nil {
		return &AuditHashChainVerification{Valid: true}, nil
	}
	events, err := s.repo.ListHashChain(input.Limit)
	if err != nil {
		return nil, err
	}
	result := &AuditHashChainVerification{Valid: true, Checked: len(events)}
	previousHash := AuditGenesisHash
	previousSequence := int64(0)
	for _, event := range events {
		if event.HashAlgorithm != AuditHashAlgorithmSHA256 {
			result.addBreak(event, AuditChainBreakUnsupportedAlg, AuditHashAlgorithmSHA256, event.HashAlgorithm)
			continue
		}
		if event.Sequence != previousSequence+1 {
			result.addBreak(event, AuditChainBreakSequenceGap, fmt.Sprintf("%d", previousSequence+1), fmt.Sprintf("%d", event.Sequence))
		}
		if event.PreviousHash != previousHash {
			result.addBreak(event, AuditChainBreakLinkMismatch, previousHash, event.PreviousHash)
		}
		if event.EventHash == "" || event.PreviousHash == "" {
			result.addBreak(event, AuditChainBreakMissingHash, "non-empty hashes", "empty hash")
		} else {
			expectedHash, err := computeAuditEventHash(&event)
			if err != nil {
				return nil, err
			}
			if event.EventHash != expectedHash {
				result.addBreak(event, AuditChainBreakHashMismatch, expectedHash, event.EventHash)
			}
		}
		previousHash = event.EventHash
		previousSequence = event.Sequence
		result.HeadHash = event.EventHash
		result.HeadSequence = event.Sequence
	}
	return result, nil
}

func (r *AuditHashChainVerification) addBreak(event model.AuditEvent, reason, expected, actual string) {
	r.Valid = false
	r.Breaks = append(r.Breaks, AuditHashChainBreak{Sequence: event.Sequence, EventID: event.ID, Reason: reason, Expected: expected, Actual: actual})
}

func computeAuditEventHash(event *model.AuditEvent) (string, error) {
	actorUserID := ""
	if event.ActorUserID != nil {
		actorUserID = event.ActorUserID.String()
	}
	payload := map[string]any{
		"id":              event.ID.String(),
		"sequence":        event.Sequence,
		"previous_hash":   event.PreviousHash,
		"actor_user_id":   actorUserID,
		"actor_device_id": event.ActorDeviceID,
		"action":          event.Action,
		"resource_type":   event.ResourceType,
		"resource_id":     event.ResourceID,
		"outcome":         event.Outcome,
		"ip_address":      event.IPAddress,
		"user_agent":      event.UserAgent,
		"metadata":        event.Metadata,
		"occurred_at":     event.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
