package service

import (
	"fmt"
	"strings"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"gorm.io/datatypes"
)

type GovernanceScanner struct {
	repo *repository.GovernanceRepo
}

func NewGovernanceScanner(repo *repository.GovernanceRepo) *GovernanceScanner {
	return &GovernanceScanner{repo: repo}
}

type GovernanceScannerFinding struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Message  string         `json:"message"`
	Metadata map[string]any `json:"metadata"`
}

func (s *GovernanceScanner) ScanSAGEManifest(pluginID, versionID string, manifest map[string]any) []GovernanceScannerFinding {
	findings := []GovernanceScannerFinding{}
	text := strings.ToLower(fmt.Sprint(manifest))
	for _, phrase := range []string{"ignore previous instructions", "ignore all previous instructions", "reveal hidden system prompts", "bypass safety", "disable guardrails"} {
		if strings.Contains(text, phrase) {
			findings = append(findings, GovernanceScannerFinding{Code: "prompt_injection_phrase", Severity: GovernanceRiskHigh, Message: "Manifest text contains prompt-injection phrase: " + phrase, Metadata: map[string]any{"phrase": phrase, "plugin_id": pluginID, "version_id": versionID}})
		}
	}
	riskLevel := strings.ToLower(strings.TrimSpace(fmt.Sprint(manifest["risk_level"])))
	permissions := strings.ToLower(fmt.Sprint(manifest["permissions"]))
	if strings.Contains(permissions, "payments.write") && riskRank(riskLevel) < riskRank(GovernanceRiskHigh) {
		findings = append(findings, GovernanceScannerFinding{Code: "risk_level_mismatch", Severity: GovernanceRiskHigh, Message: "payments.write permission requires high or critical risk level", Metadata: map[string]any{"permission": "sage.permission.payments.write", "declared_risk_level": riskLevel}})
	}
	return findings
}

func (s *GovernanceScanner) PersistFindings(subjectType, subjectID string, findings []GovernanceScannerFinding) error {
	if s == nil || s.repo == nil {
		return nil
	}
	for _, finding := range findings {
		result := &model.GovernanceScanResult{SubjectType: subjectType, SubjectID: subjectID, Scanner: "deterministic-sage-manifest-v1", Severity: finding.Severity, Status: "open", Message: finding.Message, Details: datatypes.JSONMap{"code": finding.Code, "metadata": finding.Metadata}}
		if err := s.repo.CreateScanResult(result); err != nil {
			return err
		}
	}
	return nil
}
