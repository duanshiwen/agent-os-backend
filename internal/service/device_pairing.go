package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const defaultPairingTTL = 2 * time.Minute

type DevicePairingService struct {
	pairingRepo *repository.DevicePairingRepo
	userRepo    *repository.UserRepo
	verifier    SignatureVerifier
	serverID    string
	ttl         time.Duration
	auditSvc    *AuditService
}

type DevicePairingQRPayload struct {
	Version          int       `json:"version"`
	ServerID         string    `json:"server_id"`
	PairingSessionID uuid.UUID `json:"pairing_session_id"`
	PairingToken     string    `json:"pairing_token"`
	ExpiresAt        int64     `json:"expires_at"`
}

type StartPairingResponse struct {
	PairingSessionID uuid.UUID `json:"pairing_session_id"`
	QRPayload        string    `json:"qr_payload"`
	ExpiresAt        int64     `json:"expires_at"`
}

type ClaimPairingRequest struct {
	QRPayload       string `json:"qr_payload" binding:"required"`
	NewDeviceID     string `json:"new_device_id" binding:"required"`
	NewDeviceName   string `json:"new_device_name"`
	NewDevicePubKey string `json:"new_device_pubkey" binding:"required"`
	Signature       string `json:"signature" binding:"required"`
}

func NewDevicePairingService(pairingRepo *repository.DevicePairingRepo, userRepo *repository.UserRepo, verifier SignatureVerifier) *DevicePairingService {
	return &DevicePairingService{pairingRepo: pairingRepo, userRepo: userRepo, verifier: verifier, serverID: "default", ttl: defaultPairingTTL}
}

func (s *DevicePairingService) SetAuditService(auditSvc *AuditService) {
	s.auditSvc = auditSvc
}

func (s *DevicePairingService) StartPairing(userID uuid.UUID, createdByDeviceID string) (*StartPairingResponse, error) {
	oldDevice, err := s.userRepo.GetDevice(createdByDeviceID)
	if err != nil {
		return nil, fmt.Errorf("device not found: %w", err)
	}
	if oldDevice.UserID != userID {
		return nil, fmt.Errorf("device does not belong to user")
	}
	if oldDevice.Status == "revoked" {
		return nil, fmt.Errorf("device revoked")
	}

	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(s.ttl)
	session := &model.DevicePairingSession{
		UserID:            userID,
		ExpiresAt:         expiresAt,
		CreatedByDeviceID: createdByDeviceID,
	}
	payload := DevicePairingQRPayload{
		Version:          1,
		ServerID:         s.serverID,
		PairingSessionID: session.ID,
		PairingToken:     token,
		ExpiresAt:        expiresAt.Unix(),
	}

	// Ensure Base.BeforeCreate has assigned the UUID before building the QR payload.
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
		payload.PairingSessionID = session.ID
	}

	encodedPayload, err := encodeQRPayload(payload)
	if err != nil {
		return nil, err
	}
	tokenHash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash pairing token: %w", err)
	}
	session.PairingTokenHash = string(tokenHash)
	session.QRPayloadHash = sha256Hex(encodedPayload)

	if err := s.pairingRepo.CreatePairingSession(session); err != nil {
		return nil, fmt.Errorf("create pairing session: %w", err)
	}
	s.recordAudit(&userID, createdByDeviceID, AuditActionDevicePairingStarted, "device_pairing_session", session.ID.String(), AuditOutcomeSuccess, map[string]any{"expires_at": expiresAt.Unix()})

	return &StartPairingResponse{PairingSessionID: session.ID, QRPayload: encodedPayload, ExpiresAt: expiresAt.Unix()}, nil
}

func (s *DevicePairingService) ClaimPairing(req *ClaimPairingRequest) (*model.Device, error) {
	payload, err := decodeQRPayload(req.QRPayload)
	if err != nil {
		return nil, err
	}
	if payload.ServerID != s.serverID {
		return nil, fmt.Errorf("invalid pairing server")
	}
	if payload.ExpiresAt <= time.Now().Unix() {
		return nil, fmt.Errorf("pairing session expired")
	}

	session, err := s.pairingRepo.GetPairingSession(payload.PairingSessionID)
	if err != nil {
		return nil, fmt.Errorf("pairing session not found: %w", err)
	}
	if session.UsedAt != nil {
		return nil, fmt.Errorf("pairing session already used")
	}
	if session.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("pairing session expired")
	}
	if session.QRPayloadHash != sha256Hex(req.QRPayload) {
		return nil, fmt.Errorf("invalid qr payload")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(session.PairingTokenHash), []byte(payload.PairingToken)); err != nil {
		return nil, fmt.Errorf("invalid pairing token")
	}
	if _, err := s.userRepo.GetDevice(req.NewDeviceID); err == nil {
		return nil, fmt.Errorf("device id already exists")
	}
	if s.verifier == nil {
		return nil, fmt.Errorf("new device signature verifier unavailable")
	}
	claimChallenge := s.canonicalClaimChallenge(payload, req)
	valid, err := s.verifier.VerifyEd25519Challenge(context.Background(), claimChallenge, req.Signature, req.NewDevicePubKey)
	if err != nil {
		return nil, fmt.Errorf("verify new device signature: %w", err)
	}
	if !valid {
		return nil, fmt.Errorf("new device signature verification failed")
	}

	deviceName := req.NewDeviceName
	if deviceName == "" {
		deviceName = "New Device"
	}
	device := &model.Device{
		UserID:       session.UserID,
		DeviceID:     req.NewDeviceID,
		DeviceName:   deviceName,
		DevicePubKey: req.NewDevicePubKey,
		Status:       "active",
		PairedAt:     time.Now(),
	}
	if err := s.pairingRepo.ClaimPairingSession(session.ID, device); err != nil {
		return nil, err
	}
	s.recordAudit(&session.UserID, req.NewDeviceID, AuditActionDevicePairingClaimed, "device", req.NewDeviceID, AuditOutcomeSuccess, map[string]any{"pairing_session_id": session.ID.String(), "created_by_device_id": session.CreatedByDeviceID})
	return device, nil
}

func (s *DevicePairingService) recordAudit(actorUserID *uuid.UUID, actorDeviceID, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	if s.auditSvc == nil {
		return
	}
	_, _ = s.auditSvc.Record(RecordAuditEventInput{ActorUserID: actorUserID, ActorDeviceID: actorDeviceID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, Metadata: metadata})
}

func (s *DevicePairingService) canonicalClaimChallenge(payload *DevicePairingQRPayload, req *ClaimPairingRequest) string {
	claim := map[string]any{
		"purpose":            "agentos.device_pairing.claim",
		"version":            payload.Version,
		"server_id":          payload.ServerID,
		"pairing_session_id": payload.PairingSessionID.String(),
		"new_device_id":      req.NewDeviceID,
		"new_device_pubkey":  req.NewDevicePubKey,
		"expires_at":         payload.ExpiresAt,
	}
	data, _ := json.Marshal(claim)
	return string(data)
}

func encodeQRPayload(payload DevicePairingQRPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal qr payload: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeQRPayload(encoded string) (*DevicePairingQRPayload, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid qr payload: %w", err)
	}
	var payload DevicePairingQRPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("invalid qr payload: %w", err)
	}
	return &payload, nil
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate pairing token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
