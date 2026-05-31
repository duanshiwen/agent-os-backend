package config

import "testing"

func TestLoadGovernanceEnforcementDefaultsToObserve(t *testing.T) {
	t.Setenv("GOVERNANCE_ENFORCEMENT_MODE", "")
	t.Setenv("GOVERNANCE_ENFORCEMENT_SAGE_MODE", "")
	t.Setenv("GOVERNANCE_ENFORCEMENT_OBJECT_MODE", "")
	t.Setenv("GOVERNANCE_ENFORCEMENT_KB_MODE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Governance.EnforcementMode != "observe" {
		t.Fatalf("expected default governance enforcement mode observe, got %q", cfg.Governance.EnforcementMode)
	}
	if cfg.Governance.SAGEEnforcementMode != "" || cfg.Governance.ObjectEnforcementMode != "" || cfg.Governance.KBEnforcementMode != "" {
		t.Fatalf("expected domain modes to default empty/inherit, got %+v", cfg.Governance)
	}
}

func TestLoadGovernanceEnforcementModesFromEnv(t *testing.T) {
	t.Setenv("GOVERNANCE_ENFORCEMENT_MODE", "observe")
	t.Setenv("GOVERNANCE_ENFORCEMENT_SAGE_MODE", "enforce")
	t.Setenv("GOVERNANCE_ENFORCEMENT_OBJECT_MODE", "disabled")
	t.Setenv("GOVERNANCE_ENFORCEMENT_KB_MODE", "observe")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Governance.EnforcementMode != "observe" || cfg.Governance.SAGEEnforcementMode != "enforce" || cfg.Governance.ObjectEnforcementMode != "disabled" || cfg.Governance.KBEnforcementMode != "observe" {
		t.Fatalf("unexpected governance config: %+v", cfg.Governance)
	}
}

func TestValidateRejectsInvalidGovernanceEnforcementMode(t *testing.T) {
	t.Setenv("GOVERNANCE_ENFORCEMENT_MODE", "banana")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid governance enforcement mode to fail validation")
	}
}
