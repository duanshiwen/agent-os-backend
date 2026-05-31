package service

import "testing"

func validSAGEManifestFixture() map[string]any {
	return map[string]any{
		"sage_version":    "1.0",
		"plugin_key":      "com.example.hotel-booking",
		"name":            "Hotel Booking Assistant",
		"version":         "1.0.0",
		"description":     "Search and book hotels through an AI agent.",
		"developer":       map[string]any{"name": "Example Travel Inc.", "website": "https://example.com", "support_email": "support@example.com"},
		"endpoints":       map[string]any{"manifest": "https://example.com/.well-known/sage-plugin.json", "flow": "https://example.com/sage/flow", "callback": "https://example.com/sage/callback", "health": "https://example.com/sage/health"},
		"trigger_intents": []any{"book hotel", "预订酒店"},
		"permissions":     []any{map[string]any{"key": "transaction.booking.create", "required": true, "risk": "high", "requires_user_confirmation": true, "reason": "Create bookings."}},
		"privacy":         map[string]any{"data_shared": []any{"destination"}, "data_retention": "30_days"},
		"billing":         map[string]any{"model": "free"},
	}
}

func TestSAGEManifestValidatorValidManifestPasses(t *testing.T) {
	result, err := NewSAGEManifestValidator().Validate(validSAGEManifestFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("expected valid manifest, errors=%v", result.Errors)
	}
	if result.ManifestHash == "" {
		t.Fatal("expected manifest hash")
	}
	if result.RiskSummary["highest_risk"] != "high" {
		t.Fatalf("expected high risk, got %#v", result.RiskSummary)
	}
}

func TestSAGEManifestValidatorMissingPluginKeyFails(t *testing.T) {
	manifest := validSAGEManifestFixture()
	delete(manifest, "plugin_key")
	result, err := NewSAGEManifestValidator().Validate(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("expected invalid manifest")
	}
}

func TestSAGEManifestValidatorInvalidEndpointFails(t *testing.T) {
	manifest := validSAGEManifestFixture()
	manifest["endpoints"].(map[string]any)["flow"] = "ftp://example.com/sage/flow"
	result, err := NewSAGEManifestValidator().Validate(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("expected invalid endpoint to fail")
	}
}

func TestSAGEManifestValidatorHighRiskRequiresConfirmation(t *testing.T) {
	manifest := validSAGEManifestFixture()
	manifest["permissions"] = []any{map[string]any{"key": "transaction.booking.create", "required": true, "risk": "high", "requires_user_confirmation": false}}
	result, err := NewSAGEManifestValidator().Validate(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("expected high-risk permission without confirmation to fail")
	}
}

func TestSAGEManifestValidatorHashStable(t *testing.T) {
	validator := NewSAGEManifestValidator()
	a, err := validator.Validate(validSAGEManifestFixture())
	if err != nil {
		t.Fatal(err)
	}
	b, err := validator.Validate(validSAGEManifestFixture())
	if err != nil {
		t.Fatal(err)
	}
	if a.ManifestHash != b.ManifestHash {
		t.Fatalf("expected stable hash: %s != %s", a.ManifestHash, b.ManifestHash)
	}
}
