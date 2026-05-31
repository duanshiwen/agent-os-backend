package service

import (
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
	if err := s.repo.Create(event); err != nil {
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
