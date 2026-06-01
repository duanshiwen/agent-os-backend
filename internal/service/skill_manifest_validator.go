package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type SkillManifestValidationResult struct {
	Valid        bool           `json:"valid"`
	Errors       []string       `json:"errors"`
	Warnings     []string       `json:"warnings"`
	ManifestHash string         `json:"manifest_hash"`
	Snapshot     map[string]any `json:"snapshot"`
	SkillKey     string         `json:"skill_key"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Summary      string         `json:"summary"`
	Description  string         `json:"description"`
	Category     string         `json:"category"`
	Tags         []string       `json:"tags"`
	HomepageURL  string         `json:"homepage_url"`
}

type SkillManifestValidator struct{}

func NewSkillManifestValidator() *SkillManifestValidator { return &SkillManifestValidator{} }

var skillKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,126}[a-z0-9]$|^[a-z0-9]$`)

func (v *SkillManifestValidator) Validate(expectedSkillKey string, manifest map[string]any) (*SkillManifestValidationResult, error) {
	if manifest == nil {
		manifest = map[string]any{}
	}
	snapshot := cloneMap(manifest)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	result := &SkillManifestValidationResult{Snapshot: snapshot, ManifestHash: hex.EncodeToString(sum[:])}

	result.SkillKey = firstString(snapshot, "id", "skill_key")
	if result.SkillKey == "" {
		result.SkillKey = strings.TrimSpace(expectedSkillKey)
	}
	result.Name = firstString(snapshot, "name")
	result.Version = firstString(snapshot, "version")
	result.Summary = firstString(snapshot, "summary")
	result.Description = firstString(snapshot, "description")
	if result.Summary == "" {
		result.Summary = result.Description
	}
	result.Category = firstString(snapshot, "category")
	result.HomepageURL = firstStringFromNested(snapshot, "metadata", "homepage")
	if result.HomepageURL == "" {
		result.HomepageURL = firstString(snapshot, "homepage", "homepage_url")
	}
	result.Tags = stringSliceFromAny(snapshot["tags"])
	if len(result.Tags) == 0 {
		result.Tags = stringSliceFromAny(nestedAny(snapshot, "metadata", "tags"))
	}

	if err := validateSkillKey(result.SkillKey); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	if expected := strings.TrimSpace(expectedSkillKey); expected != "" && result.SkillKey != "" && expected != result.SkillKey {
		result.Errors = append(result.Errors, fmt.Sprintf("manifest skill key %q does not match expected key %q", result.SkillKey, expected))
	}
	if strings.TrimSpace(result.Name) == "" {
		result.Errors = append(result.Errors, "name is required")
	}
	if strings.TrimSpace(result.Version) == "" {
		result.Errors = append(result.Errors, "version is required")
	}
	if !hasSkillInstructions(snapshot) {
		result.Errors = append(result.Errors, "at least one instruction, entrypoint, or instruction object is required")
	}
	if len(result.Tags) == 0 {
		result.Warnings = append(result.Warnings, "tags are recommended")
	}
	if strings.TrimSpace(result.Description) == "" && strings.TrimSpace(result.Summary) == "" {
		result.Warnings = append(result.Warnings, "description or summary is recommended")
	}
	result.Valid = len(result.Errors) == 0
	return result, nil
}

func validateSkillKey(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("skill key is required")
	}
	if len(value) > 128 {
		return fmt.Errorf("skill key must be at most 128 bytes")
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, ".") || strings.HasSuffix(value, "-") || strings.HasSuffix(value, ".") {
		return fmt.Errorf("skill key must not start or end with '-' or '.'")
	}
	if strings.Contains(value, "..") {
		return fmt.Errorf("skill key must not contain consecutive dots")
	}
	if !skillKeyPattern.MatchString(value) {
		return fmt.Errorf("skill key may only contain lowercase ASCII letters, numbers, '-', and '.'")
	}
	return nil
}

func hasSkillInstructions(manifest map[string]any) bool {
	if strings.TrimSpace(firstString(manifest, "entrypoint")) != "" || strings.TrimSpace(firstString(manifest, "instructions_object_id")) != "" {
		return true
	}
	instructions, ok := manifest["instructions"].([]any)
	if !ok || len(instructions) == 0 {
		return false
	}
	for _, item := range instructions {
		switch typed := item.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return true
			}
		case map[string]any:
			if strings.TrimSpace(firstString(typed, "content", "text")) != "" {
				return true
			}
		}
	}
	return false
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstStringFromNested(m map[string]any, parent, child string) string {
	if nested, ok := m[parent].(map[string]any); ok {
		return firstString(nested, child)
	}
	return ""
}

func nestedAny(m map[string]any, parent, child string) any {
	if nested, ok := m[parent].(map[string]any); ok {
		return nested[child]
	}
	return nil
}

func stringSliceFromAny(value any) []string {
	out := []string{}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

func cloneMap(input map[string]any) map[string]any {
	raw, _ := json.Marshal(input)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}
