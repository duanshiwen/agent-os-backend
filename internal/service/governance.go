package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

const (
	GovernanceStatusActive   = "active"
	GovernanceStatusInactive = "inactive"
)

const (
	GovernanceRiskLow      = "low"
	GovernanceRiskMedium   = "medium"
	GovernanceRiskHigh     = "high"
	GovernanceRiskCritical = "critical"
)

const (
	GovernanceDecisionAllow                = "allow"
	GovernanceDecisionRequireUserApproval  = "require_user_approval"
	GovernanceDecisionRequireAdminApproval = "require_admin_approval"
	GovernanceDecisionDeny                 = "deny"
)

const (
	GovernanceSubjectSAGEPlugin          = "sage_plugin"
	GovernanceSubjectSAGEPermissionGrant = "sage_permission_grant"
	GovernanceSubjectSAGEInvocation      = "sage_invocation"
	GovernanceSubjectObjectOperation     = "object_operation"
	GovernanceSubjectKBOperation         = "kb_operation"
	GovernanceSubjectAdminOperation      = "admin_operation"
)

const (
	GovernanceKillSwitchGlobal     = "global"
	GovernanceKillSwitchPlugin     = "plugin"
	GovernanceKillSwitchUser       = "user"
	GovernanceKillSwitchCapability = "capability"
	GovernanceKillSwitchServer     = "server"
)

const (
	GovernanceApprovalPending  = "pending"
	GovernanceApprovalConsumed = "consumed"
	GovernanceApprovalRevoked  = "revoked"
)

var ErrGovernanceInvalidInput = errors.New("invalid governance input")
var ErrApprovalReceiptInvalid = errors.New("approval receipt is invalid, expired, or already consumed")

type GovernanceService struct {
	repo *repository.GovernanceRepo
}

func NewGovernanceService(repo *repository.GovernanceRepo) *GovernanceService {
	return &GovernanceService{repo: repo}
}

type CreateCapabilityInput struct {
	Key         string
	Name        string
	Description string
	RiskLevel   string
	Status      string
	Metadata    map[string]any
}

func (s *GovernanceService) CreateCapability(input CreateCapabilityInput) (*model.CapabilityDefinition, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGovernanceInvalidInput
	}
	key := strings.TrimSpace(input.Key)
	name := strings.TrimSpace(input.Name)
	if key == "" || name == "" {
		return nil, ErrGovernanceInvalidInput
	}
	capability := &model.CapabilityDefinition{Key: key, Name: name, Description: strings.TrimSpace(input.Description), RiskLevel: normalizeOrDefault(input.RiskLevel, GovernanceRiskLow), Status: normalizeOrDefault(input.Status, GovernanceStatusActive), Metadata: toJSONMap(input.Metadata)}
	if err := s.repo.CreateCapability(capability); err != nil {
		return nil, err
	}
	return capability, nil
}

type GovernanceCapabilitiesPage struct {
	Items  []model.CapabilityDefinition `json:"items"`
	Limit  int                          `json:"limit"`
	Offset int                          `json:"offset"`
	Total  int64                        `json:"total"`
}

func (s *GovernanceService) ListCapabilities(limit, offset int) (*GovernanceCapabilitiesPage, error) {
	limit, offset = normalizePage(limit, offset)
	items, total, err := s.repo.ListCapabilities(limit, offset)
	if err != nil {
		return nil, err
	}
	return &GovernanceCapabilitiesPage{Items: items, Limit: limit, Offset: offset, Total: total}, nil
}

type CreatePolicyRuleInput struct {
	Name          string
	Description   string
	CapabilityKey string
	SubjectType   string
	SubjectID     string
	ActorUserID   *uuid.UUID
	RiskLevel     string
	Effect        string
	Priority      int
	Status        string
	Conditions    map[string]any
	Metadata      map[string]any
}

func (s *GovernanceService) CreatePolicyRule(input CreatePolicyRuleInput) (*model.PolicyRule, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGovernanceInvalidInput
	}
	name := strings.TrimSpace(input.Name)
	effect := normalizeDecision(input.Effect)
	if name == "" || effect == "" {
		return nil, ErrGovernanceInvalidInput
	}
	priority := input.Priority
	if priority == 0 {
		priority = 100
	}
	rule := &model.PolicyRule{Name: name, Description: strings.TrimSpace(input.Description), CapabilityKey: strings.TrimSpace(input.CapabilityKey), SubjectType: strings.TrimSpace(input.SubjectType), SubjectID: strings.TrimSpace(input.SubjectID), ActorUserID: input.ActorUserID, RiskLevel: strings.TrimSpace(input.RiskLevel), Effect: effect, Priority: priority, Status: normalizeOrDefault(input.Status, GovernanceStatusActive), Conditions: toJSONMap(input.Conditions), Metadata: toJSONMap(input.Metadata)}
	if err := s.repo.CreatePolicyRule(rule); err != nil {
		return nil, err
	}
	return rule, nil
}

type GovernancePolicyRulesPage struct {
	Items  []model.PolicyRule `json:"items"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
	Total  int64              `json:"total"`
}

func (s *GovernanceService) ListPolicyRules(limit, offset int) (*GovernancePolicyRulesPage, error) {
	limit, offset = normalizePage(limit, offset)
	items, total, err := s.repo.ListPolicyRules(limit, offset)
	if err != nil {
		return nil, err
	}
	return &GovernancePolicyRulesPage{Items: items, Limit: limit, Offset: offset, Total: total}, nil
}

type EvaluatePolicyInput struct {
	ActorUserID   *uuid.UUID
	ActorDeviceID string
	SubjectType   string
	SubjectID     string
	CapabilityKey string
	RiskLevel     string
	Context       map[string]any
}

func (s *GovernanceService) Evaluate(input EvaluatePolicyInput) (*model.PolicyDecision, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGovernanceInvalidInput
	}
	subjectType := strings.TrimSpace(input.SubjectType)
	capabilityKey := strings.TrimSpace(input.CapabilityKey)
	if subjectType == "" || capabilityKey == "" {
		return nil, ErrGovernanceInvalidInput
	}
	riskLevel := normalizeOrDefault(input.RiskLevel, GovernanceRiskLow)
	now := time.Now().UTC()

	var killSwitchID *uuid.UUID
	if killSwitch, err := s.repo.FindActiveKillSwitch(repository.KillSwitchQuery{ScopeType: subjectType, ScopeID: strings.TrimSpace(input.SubjectID), CapabilityKey: capabilityKey, ActorUserID: input.ActorUserID, Now: now}); err == nil && killSwitch != nil {
		killSwitchID = &killSwitch.ID
		return s.persistDecision(input, riskLevel, GovernanceDecisionDeny, fmt.Sprintf("blocked by %s kill switch", killSwitch.ScopeType), nil, killSwitchID, now)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	rules, err := s.repo.FindMatchingPolicyRules(repository.PolicyRuleMatchQuery{CapabilityKey: capabilityKey, SubjectType: subjectType, SubjectID: strings.TrimSpace(input.SubjectID), ActorUserID: input.ActorUserID, RiskLevel: riskLevel})
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return s.persistDecision(input, riskLevel, GovernanceDecisionAllow, "default allow: no matching active policy rule", nil, nil, now)
	}
	rule := rules[0]
	ruleID := rule.ID
	return s.persistDecision(input, riskLevel, rule.Effect, "matched policy rule: "+rule.Name, &ruleID, nil, now)
}

func (s *GovernanceService) persistDecision(input EvaluatePolicyInput, riskLevel, decision, reason string, ruleID, killSwitchID *uuid.UUID, now time.Time) (*model.PolicyDecision, error) {
	policyDecision := &model.PolicyDecision{ActorUserID: input.ActorUserID, ActorDeviceID: strings.TrimSpace(input.ActorDeviceID), SubjectType: strings.TrimSpace(input.SubjectType), SubjectID: strings.TrimSpace(input.SubjectID), CapabilityKey: strings.TrimSpace(input.CapabilityKey), RiskLevel: riskLevel, Decision: decision, Reason: reason, PolicyRuleID: ruleID, KillSwitchID: killSwitchID, Context: toJSONMap(input.Context), DecidedAt: now}
	if err := s.repo.CreatePolicyDecision(policyDecision); err != nil {
		return nil, err
	}
	return policyDecision, nil
}

type ListPolicyDecisionsInput struct {
	SubjectType   string
	SubjectID     string
	CapabilityKey string
	Decision      string
	RiskLevel     string
}

type GovernancePolicyDecisionsPage struct {
	Items  []model.PolicyDecision `json:"items"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
	Total  int64                  `json:"total"`
}

func (s *GovernanceService) ListPolicyDecisions(input ListPolicyDecisionsInput, limit, offset int) (*GovernancePolicyDecisionsPage, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGovernanceInvalidInput
	}
	limit, offset = normalizePage(limit, offset)
	items, total, err := s.repo.ListPolicyDecisions(repository.PolicyDecisionListFilter{SubjectType: strings.TrimSpace(input.SubjectType), SubjectID: strings.TrimSpace(input.SubjectID), CapabilityKey: strings.TrimSpace(input.CapabilityKey), Decision: strings.TrimSpace(input.Decision), RiskLevel: strings.TrimSpace(input.RiskLevel)}, limit, offset)
	if err != nil {
		return nil, err
	}
	return &GovernancePolicyDecisionsPage{Items: items, Limit: limit, Offset: offset, Total: total}, nil
}

func (s *GovernanceService) GetPolicyDecision(id uuid.UUID) (*model.PolicyDecision, error) {
	if s == nil || s.repo == nil || id == uuid.Nil {
		return nil, ErrGovernanceInvalidInput
	}
	return s.repo.GetPolicyDecision(id)
}

type CreateKillSwitchInput struct {
	ScopeType string
	ScopeID   string
	Reason    string
	Status    string
	ExpiresAt *time.Time
	CreatedBy *uuid.UUID
	Metadata  map[string]any
}

func (s *GovernanceService) CreateKillSwitch(input CreateKillSwitchInput) (*model.KillSwitch, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGovernanceInvalidInput
	}
	scopeType := strings.TrimSpace(input.ScopeType)
	if scopeType == "" {
		return nil, ErrGovernanceInvalidInput
	}
	killSwitch := &model.KillSwitch{ScopeType: scopeType, ScopeID: strings.TrimSpace(input.ScopeID), Reason: strings.TrimSpace(input.Reason), Status: normalizeOrDefault(input.Status, GovernanceStatusActive), ExpiresAt: input.ExpiresAt, CreatedBy: input.CreatedBy, Metadata: toJSONMap(input.Metadata)}
	if err := s.repo.CreateKillSwitch(killSwitch); err != nil {
		return nil, err
	}
	return killSwitch, nil
}

type CreateApprovalReceiptInput struct {
	PolicyDecisionID uuid.UUID
	ActorUserID      uuid.UUID
	SubjectType      string
	SubjectID        string
	CapabilityKey    string
	Decision         string
	ExpiresAt        time.Time
	Metadata         map[string]any
}

func (s *GovernanceService) CreateApprovalReceipt(input CreateApprovalReceiptInput) (*model.ApprovalReceipt, string, error) {
	if s == nil || s.repo == nil || input.PolicyDecisionID == uuid.Nil || input.ActorUserID == uuid.Nil || strings.TrimSpace(input.SubjectType) == "" || strings.TrimSpace(input.CapabilityKey) == "" {
		return nil, "", ErrGovernanceInvalidInput
	}
	if input.ExpiresAt.IsZero() {
		input.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	}
	token, tokenHash, err := newGovernanceToken()
	if err != nil {
		return nil, "", err
	}
	receipt := &model.ApprovalReceipt{PolicyDecisionID: input.PolicyDecisionID, ActorUserID: input.ActorUserID, SubjectType: strings.TrimSpace(input.SubjectType), SubjectID: strings.TrimSpace(input.SubjectID), CapabilityKey: strings.TrimSpace(input.CapabilityKey), Decision: normalizeDecision(input.Decision), TokenHash: tokenHash, Status: GovernanceApprovalPending, ExpiresAt: input.ExpiresAt.UTC(), Metadata: toJSONMap(input.Metadata)}
	if err := s.repo.CreateApprovalReceipt(receipt); err != nil {
		return nil, "", err
	}
	return receipt, token, nil
}

type ConsumeApprovalReceiptInput struct {
	Token         string
	ConsumedBy    string
	ActorUserID   *uuid.UUID
	SubjectType   string
	SubjectID     string
	CapabilityKey string
}

func (s *GovernanceService) ConsumeApprovalReceipt(token, consumedBy string) (*model.ApprovalReceipt, error) {
	return s.ConsumeApprovalReceiptForOperation(ConsumeApprovalReceiptInput{Token: token, ConsumedBy: consumedBy})
}

func (s *GovernanceService) ConsumeApprovalReceiptForOperation(input ConsumeApprovalReceiptInput) (*model.ApprovalReceipt, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(input.Token) == "" {
		return nil, ErrGovernanceInvalidInput
	}
	receipt, err := s.repo.ConsumeApprovalReceiptByTokenHash(hashGovernanceToken(input.Token), strings.TrimSpace(input.ConsumedBy), time.Now().UTC(), repository.ApprovalReceiptConsumeConstraints{
		ActorUserID:   input.ActorUserID,
		SubjectType:   strings.TrimSpace(input.SubjectType),
		SubjectID:     strings.TrimSpace(input.SubjectID),
		CapabilityKey: strings.TrimSpace(input.CapabilityKey),
	})
	if err != nil {
		return nil, ErrApprovalReceiptInvalid
	}
	return receipt, nil
}

type ListApprovalReceiptsInput struct {
	SubjectType   string
	SubjectID     string
	CapabilityKey string
	Status        string
}

type GovernanceApprovalReceiptsPage struct {
	Items  []model.ApprovalReceipt `json:"items"`
	Limit  int                     `json:"limit"`
	Offset int                     `json:"offset"`
	Total  int64                   `json:"total"`
}

func (s *GovernanceService) ListApprovalReceipts(input ListApprovalReceiptsInput, limit, offset int) (*GovernanceApprovalReceiptsPage, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGovernanceInvalidInput
	}
	limit, offset = normalizePage(limit, offset)
	items, total, err := s.repo.ListApprovalReceipts(repository.ApprovalReceiptListFilter{SubjectType: strings.TrimSpace(input.SubjectType), SubjectID: strings.TrimSpace(input.SubjectID), CapabilityKey: strings.TrimSpace(input.CapabilityKey), Status: strings.TrimSpace(input.Status)}, limit, offset)
	if err != nil {
		return nil, err
	}
	return &GovernanceApprovalReceiptsPage{Items: items, Limit: limit, Offset: offset, Total: total}, nil
}

func (s *GovernanceService) RevokeApprovalReceipt(id uuid.UUID, reason string) (*model.ApprovalReceipt, error) {
	if s == nil || s.repo == nil || id == uuid.Nil {
		return nil, ErrGovernanceInvalidInput
	}
	receipt, err := s.repo.RevokeApprovalReceipt(id, strings.TrimSpace(reason), time.Now().UTC())
	if err != nil {
		return nil, ErrApprovalReceiptInvalid
	}
	return receipt, nil
}

type ListScanResultsInput struct {
	SubjectType string
	SubjectID   string
	Scanner     string
	Severity    string
	Status      string
}

type GovernanceScanResultsPage struct {
	Items  []model.GovernanceScanResult `json:"items"`
	Limit  int                          `json:"limit"`
	Offset int                          `json:"offset"`
	Total  int64                        `json:"total"`
}

func (s *GovernanceService) ListScanResults(input ListScanResultsInput, limit, offset int) (*GovernanceScanResultsPage, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGovernanceInvalidInput
	}
	limit, offset = normalizePage(limit, offset)
	items, total, err := s.repo.ListScanResults(repository.ScanResultListFilter{SubjectType: strings.TrimSpace(input.SubjectType), SubjectID: strings.TrimSpace(input.SubjectID), Scanner: strings.TrimSpace(input.Scanner), Severity: strings.TrimSpace(input.Severity), Status: strings.TrimSpace(input.Status)}, limit, offset)
	if err != nil {
		return nil, err
	}
	return &GovernanceScanResultsPage{Items: items, Limit: limit, Offset: offset, Total: total}, nil
}

func (s *GovernanceService) ResolveScanResult(id uuid.UUID, resolvedBy uuid.UUID) (*model.GovernanceScanResult, error) {
	if s == nil || s.repo == nil || id == uuid.Nil {
		return nil, ErrGovernanceInvalidInput
	}
	return s.repo.ResolveScanResult(id, resolvedBy, time.Now().UTC())
}

type GovernanceSummary struct {
	WindowHours               int                             `json:"window_hours"`
	Since                     time.Time                       `json:"since"`
	PolicyDecisionTotal       int64                           `json:"policy_decision_total"`
	PolicyDecisionsByDecision []repository.GovernanceCountRow `json:"policy_decisions_by_decision"`
	PolicyDecisionsByRisk     []repository.GovernanceCountRow `json:"policy_decisions_by_risk"`
	ApprovalReceiptTotal      int64                           `json:"approval_receipt_total"`
	ApprovalReceiptsByStatus  []repository.GovernanceCountRow `json:"approval_receipts_by_status"`
	OpenScanResultTotal       int64                           `json:"open_scan_result_total"`
	OpenScanResultsBySeverity []repository.GovernanceCountRow `json:"open_scan_results_by_severity"`
}

func (s *GovernanceService) Summary(windowHours int) (*GovernanceSummary, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGovernanceInvalidInput
	}
	if windowHours <= 0 || windowHours > 24*30 {
		windowHours = 24
	}
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	decisionTotal, decisionsByDecision, decisionsByRisk, err := s.repo.CountPolicyDecisionsSince(since)
	if err != nil {
		return nil, err
	}
	receiptTotal, receiptsByStatus, err := s.repo.CountApprovalReceiptsSince(since)
	if err != nil {
		return nil, err
	}
	openScanTotal, openScansBySeverity, err := s.repo.CountOpenScanResults()
	if err != nil {
		return nil, err
	}
	return &GovernanceSummary{WindowHours: windowHours, Since: since, PolicyDecisionTotal: decisionTotal, PolicyDecisionsByDecision: decisionsByDecision, PolicyDecisionsByRisk: decisionsByRisk, ApprovalReceiptTotal: receiptTotal, ApprovalReceiptsByStatus: receiptsByStatus, OpenScanResultTotal: openScanTotal, OpenScanResultsBySeverity: openScansBySeverity}, nil
}

func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func normalizeOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeDecision(value string) string {
	switch strings.TrimSpace(value) {
	case GovernanceDecisionAllow, GovernanceDecisionRequireUserApproval, GovernanceDecisionRequireAdminApproval, GovernanceDecisionDeny:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func toJSONMap(input map[string]any) datatypes.JSONMap {
	if input == nil {
		return datatypes.JSONMap{}
	}
	return datatypes.JSONMap(input)
}

func newGovernanceToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(buf)
	return token, hashGovernanceToken(token), nil
}

func hashGovernanceToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
