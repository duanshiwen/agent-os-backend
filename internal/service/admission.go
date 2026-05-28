package service

import (
	"fmt"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

type AdmissionService struct {
	userRepo *repository.UserRepo
	cfg      config.AdmissionConfig
}

func NewAdmissionService(userRepo *repository.UserRepo, cfg config.AdmissionConfig) *AdmissionService {
	return &AdmissionService{userRepo: userRepo, cfg: cfg}
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
