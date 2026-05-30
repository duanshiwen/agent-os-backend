package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const defaultSensitiveConfirmationTTL = 5 * time.Minute

const (
	SensitiveOperationDeviceRevoke              = "device.revoke"
	SensitiveOperationAdmissionInvitationUpdate = "admission.invitation_code.update"
	SensitiveOperationAdmissionPolicyUpdate     = "admission.policy.update"
)

type SensitiveOperationService struct {
	userRepo *repository.UserRepo
	repo     *repository.SensitiveOperationRepo
	auditSvc *AuditService
	ttl      time.Duration
}

type SensitiveConfirmationResponse struct {
	ConfirmationToken string    `json:"confirmation_token"`
	Operation         string    `json:"operation"`
	ExpiresAt         time.Time `json:"expires_at"`
}

func NewSensitiveOperationService(userRepo *repository.UserRepo, repo *repository.SensitiveOperationRepo, auditSvc *AuditService) *SensitiveOperationService {
	return &SensitiveOperationService{userRepo: userRepo, repo: repo, auditSvc: auditSvc, ttl: defaultSensitiveConfirmationTTL}
}

func (s *SensitiveOperationService) SetPassword(userID uuid.UUID, actorDeviceID, password string) error {
	password = strings.TrimSpace(password)
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	if user.PasswordHash != "" {
		return fmt.Errorf("password already set")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hash)
	if err := s.userRepo.Update(user); err != nil {
		return err
	}
	s.recordAudit(userID, actorDeviceID, AuditActionPasswordSet, "user", userID.String(), AuditOutcomeSuccess, nil)
	return nil
}

func (s *SensitiveOperationService) ChangePassword(userID uuid.UUID, actorDeviceID, currentPassword, newPassword string) error {
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	if user.PasswordHash == "" {
		return fmt.Errorf("no password set")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		s.recordAudit(userID, actorDeviceID, AuditActionPasswordChanged, "user", userID.String(), AuditOutcomeDenied, map[string]any{"reason": "invalid_current_password"})
		return fmt.Errorf("invalid current password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hash)
	if err := s.userRepo.Update(user); err != nil {
		return err
	}
	s.recordAudit(userID, actorDeviceID, AuditActionPasswordChanged, "user", userID.String(), AuditOutcomeSuccess, nil)
	return nil
}

func (s *SensitiveOperationService) IssueConfirmation(userID uuid.UUID, actorDeviceID, password, operation string) (*SensitiveConfirmationResponse, error) {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return nil, fmt.Errorf("operation is required")
	}
	ok, err := s.verifyPassword(userID, password)
	if err != nil {
		return nil, err
	}
	if !ok {
		s.recordAudit(userID, actorDeviceID, AuditActionSensitiveConfirmationIssued, "sensitive_operation", operation, AuditOutcomeDenied, map[string]any{"reason": "invalid_password"})
		return nil, fmt.Errorf("invalid password")
	}
	token, err := randomSensitiveToken()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().UTC().Add(s.ttl)
	confirmation := &model.SensitiveOperationConfirmation{UserID: userID, DeviceID: actorDeviceID, Operation: operation, TokenHash: hashSensitiveToken(token), ExpiresAt: expiresAt}
	if err := s.repo.CreateConfirmation(confirmation); err != nil {
		return nil, err
	}
	s.recordAudit(userID, actorDeviceID, AuditActionSensitiveConfirmationIssued, "sensitive_operation", operation, AuditOutcomeSuccess, map[string]any{"expires_at": expiresAt.Unix()})
	return &SensitiveConfirmationResponse{ConfirmationToken: token, Operation: operation, ExpiresAt: expiresAt}, nil
}

func (s *SensitiveOperationService) ConsumeConfirmation(userID uuid.UUID, token, operation, consumedBy string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("confirmation token is required")
	}
	confirmation, err := s.repo.GetUsableConfirmation(userID, hashSensitiveToken(token), operation, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("invalid or expired confirmation token")
	}
	if err := s.repo.MarkUsed(confirmation.ID, consumedBy); err != nil {
		return err
	}
	s.recordAudit(userID, confirmation.DeviceID, AuditActionSensitiveConfirmationConsumed, "sensitive_operation", operation, AuditOutcomeSuccess, map[string]any{"consumed_by": consumedBy})
	return nil
}

func (s *SensitiveOperationService) verifyPassword(userID uuid.UUID, password string) (bool, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return false, fmt.Errorf("user not found: %w", err)
	}
	if user.PasswordHash == "" {
		return false, fmt.Errorf("no password set")
	}
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil, nil
}

func (s *SensitiveOperationService) recordAudit(userID uuid.UUID, actorDeviceID, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	if s.auditSvc == nil {
		return
	}
	actorID := userID
	_, _ = s.auditSvc.Record(RecordAuditEventInput{ActorUserID: &actorID, ActorDeviceID: actorDeviceID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, Metadata: metadata})
}

func randomSensitiveToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashSensitiveToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
