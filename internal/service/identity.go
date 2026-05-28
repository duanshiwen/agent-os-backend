package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

type IdentityService struct {
	userRepo *repository.UserRepo
	jwtCfg   config.JWTConfig
	verifier SignatureVerifier
}

func NewIdentityService(userRepo *repository.UserRepo, jwtCfg config.JWTConfig, verifier SignatureVerifier) *IdentityService {
	return &IdentityService{userRepo: userRepo, jwtCfg: jwtCfg, verifier: verifier}
}

func NewIdentityServiceWithVerifier(userRepo *repository.UserRepo, jwtCfg config.JWTConfig, verifier SignatureVerifier) *IdentityService {
	return NewIdentityService(userRepo, jwtCfg, verifier)
}

// ChallengeResult is what the server sends back to the client.
type ChallengeResult struct {
	Challenge string `json:"challenge"`
	Nonce     string `json:"nonce"`
	ExpiresAt int64  `json:"expires_at"`
}

// InitiateChallenge generates a random challenge for the given device+pubkey.
func (s *IdentityService) InitiateChallenge(deviceID, userPubKey string) (*ChallengeResult, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return nil, fmt.Errorf("generate challenge: %w", err)
	}

	ch := &model.AuthChallenge{
		DeviceID:   deviceID,
		UserPubKey: userPubKey,
		Nonce:      hex.EncodeToString(nonce),
		Challenge:  hex.EncodeToString(challengeBytes),
		ExpiresAt:  time.Now().Add(2 * time.Minute),
	}
	if err := s.userRepo.SaveChallenge(ch); err != nil {
		return nil, fmt.Errorf("save challenge: %w", err)
	}

	return &ChallengeResult{
		Challenge: ch.Challenge,
		Nonce:     ch.Nonce,
		ExpiresAt: ch.ExpiresAt.Unix(),
	}, nil
}

// VerifyRequest is what the client sends after signing the challenge.
type VerifyRequest struct {
	DeviceID   string `json:"device_id" binding:"required"`
	UserPubKey string `json:"user_pubkey" binding:"required"`
	Nonce      string `json:"nonce" binding:"required"`
	Signature  string `json:"signature" binding:"required"` // hex-encoded Ed25519 signature
}

// AuthResponse is returned after successful verification.
type AuthResponse struct {
	AccessToken string        `json:"access_token"`
	User        *model.User   `json:"user"`
	Device      *model.Device `json:"device"`
	IsNewUser   bool          `json:"is_new_user"`
}

// VerifySignature validates the Ed25519 signature and issues a JWT.
func (s *IdentityService) VerifySignature(req *VerifyRequest) (*AuthResponse, error) {
	return s.VerifySignatureContext(context.Background(), req)
}

// VerifySignatureContext validates the Ed25519 signature and issues a JWT.
func (s *IdentityService) VerifySignatureContext(ctx context.Context, req *VerifyRequest) (*AuthResponse, error) {
	// 1. Retrieve and validate challenge
	ch, err := s.userRepo.GetChallenge(req.DeviceID, req.Nonce)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired challenge")
	}

	// 2. Verify Ed25519 signature via the configured verifier.
	valid, err := s.verifier.VerifyEd25519Challenge(ctx, ch.Challenge, req.Signature, req.UserPubKey)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("signature verification failed")
	}

	// 4. Mark challenge as used
	_ = s.userRepo.MarkChallengeUsed(ch.ID)

	// 5. Find or create user
	user, isNew, err := s.findOrCreateUser(req.UserPubKey)
	if err != nil {
		return nil, err
	}

	// 6. Register device if new
	device, err := s.findOrCreateDevice(req.DeviceID, user.ID, req.UserPubKey)
	if err != nil {
		return nil, err
	}

	// 7. Update last seen
	_ = s.userRepo.UpdateDeviceLastSeen(req.DeviceID)

	// 8. Generate JWT
	token, err := middleware.GenerateToken(
		s.jwtCfg.Secret, user.ID, req.DeviceID,
		s.jwtCfg.Issuer, s.jwtCfg.AccessTokenMins,
	)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	return &AuthResponse{
		AccessToken: token,
		User:        user,
		Device:      device,
		IsNewUser:   isNew,
	}, nil
}

func (s *IdentityService) findOrCreateUser(pubKey string) (*model.User, bool, error) {
	user, err := s.userRepo.GetByPubKey(pubKey)
	if err == nil {
		return user, false, nil
	}

	// Create new user
	user = &model.User{
		PubKeyEd25519: pubKey,
		DisplayName:   "AgentOS User",
		Status:        "active",
	}
	if err := s.userRepo.Create(user); err != nil {
		return nil, false, fmt.Errorf("create user: %w", err)
	}
	return user, true, nil
}

func (s *IdentityService) findOrCreateDevice(deviceID string, userID uuid.UUID, pubKey string) (*model.Device, error) {
	device, err := s.userRepo.GetDevice(deviceID)
	if err == nil {
		return device, nil
	}

	device = &model.Device{
		UserID:       userID,
		DeviceID:     deviceID,
		DeviceName:   "New Device",
		DevicePubKey: pubKey,
		PairedAt:     time.Now(),
	}
	if err := s.userRepo.CreateDevice(device); err != nil {
		return nil, fmt.Errorf("create device: %w", err)
	}
	return device, nil
}

// RegisterRequest is sent by a new user to set up their account.
type RegisterRequest struct {
	PubKey      string `json:"pubkey_ed25519" binding:"required"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"` // optional, for sensitive ops
}

// Register creates a new user with the given public key.
func (s *IdentityService) Register(req *RegisterRequest) (*model.User, error) {
	exists, err := s.userRepo.PubKeyExists(req.PubKey)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("public key already registered")
	}

	user := &model.User{
		PubKeyEd25519: req.PubKey,
		DisplayName:   req.DisplayName,
		Status:        "active",
	}

	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		user.PasswordHash = string(hash)
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// GetProfile returns the current user's profile.
func (s *IdentityService) GetProfile(userID uuid.UUID) (*model.User, error) {
	return s.userRepo.GetByID(userID)
}

// UpdateProfile updates the current user's profile.
func (s *IdentityService) UpdateProfile(userID uuid.UUID, displayName, avatarURL *string) (*model.User, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if displayName != nil {
		user.DisplayName = *displayName
	}
	if avatarURL != nil {
		user.AvatarURL = *avatarURL
	}
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	return user, nil
}

// PairDeviceRequest adds a new device to an existing user.
type PairDeviceRequest struct {
	DeviceID     string `json:"device_id" binding:"required"`
	DeviceName   string `json:"device_name"`
	DevicePubKey string `json:"device_pubkey" binding:"required"`
}

// PairDevice adds a new device to the authenticated user.
func (s *IdentityService) PairDevice(userID uuid.UUID, req *PairDeviceRequest) (*model.Device, error) {
	// Check if device already exists
	existing, err := s.userRepo.GetDevice(req.DeviceID)
	if err == nil {
		if existing.UserID != userID {
			return nil, fmt.Errorf("device belongs to another user")
		}
		return existing, nil
	}

	device := &model.Device{
		UserID:       userID,
		DeviceID:     req.DeviceID,
		DeviceName:   req.DeviceName,
		DevicePubKey: req.DevicePubKey,
		PairedAt:     time.Now(),
	}
	if err := s.userRepo.CreateDevice(device); err != nil {
		return nil, fmt.Errorf("pair device: %w", err)
	}
	return device, nil
}

// GetUserDevices returns all devices for the authenticated user.
func (s *IdentityService) GetUserDevices(userID uuid.UUID) ([]model.Device, error) {
	return s.userRepo.GetUserDevices(userID)
}

// VerifyPassword checks if the provided password matches the user's stored hash.
// Used for sensitive operations like new device login or asset transactions.
func (s *IdentityService) VerifyPassword(userID uuid.UUID, password string) (bool, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return false, fmt.Errorf("user not found: %w", err)
	}
	if user.PasswordHash == "" {
		return false, fmt.Errorf("no password set")
	}
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	return err == nil, nil
}

// SetPassword sets or updates the user's password.
func (s *IdentityService) SetPassword(userID uuid.UUID, password string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user.PasswordHash = string(hash)
	return s.userRepo.Update(user)
}
