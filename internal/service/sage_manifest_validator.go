package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	SAGERiskLow      = "low"
	SAGERiskMedium   = "medium"
	SAGERiskHigh     = "high"
	SAGERiskCritical = "critical"
)

type SAGEPluginManifest struct {
	SAGEVersion    string                   `json:"sage_version"`
	PluginKey      string                   `json:"plugin_key"`
	Name           string                   `json:"name"`
	Version        string                   `json:"version"`
	Description    string                   `json:"description"`
	Developer      SAGEManifestDeveloper    `json:"developer"`
	Endpoints      SAGEManifestEndpoints    `json:"endpoints"`
	TriggerIntents []string                 `json:"trigger_intents"`
	Permissions    []SAGEManifestPermission `json:"permissions"`
	Privacy        map[string]any           `json:"privacy"`
	Billing        map[string]any           `json:"billing"`
	Raw            map[string]any           `json:"-"`
}

type SAGEManifestDeveloper struct {
	Name         string `json:"name"`
	Website      string `json:"website"`
	SupportEmail string `json:"support_email"`
}

type SAGEManifestEndpoints struct {
	Manifest string `json:"manifest"`
	Flow     string `json:"flow"`
	Callback string `json:"callback"`
	Health   string `json:"health"`
}

type SAGEManifestPermission struct {
	Key                      string `json:"key"`
	Required                 bool   `json:"required"`
	Risk                     string `json:"risk"`
	RequiresUserConfirmation bool   `json:"requires_user_confirmation"`
	Reason                   string `json:"reason"`
}

type SAGEManifestValidationResult struct {
	Manifest          SAGEPluginManifest `json:"manifest"`
	Snapshot          map[string]any     `json:"snapshot"`
	ManifestHash      string             `json:"manifest_hash"`
	Valid             bool               `json:"valid"`
	Errors            []string           `json:"errors"`
	Warnings          []string           `json:"warnings"`
	RiskSummary       map[string]any     `json:"risk_summary"`
	PermissionSummary map[string]any     `json:"permission_summary"`
}

type SAGEManifestValidator struct{}

func NewSAGEManifestValidator() *SAGEManifestValidator { return &SAGEManifestValidator{} }

var sagePluginKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9][a-z0-9-]*)+$`)

var knownSAGEPermissions = map[string]string{
	"context.profile.read":            SAGERiskMedium,
	"context.memory.read":             SAGERiskHigh,
	"context.calendar.read":           SAGERiskMedium,
	"context.location.read":           SAGERiskHigh,
	"context.files.read":              SAGERiskHigh,
	"context.contacts.read":           SAGERiskHigh,
	"communication.message.draft":     SAGERiskMedium,
	"communication.message.send":      SAGERiskHigh,
	"communication.email.draft":       SAGERiskMedium,
	"communication.email.send":        SAGERiskHigh,
	"communication.social.publish":    SAGERiskHigh,
	"transaction.booking.create":      SAGERiskHigh,
	"transaction.order.create":        SAGERiskHigh,
	"transaction.payment.initiate":    SAGERiskCritical,
	"transaction.subscription.create": SAGERiskCritical,
	"local.file.write":                SAGERiskHigh,
	"local.browser.open":              SAGERiskMedium,
	"local.browser.operate":           SAGERiskHigh,
	"local.notification.schedule":     SAGERiskMedium,
	"local.task.schedule":             SAGERiskMedium,
	"plugin.api.call":                 SAGERiskLow,
	"plugin.callback.send":            SAGERiskLow,
	"third_party.api.call":            SAGERiskMedium,
}

func (v *SAGEManifestValidator) Validate(raw map[string]any) (*SAGEManifestValidationResult, error) {
	if raw == nil {
		raw = map[string]any{}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var manifest SAGEPluginManifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil, err
	}
	manifest.Raw = raw
	result := &SAGEManifestValidationResult{Manifest: manifest, Snapshot: normalizeJSONMap(raw), Errors: []string{}, Warnings: []string{}}
	result.ManifestHash = hashNormalizedJSON(result.Snapshot)

	v.validateRequired(&manifest, result)
	v.validateEndpoints(&manifest, result)
	v.validatePermissions(&manifest, result)
	result.RiskSummary, result.PermissionSummary = summarizeSAGEPermissions(manifest.Permissions)
	result.Valid = len(result.Errors) == 0
	return result, nil
}

func (v *SAGEManifestValidator) validateRequired(m *SAGEPluginManifest, r *SAGEManifestValidationResult) {
	if strings.TrimSpace(m.SAGEVersion) == "" {
		r.Errors = append(r.Errors, "sage_version is required")
	}
	if strings.TrimSpace(m.PluginKey) == "" {
		r.Errors = append(r.Errors, "plugin_key is required")
	} else if !sagePluginKeyPattern.MatchString(strings.TrimSpace(m.PluginKey)) {
		r.Errors = append(r.Errors, "plugin_key must be reverse-DNS lowercase text")
	}
	if strings.TrimSpace(m.Name) == "" {
		r.Errors = append(r.Errors, "name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		r.Errors = append(r.Errors, "version is required")
	}
	if strings.TrimSpace(m.Description) == "" {
		r.Errors = append(r.Errors, "description is required")
	}
	if strings.TrimSpace(m.Developer.Name) == "" {
		r.Errors = append(r.Errors, "developer.name is required")
	}
	if len(m.TriggerIntents) == 0 {
		r.Errors = append(r.Errors, "trigger_intents requires at least one item")
	}
}

func (v *SAGEManifestValidator) validateEndpoints(m *SAGEPluginManifest, r *SAGEManifestValidationResult) {
	if strings.TrimSpace(m.Endpoints.Flow) == "" {
		r.Errors = append(r.Errors, "endpoints.flow is required")
	}
	for name, value := range map[string]string{"manifest": m.Endpoints.Manifest, "flow": m.Endpoints.Flow, "callback": m.Endpoints.Callback, "health": m.Endpoints.Health, "developer.website": m.Developer.Website} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !isValidSAGEURL(value) {
			r.Errors = append(r.Errors, fmt.Sprintf("%s must be an absolute https URL", name))
		}
	}
}

func (v *SAGEManifestValidator) validatePermissions(m *SAGEPluginManifest, r *SAGEManifestValidationResult) {
	seen := map[string]bool{}
	for _, p := range m.Permissions {
		key := strings.TrimSpace(p.Key)
		if key == "" {
			r.Errors = append(r.Errors, "permission key is required")
			continue
		}
		if seen[key] {
			r.Errors = append(r.Errors, "duplicate permission: "+key)
		}
		seen[key] = true
		inferredRisk, ok := knownSAGEPermissions[key]
		if !ok {
			r.Warnings = append(r.Warnings, "unknown permission: "+key)
			inferredRisk = SAGERiskMedium
		}
		risk := strings.TrimSpace(p.Risk)
		if risk == "" {
			r.Warnings = append(r.Warnings, "permission "+key+" missing risk; inferred "+inferredRisk)
			risk = inferredRisk
		}
		if !validSAGERisk(risk) {
			r.Errors = append(r.Errors, "invalid risk for permission "+key)
			continue
		}
		if riskRank(risk) >= riskRank(SAGERiskHigh) && !p.RequiresUserConfirmation {
			r.Errors = append(r.Errors, "high-risk permission "+key+" requires user confirmation")
		}
	}
}

func isValidSAGEURL(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && (strings.HasPrefix(u.Host, "localhost") || strings.HasPrefix(u.Host, "127.0.0.1"))
}

func validSAGERisk(risk string) bool { return riskRank(risk) >= 0 }

func riskRank(risk string) int {
	switch strings.TrimSpace(strings.ToLower(risk)) {
	case SAGERiskLow:
		return 0
	case SAGERiskMedium:
		return 1
	case SAGERiskHigh:
		return 2
	case SAGERiskCritical:
		return 3
	default:
		return -1
	}
}

func summarizeSAGEPermissions(perms []SAGEManifestPermission) (map[string]any, map[string]any) {
	counts := map[string]int{SAGERiskLow: 0, SAGERiskMedium: 0, SAGERiskHigh: 0, SAGERiskCritical: 0}
	keys := make([]string, 0, len(perms))
	highRisk := []string{}
	highest := SAGERiskLow
	for _, p := range perms {
		key := strings.TrimSpace(p.Key)
		if key == "" {
			continue
		}
		risk := strings.TrimSpace(strings.ToLower(p.Risk))
		if risk == "" {
			risk = knownSAGEPermissions[key]
		}
		if !validSAGERisk(risk) {
			risk = SAGERiskMedium
		}
		counts[risk]++
		keys = append(keys, key)
		if riskRank(risk) >= riskRank(SAGERiskHigh) {
			highRisk = append(highRisk, key)
		}
		if riskRank(risk) > riskRank(highest) {
			highest = risk
		}
	}
	sort.Strings(keys)
	sort.Strings(highRisk)
	return map[string]any{"highest_risk": highest, "counts": counts, "high_risk_permissions": highRisk}, map[string]any{"permissions": keys, "total": len(keys)}
}

func normalizeJSONMap(raw map[string]any) map[string]any {
	b, _ := json.Marshal(raw)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func hashNormalizedJSON(raw map[string]any) string {
	b, _ := json.Marshal(raw)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
