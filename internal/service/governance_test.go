package service

import (
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newGovernanceTestService(t *testing.T) (*GovernanceService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.CapabilityDefinition{}, &model.PolicyRule{}, &model.PolicyDecision{}, &model.ApprovalReceipt{}, &model.KillSwitch{}, &model.GovernanceScanResult{}); err != nil {
		t.Fatalf("migrate governance models: %v", err)
	}
	return NewGovernanceService(repository.NewGovernanceRepo(db)), db
}

func TestGovernanceServiceEvaluatesPolicyRulesDeterministically(t *testing.T) {
	svc, _ := newGovernanceTestService(t)
	actorID := uuid.New()

	capability, err := svc.CreateCapability(CreateCapabilityInput{
		Key:         "sage.permission.payments.write",
		Name:        "Payment Write",
		Description: "Allows a SAGE plugin to initiate payment-affecting actions.",
		RiskLevel:   GovernanceRiskHigh,
		Status:      GovernanceStatusActive,
	})
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}

	if _, err := svc.CreatePolicyRule(CreatePolicyRuleInput{
		Name:          "High-risk payment grants require user approval",
		CapabilityKey: capability.Key,
		SubjectType:   GovernanceSubjectSAGEPermissionGrant,
		Effect:        GovernanceDecisionRequireUserApproval,
		Priority:      10,
		Status:        GovernanceStatusActive,
	}); err != nil {
		t.Fatalf("create policy rule: %v", err)
	}

	decision, err := svc.Evaluate(EvaluatePolicyInput{
		ActorUserID:   &actorID,
		ActorDeviceID: "device-a",
		SubjectType:   GovernanceSubjectSAGEPermissionGrant,
		SubjectID:     "grant-1",
		CapabilityKey: capability.Key,
		RiskLevel:     GovernanceRiskHigh,
		Context:       map[string]any{"permission_key": capability.Key},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Decision != GovernanceDecisionRequireUserApproval {
		t.Fatalf("expected require_user_approval decision, got %s", decision.Decision)
	}
	if decision.PolicyRuleID == nil || *decision.PolicyRuleID == uuid.Nil {
		t.Fatalf("expected decision to reference matched policy rule: %+v", decision)
	}
	if decision.ActorUserID == nil || *decision.ActorUserID != actorID {
		t.Fatalf("expected actor to be persisted, got %+v", decision.ActorUserID)
	}
	if decision.Context["permission_key"] != capability.Key {
		t.Fatalf("expected decision context to be persisted, got %+v", decision.Context)
	}
}

func TestGovernanceServiceKillSwitchOverridesAllowPolicy(t *testing.T) {
	svc, _ := newGovernanceTestService(t)

	if _, err := svc.CreateCapability(CreateCapabilityInput{Key: "sage.plugin.install", Name: "Install Plugin", RiskLevel: GovernanceRiskMedium, Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create capability: %v", err)
	}
	if _, err := svc.CreatePolicyRule(CreatePolicyRuleInput{Name: "Allow plugin install", CapabilityKey: "sage.plugin.install", SubjectType: GovernanceSubjectSAGEPlugin, Effect: GovernanceDecisionAllow, Priority: 100, Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create allow rule: %v", err)
	}
	if _, err := svc.CreateKillSwitch(CreateKillSwitchInput{ScopeType: GovernanceKillSwitchPlugin, ScopeID: "plugin-1", Reason: "malicious plugin", Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create kill switch: %v", err)
	}

	decision, err := svc.Evaluate(EvaluatePolicyInput{SubjectType: GovernanceSubjectSAGEPlugin, SubjectID: "plugin-1", CapabilityKey: "sage.plugin.install", RiskLevel: GovernanceRiskMedium})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Decision != GovernanceDecisionDeny {
		t.Fatalf("expected kill switch deny, got %s", decision.Decision)
	}
	if decision.KillSwitchID == nil || *decision.KillSwitchID == uuid.Nil {
		t.Fatalf("expected decision to reference kill switch: %+v", decision)
	}
}

func TestGovernanceServiceApprovalReceiptCanBeConsumedOnlyOnceBeforeExpiry(t *testing.T) {
	svc, _ := newGovernanceTestService(t)
	userID := uuid.New()
	decisionID := uuid.New()

	receipt, token, err := svc.CreateApprovalReceipt(CreateApprovalReceiptInput{
		PolicyDecisionID: decisionID,
		ActorUserID:      userID,
		SubjectType:      GovernanceSubjectSAGEPermissionGrant,
		SubjectID:        "grant-1",
		CapabilityKey:    "sage.permission.payments.write",
		Decision:         GovernanceDecisionRequireUserApproval,
		ExpiresAt:        time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create approval receipt: %v", err)
	}
	if token == "" {
		t.Fatalf("expected opaque approval token")
	}
	if receipt.Status != GovernanceApprovalPending {
		t.Fatalf("expected pending receipt, got %s", receipt.Status)
	}

	consumed, err := svc.ConsumeApprovalReceipt(token, "device-a")
	if err != nil {
		t.Fatalf("consume approval receipt: %v", err)
	}
	if consumed.Status != GovernanceApprovalConsumed || consumed.ConsumedAt == nil || consumed.ConsumedBy != "device-a" {
		t.Fatalf("expected consumed receipt with device marker, got %+v", consumed)
	}
	if _, err := svc.ConsumeApprovalReceipt(token, "device-a"); err == nil {
		t.Fatalf("expected one-time token reuse to fail")
	}

	_, expiredToken, err := svc.CreateApprovalReceipt(CreateApprovalReceiptInput{PolicyDecisionID: uuid.New(), ActorUserID: userID, SubjectType: GovernanceSubjectSAGEPermissionGrant, SubjectID: "grant-2", CapabilityKey: "sage.permission.payments.write", Decision: GovernanceDecisionRequireUserApproval, ExpiresAt: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatalf("create expired receipt: %v", err)
	}
	if _, err := svc.ConsumeApprovalReceipt(expiredToken, "device-a"); err == nil {
		t.Fatalf("expected expired token to fail")
	}
}
