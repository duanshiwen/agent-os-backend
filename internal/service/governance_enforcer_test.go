package service

import (
	"errors"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
)

func TestGovernanceEnforcerDefaultsToObserveAndDoesNotBlockDenyDecision(t *testing.T) {
	governance, _ := newGovernanceTestService(t)
	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{
		Name:          "Deny SAGE invocation",
		CapabilityKey: "sage.invocation.create",
		SubjectType:   GovernanceSubjectSAGEInvocation,
		Effect:        GovernanceDecisionDeny,
		Priority:      1,
		Status:        GovernanceStatusActive,
	}); err != nil {
		t.Fatalf("create policy rule: %v", err)
	}
	enforcer := NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{})
	actorID := uuid.New()

	result, err := enforcer.Enforce(GovernanceEnforcementInput{
		ActorUserID:   &actorID,
		ActorDeviceID: "device-a",
		SubjectType:   GovernanceSubjectSAGEInvocation,
		SubjectID:     "plugin-1",
		CapabilityKey: "sage.invocation.create",
		RiskLevel:     GovernanceRiskHigh,
	})
	if err != nil {
		t.Fatalf("observe mode should not return denial error: %v", err)
	}
	if result == nil || !result.Allowed {
		t.Fatalf("observe mode should allow business execution, got %+v", result)
	}
	if result.Mode != GovernanceEnforcementModeObserve {
		t.Fatalf("expected observe mode, got %q", result.Mode)
	}
	if !result.WouldHaveBlocked {
		t.Fatalf("expected would_have_blocked=true for deny decision in observe mode, got %+v", result)
	}
	if result.Decision == nil || result.Decision.Decision != GovernanceDecisionDeny {
		t.Fatalf("expected persisted deny decision, got %+v", result.Decision)
	}
}

func TestGovernanceEnforcerBlocksDenyDecisionInEnforceMode(t *testing.T) {
	governance, _ := newGovernanceTestService(t)
	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{
		Name:          "Deny object upload",
		CapabilityKey: "object.upload.sage_plugin_package",
		SubjectType:   GovernanceSubjectObjectOperation,
		Effect:        GovernanceDecisionDeny,
		Priority:      1,
		Status:        GovernanceStatusActive,
	}); err != nil {
		t.Fatalf("create policy rule: %v", err)
	}
	enforcer := NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{Mode: GovernanceEnforcementModeEnforce})
	actorID := uuid.New()

	result, err := enforcer.Enforce(GovernanceEnforcementInput{
		ActorUserID:   &actorID,
		SubjectType:   GovernanceSubjectObjectOperation,
		SubjectID:     "sage-plugin-package",
		CapabilityKey: "object.upload.sage_plugin_package",
		RiskLevel:     GovernanceRiskMedium,
	})
	if !errors.Is(err, ErrGovernanceDenied) {
		t.Fatalf("expected ErrGovernanceDenied, got result=%+v err=%v", result, err)
	}
	if result == nil || result.Allowed || !result.WouldHaveBlocked {
		t.Fatalf("expected blocked result, got %+v", result)
	}
}

func TestGovernanceEnforcerRequiresApprovalOnlyInEnforceMode(t *testing.T) {
	governance, _ := newGovernanceTestService(t)
	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{
		Name:          "High-risk SAGE grant requires approval",
		CapabilityKey: "sage.permission.payments.write",
		SubjectType:   GovernanceSubjectSAGEPermissionGrant,
		Effect:        GovernanceDecisionRequireUserApproval,
		Priority:      1,
		Status:        GovernanceStatusActive,
	}); err != nil {
		t.Fatalf("create policy rule: %v", err)
	}
	enforcer := NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{SAGEMode: GovernanceEnforcementModeEnforce})
	actorID := uuid.New()

	result, err := enforcer.Enforce(GovernanceEnforcementInput{
		ActorUserID:   &actorID,
		ActorDeviceID: "device-a",
		SubjectType:   GovernanceSubjectSAGEPermissionGrant,
		SubjectID:     "grant-1",
		CapabilityKey: "sage.permission.payments.write",
		RiskLevel:     GovernanceRiskHigh,
	})
	if !errors.Is(err, ErrGovernanceApprovalRequired) {
		t.Fatalf("expected approval required, got result=%+v err=%v", result, err)
	}
	if result == nil || result.ApprovalReceipt == nil || result.ApprovalToken == "" {
		t.Fatalf("expected approval receipt/token in enforce mode, got %+v", result)
	}

	approved, err := enforcer.Enforce(GovernanceEnforcementInput{
		ActorUserID:        &actorID,
		ActorDeviceID:      "device-a",
		SubjectType:        GovernanceSubjectSAGEPermissionGrant,
		SubjectID:          "grant-1",
		CapabilityKey:      "sage.permission.payments.write",
		RiskLevel:          GovernanceRiskHigh,
		ApprovalToken:      result.ApprovalToken,
		ApprovalConsumedBy: "device-a",
	})
	if err != nil {
		t.Fatalf("approval token should allow once: %v", err)
	}
	if approved == nil || !approved.Allowed || approved.ConsumedApprovalReceipt == nil {
		t.Fatalf("expected consumed approval result, got %+v", approved)
	}
}

func TestGovernanceEnforcerRejectsApprovalTokenForDifferentCapability(t *testing.T) {
	governance, _ := newGovernanceTestService(t)
	enforcer := NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{Mode: GovernanceEnforcementModeEnforce})
	actorID := uuid.New()

	decision := mustCreateApprovalDecision(t, governance, actorID, GovernanceSubjectSAGEPermissionGrant, "grant-1", "sage.permission.payments.write")
	_, token, err := governance.CreateApprovalReceipt(CreateApprovalReceiptInput{
		PolicyDecisionID: decision.ID,
		ActorUserID:      actorID,
		SubjectType:      GovernanceSubjectSAGEPermissionGrant,
		SubjectID:        "grant-1",
		CapabilityKey:    "sage.permission.payments.write",
		Decision:         GovernanceDecisionRequireUserApproval,
	})
	if err != nil {
		t.Fatalf("create approval receipt: %v", err)
	}

	result, err := enforcer.Enforce(GovernanceEnforcementInput{
		ActorUserID:        &actorID,
		ActorDeviceID:      "device-a",
		SubjectType:        GovernanceSubjectSAGEPermissionGrant,
		SubjectID:          "grant-1",
		CapabilityKey:      "sage.permission.files.write",
		RiskLevel:          GovernanceRiskHigh,
		ApprovalToken:      token,
		ApprovalConsumedBy: "device-a",
	})
	if !errors.Is(err, ErrGovernanceApprovalInvalid) {
		t.Fatalf("expected mismatched capability to fail with ErrGovernanceApprovalInvalid, got result=%+v err=%v", result, err)
	}
	if result != nil && result.Allowed {
		t.Fatalf("mismatched approval token must not allow execution, got %+v", result)
	}
}

func TestGovernanceEnforcerRejectsApprovalTokenForDifferentSubject(t *testing.T) {
	governance, _ := newGovernanceTestService(t)
	enforcer := NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{Mode: GovernanceEnforcementModeEnforce})
	actorID := uuid.New()

	decision := mustCreateApprovalDecision(t, governance, actorID, GovernanceSubjectKBOperation, "collection-a", "kb.collection.pricing.update")
	_, token, err := governance.CreateApprovalReceipt(CreateApprovalReceiptInput{
		PolicyDecisionID: decision.ID,
		ActorUserID:      actorID,
		SubjectType:      GovernanceSubjectKBOperation,
		SubjectID:        "collection-a",
		CapabilityKey:    "kb.collection.pricing.update",
		Decision:         GovernanceDecisionRequireUserApproval,
	})
	if err != nil {
		t.Fatalf("create approval receipt: %v", err)
	}

	result, err := enforcer.Enforce(GovernanceEnforcementInput{
		ActorUserID:        &actorID,
		ActorDeviceID:      "device-a",
		SubjectType:        GovernanceSubjectKBOperation,
		SubjectID:          "collection-b",
		CapabilityKey:      "kb.collection.pricing.update",
		RiskLevel:          GovernanceRiskHigh,
		ApprovalToken:      token,
		ApprovalConsumedBy: "device-a",
	})
	if !errors.Is(err, ErrGovernanceApprovalInvalid) {
		t.Fatalf("expected mismatched subject to fail with ErrGovernanceApprovalInvalid, got result=%+v err=%v", result, err)
	}
	if result != nil && result.Allowed {
		t.Fatalf("mismatched approval token must not allow execution, got %+v", result)
	}
}

func mustCreateApprovalDecision(t *testing.T, governance *GovernanceService, actorID uuid.UUID, subjectType, subjectID, capabilityKey string) *model.PolicyDecision {
	t.Helper()
	decision, err := governance.persistDecision(EvaluatePolicyInput{
		ActorUserID:   &actorID,
		ActorDeviceID: "device-a",
		SubjectType:   subjectType,
		SubjectID:     subjectID,
		CapabilityKey: capabilityKey,
		RiskLevel:     GovernanceRiskHigh,
	}, GovernanceRiskHigh, GovernanceDecisionRequireUserApproval, "test approval required", nil, nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("create approval decision: %v", err)
	}
	return decision
}

func TestGovernanceEnforcerDisabledModeSkipsDecisionPersistence(t *testing.T) {
	governance, _ := newGovernanceTestService(t)
	enforcer := NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{Mode: GovernanceEnforcementModeDisabled})
	actorID := uuid.New()

	result, err := enforcer.Enforce(GovernanceEnforcementInput{
		ActorUserID:   &actorID,
		SubjectType:   GovernanceSubjectKBOperation,
		SubjectID:     "collection-1",
		CapabilityKey: "kb.collection.pricing.update",
		RiskLevel:     GovernanceRiskHigh,
	})
	if err != nil {
		t.Fatalf("disabled mode should allow without error: %v", err)
	}
	if result == nil || !result.Allowed || result.Mode != GovernanceEnforcementModeDisabled || result.Decision != nil {
		t.Fatalf("expected disabled allow without decision, got %+v", result)
	}
}
