package service

import (
	"net/url"
	"strings"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAdmissionServiceProtocolAutoAdmits(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "protocol"})

	allowed, reason, err := svc.CheckAdmission("pubkey-1", nil)
	if err != nil {
		t.Fatalf("check admission: %v", err)
	}
	if !allowed || reason != "auto_admitted" {
		t.Fatalf("expected protocol auto admission, got allowed=%v reason=%q", allowed, reason)
	}
}

func TestAdmissionServiceInvitationRequiresMatchingCode(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "invitation", InvitationCode: "secret-code"})

	allowed, reason, err := svc.CheckAdmission("pubkey-1", nil)
	if err != nil {
		t.Fatalf("check admission without code: %v", err)
	}
	if allowed || reason != "invalid_invitation_code" {
		t.Fatalf("expected invalid invitation without code, got allowed=%v reason=%q", allowed, reason)
	}

	wrong := "wrong-code"
	allowed, reason, err = svc.CheckAdmission("pubkey-1", &wrong)
	if err != nil {
		t.Fatalf("check admission with wrong code: %v", err)
	}
	if allowed || reason != "invalid_invitation_code" {
		t.Fatalf("expected invalid invitation with wrong code, got allowed=%v reason=%q", allowed, reason)
	}

	correct := "secret-code"
	allowed, reason, err = svc.CheckAdmission("pubkey-1", &correct)
	if err != nil {
		t.Fatalf("check admission with correct code: %v", err)
	}
	if !allowed || reason != "admitted_via_invitation" {
		t.Fatalf("expected invitation admission, got allowed=%v reason=%q", allowed, reason)
	}
}

func TestAdmissionServiceApprovalCreatesPendingRequest(t *testing.T) {
	svc, repo := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "approval"})

	allowed, reason, err := svc.CheckAdmission("pubkey-approval", nil)
	if err != nil {
		t.Fatalf("check admission: %v", err)
	}
	if allowed || reason != "pending_approval" {
		t.Fatalf("expected pending approval, got allowed=%v reason=%q", allowed, reason)
	}

	pending, err := repo.GetPendingAdmissionRequests()
	if err != nil {
		t.Fatalf("get pending: %v", err)
	}
	if len(pending) != 1 || pending[0].UserPubKey != "pubkey-approval" || pending[0].Status != "pending" {
		t.Fatalf("unexpected pending requests: %+v", pending)
	}
}

func TestAdmissionServiceApproveRequest(t *testing.T) {
	svc, repo := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "approval"})
	req := &model.AdmissionRequest{UserPubKey: "pubkey-approve", Status: "pending"}
	if err := repo.CreateAdmissionRequest(req); err != nil {
		t.Fatalf("create request: %v", err)
	}

	if err := svc.ApproveRequest(req.ID); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	updated, err := repo.GetAdmissionRequest(req.ID)
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if updated.Status != "approved" {
		t.Fatalf("expected approved, got %q", updated.Status)
	}

	if err := svc.ApproveRequest(req.ID); err == nil || !strings.Contains(err.Error(), "request already approved") {
		t.Fatalf("expected already approved error, got %v", err)
	}
}

func TestAdmissionServiceRejectRequest(t *testing.T) {
	svc, repo := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "approval"})
	req := &model.AdmissionRequest{UserPubKey: "pubkey-reject", Status: "pending"}
	if err := repo.CreateAdmissionRequest(req); err != nil {
		t.Fatalf("create request: %v", err)
	}

	if err := svc.RejectRequest(req.ID, "not eligible"); err != nil {
		t.Fatalf("reject request: %v", err)
	}
	updated, err := repo.GetAdmissionRequest(req.ID)
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if updated.Status != "rejected" || updated.Reason != "not eligible" {
		t.Fatalf("expected rejected with reason, got %+v", updated)
	}

	if err := svc.RejectRequest(req.ID, "again"); err == nil || !strings.Contains(err.Error(), "request already rejected") {
		t.Fatalf("expected already rejected error, got %v", err)
	}
}

func TestAdmissionServiceGetPendingRequestsOnlyReturnsPending(t *testing.T) {
	svc, repo := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "approval"})
	requests := []*model.AdmissionRequest{
		{UserPubKey: "pending-1", Status: "pending"},
		{UserPubKey: "approved-1", Status: "approved"},
		{UserPubKey: "pending-2", Status: "pending"},
	}
	for _, req := range requests {
		if err := repo.CreateAdmissionRequest(req); err != nil {
			t.Fatalf("create request: %v", err)
		}
	}

	pending, err := svc.GetPendingRequests()
	if err != nil {
		t.Fatalf("get pending requests: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending requests, got %+v", pending)
	}
	for _, req := range pending {
		if req.Status != "pending" {
			t.Fatalf("expected only pending requests, got %+v", pending)
		}
	}
}

func TestAdmissionServiceUnknownPolicyReturnsError(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "unknown"})

	allowed, reason, err := svc.CheckAdmission("pubkey-1", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown admission policy") {
		t.Fatalf("expected unknown policy error, got allowed=%v reason=%q err=%v", allowed, reason, err)
	}
	if allowed || reason != "" {
		t.Fatalf("expected denied empty reason on unknown policy, got allowed=%v reason=%q", allowed, reason)
	}
}

func TestAdmissionServiceGetPolicyCreatesDefaultPersistentPolicy(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "protocol"})

	policy, err := svc.GetPolicy("default")
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if policy.ServerID != "default" || policy.PolicyType != "protocol" {
		t.Fatalf("unexpected default policy: %+v", policy)
	}
	if policy.InvitationCodeHash != "" {
		t.Fatalf("default protocol policy should not have invitation hash")
	}
}

func TestAdmissionServiceUpdatePolicyPersistsValidPolicy(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "protocol"})
	adminID := uuid.New()

	policy, err := svc.UpdatePolicy("default", "approval", adminID)
	if err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if policy.PolicyType != "approval" || !policy.AdminApprovalRequired {
		t.Fatalf("expected approval policy with admin approval required, got %+v", policy)
	}
	if policy.UpdatedBy == nil || *policy.UpdatedBy != adminID {
		t.Fatalf("expected updated_by %s, got %+v", adminID, policy.UpdatedBy)
	}

	loaded, err := svc.GetPolicy("default")
	if err != nil {
		t.Fatalf("reload policy: %v", err)
	}
	if loaded.PolicyType != "approval" || !loaded.AdminApprovalRequired {
		t.Fatalf("policy was not persisted: %+v", loaded)
	}
}

func TestAdmissionServiceUpdatePolicyRejectsInvalidPolicy(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "protocol"})

	_, err := svc.UpdatePolicy("default", "invalid", uuid.New())
	if err == nil || !strings.Contains(err.Error(), "unsupported admission policy") {
		t.Fatalf("expected unsupported policy error, got %v", err)
	}
}

func TestAdmissionServiceUpdateInvitationCodeStoresHashOnly(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "protocol"})
	adminID := uuid.New()

	policy, err := svc.UpdateInvitationCode("default", "secret-code", adminID)
	if err != nil {
		t.Fatalf("update invitation code: %v", err)
	}
	if policy.InvitationCodeHash == "" {
		t.Fatal("expected invitation code hash")
	}
	if policy.InvitationCodeHash == "secret-code" || strings.Contains(policy.InvitationCodeHash, "secret-code") {
		t.Fatalf("invitation code stored in plaintext: %q", policy.InvitationCodeHash)
	}
	if policy.UpdatedBy == nil || *policy.UpdatedBy != adminID {
		t.Fatalf("expected updated_by %s, got %+v", adminID, policy.UpdatedBy)
	}

	loaded, err := svc.GetPolicy("default")
	if err != nil {
		t.Fatalf("reload policy: %v", err)
	}
	if loaded.InvitationCodeHash != policy.InvitationCodeHash {
		t.Fatalf("invitation hash was not persisted")
	}
}

func TestAdmissionServiceUpdateInvitationCodeRejectsEmptyCode(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "protocol"})

	_, err := svc.UpdateInvitationCode("default", "", uuid.New())
	if err == nil || !strings.Contains(err.Error(), "invitation code is required") {
		t.Fatalf("expected required invitation code error, got %v", err)
	}
}

func TestAdmissionServiceApproveUnknownRequestReturnsError(t *testing.T) {
	svc, _ := newAdmissionTestService(t, config.AdmissionConfig{PolicyType: "approval"})

	if err := svc.ApproveRequest(uuid.New()); err == nil || !strings.Contains(err.Error(), "request not found") {
		t.Fatalf("expected request not found error, got %v", err)
	}
}

func newAdmissionTestService(t *testing.T, cfg config.AdmissionConfig) (*AdmissionService, *repository.UserRepo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AdmissionRequest{}, &model.ServerAdmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewUserRepo(db)
	admissionRepo := repository.NewAdmissionRepo(db)
	return NewAdmissionServiceWithRepo(repo, admissionRepo, cfg), repo
}
