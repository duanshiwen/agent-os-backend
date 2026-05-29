package service

import (
	"fmt"
	"strings"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AdmissionService struct {
	userRepo      *repository.UserRepo
	admissionRepo *repository.AdmissionRepo
	cfg           config.AdmissionConfig
}

func NewAdmissionService(userRepo *repository.UserRepo, cfg config.AdmissionConfig) *AdmissionService {
	return &AdmissionService{userRepo: userRepo, cfg: cfg}
}

func NewAdmissionServiceWithRepo(userRepo *repository.UserRepo, admissionRepo *repository.AdmissionRepo, cfg config.AdmissionConfig) *AdmissionService {
	return &AdmissionService{userRepo: userRepo, admissionRepo: admissionRepo, cfg: cfg}
}

func (s *AdmissionService) GetPolicy(serverID string) (*model.ServerAdmission, error) {
	if strings.TrimSpace(serverID) == "" {
		serverID = "default"
	}
	if s.admissionRepo == nil {
		return &model.ServerAdmission{ServerID: serverID, PolicyType: s.cfg.PolicyType, AdminApprovalRequired: s.cfg.PolicyType == "approval"}, nil
	}

	policy, err := s.admissionRepo.GetServerAdmission(serverID)
	if err == nil {
		return policy, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	policy = &model.ServerAdmission{
		ServerID:              serverID,
		PolicyType:            s.cfg.PolicyType,
		AdminApprovalRequired: s.cfg.PolicyType == "approval",
	}
	if err := s.admissionRepo.CreateServerAdmission(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func (s *AdmissionService) UpdatePolicy(serverID, policyType string, updatedBy uuid.UUID) (*model.ServerAdmission, error) {
	policyType = strings.TrimSpace(policyType)
	if !isSupportedAdmissionPolicy(policyType) {
		return nil, fmt.Errorf("unsupported admission policy: %s", policyType)
	}

	policy, err := s.GetPolicy(serverID)
	if err != nil {
		return nil, err
	}
	policy.PolicyType = policyType
	policy.AdminApprovalRequired = policyType == "approval"
	policy.UpdatedBy = &updatedBy

	if s.admissionRepo == nil {
		return policy, nil
	}
	if err := s.admissionRepo.SaveServerAdmission(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func (s *AdmissionService) UpdateInvitationCode(serverID, code string, updatedBy uuid.UUID) (*model.ServerAdmission, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("invitation code is required")
	}
	policy, err := s.GetPolicy(serverID)
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash invitation code: %w", err)
	}
	policy.InvitationCodeHash = string(hash)
	policy.UpdatedBy = &updatedBy

	if s.admissionRepo == nil {
		return policy, nil
	}
	if err := s.admissionRepo.SaveServerAdmission(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func isSupportedAdmissionPolicy(policyType string) bool {
	switch policyType {
	case "protocol", "invitation", "approval":
		return true
	default:
		return false
	}
}

// CheckAdmission evaluates whether a user can register based on the server's admission policy.
func (s *AdmissionService) CheckAdmission(pubKey string, invitationCode *string) (bool, string, error) {
	switch s.cfg.PolicyType {
	case "protocol":
		// Auto-admit: any valid AgentOS client is accepted
		return true, "auto_admitted", nil

	case "invitation":
		if invitationCode == nil || *invitationCode != s.cfg.InvitationCode {
			return false, "invalid_invitation_code", nil
		}
		return true, "admitted_via_invitation", nil

	case "approval":
		// Create a pending admission request
		req := &model.AdmissionRequest{
			UserPubKey: pubKey,
			Status:     "pending",
		}
		if err := s.userRepo.CreateAdmissionRequest(req); err != nil {
			return false, "", fmt.Errorf("create admission request: %w", err)
		}
		return false, "pending_approval", nil

	default:
		return false, "", fmt.Errorf("unknown admission policy: %s", s.cfg.PolicyType)
	}
}

// ApproveRequest approves a pending admission request (admin only).
func (s *AdmissionService) ApproveRequest(requestID uuid.UUID) error {
	req, err := s.userRepo.GetAdmissionRequest(requestID)
	if err != nil {
		return fmt.Errorf("request not found: %w", err)
	}
	if req.Status != "pending" {
		return fmt.Errorf("request already %s", req.Status)
	}
	req.Status = "approved"
	return s.userRepo.UpdateAdmissionRequest(req)
}

// RejectRequest rejects a pending admission request (admin only).
func (s *AdmissionService) RejectRequest(requestID uuid.UUID, reason string) error {
	req, err := s.userRepo.GetAdmissionRequest(requestID)
	if err != nil {
		return fmt.Errorf("request not found: %w", err)
	}
	if req.Status != "pending" {
		return fmt.Errorf("request already %s", req.Status)
	}
	req.Status = "rejected"
	req.Reason = reason
	return s.userRepo.UpdateAdmissionRequest(req)
}

// GetPendingRequests returns all pending admission requests.
func (s *AdmissionService) GetPendingRequests() ([]model.AdmissionRequest, error) {
	return s.userRepo.GetPendingAdmissionRequests()
}
