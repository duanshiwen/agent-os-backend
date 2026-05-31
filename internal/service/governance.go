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

func (s *GovernanceService) ConsumeApprovalReceipt(token, consumedBy string) (*model.ApprovalReceipt, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(token) == "" {
		return nil, ErrGovernanceInvalidInput
	}
	receipt, err := s.repo.ConsumeApprovalReceiptByTokenHash(hashGovernanceToken(token), strings.TrimSpace(consumedBy), time.Now().UTC())
	if err != nil {
		return nil, ErrApprovalReceiptInvalid
	}
	return receipt, nil
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
