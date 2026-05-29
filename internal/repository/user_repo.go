package repository

import (
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// === User ===

func (r *UserRepo) Create(u *model.User) error {
	return r.db.Create(u).Error
}

func (r *UserRepo) GetByID(id uuid.UUID) (*model.User, error) {
	var u model.User
	err := r.db.Where("id = ?", id).First(&u).Error
	return &u, err
}

func (r *UserRepo) GetByPubKey(pubKey string) (*model.User, error) {
	var u model.User
	err := r.db.Where("pubkey_ed25519 = ?", pubKey).First(&u).Error
	return &u, err
}

func (r *UserRepo) Update(u *model.User) error {
	return r.db.Save(u).Error
}

func (r *UserRepo) PubKeyExists(pubKey string) (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).Where("pubkey_ed25519 = ?", pubKey).Count(&count).Error
	return count > 0, err
}

// === Device ===

func (r *UserRepo) CreateDevice(d *model.Device) error {
	return r.db.Create(d).Error
}

func (r *UserRepo) GetDevice(deviceID string) (*model.Device, error) {
	var d model.Device
	err := r.db.Where("device_id = ?", deviceID).First(&d).Error
	return &d, err
}

func (r *UserRepo) GetUserDevices(userID uuid.UUID) ([]model.Device, error) {
	var devices []model.Device
	err := r.db.Where("user_id = ?", userID).Order("paired_at DESC").Find(&devices).Error
	return devices, err
}

func (r *UserRepo) UpdateDeviceLastSeen(deviceID string) error {
	now := time.Now()
	return r.db.Model(&model.Device{}).Where("device_id = ?", deviceID).Update("last_seen_at", &now).Error
}

func (r *UserRepo) DeviceBelongsToUser(deviceID string, userID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&model.Device{}).Where("device_id = ? AND user_id = ?", deviceID, userID).Count(&count).Error
	return count > 0, err
}

// === AuthChallenge ===

func (r *UserRepo) SaveChallenge(ch *model.AuthChallenge) error {
	return r.db.Create(ch).Error
}

func (r *UserRepo) GetChallenge(deviceID, nonce string) (*model.AuthChallenge, error) {
	var ch model.AuthChallenge
	err := r.db.Where("device_id = ? AND nonce = ? AND used_at IS NULL", deviceID, nonce).
		Where("expires_at > ?", time.Now()).
		First(&ch).Error
	return &ch, err
}

func (r *UserRepo) MarkChallengeUsed(id uuid.UUID) error {
	now := time.Now()
	return r.db.Model(&model.AuthChallenge{}).Where("id = ?", id).Update("used_at", &now).Error
}

// === AdmissionRequest ===

func (r *UserRepo) CreateAdmissionRequest(req *model.AdmissionRequest) error {
	return r.db.Create(req).Error
}

func (r *UserRepo) GetAdmissionRequest(id uuid.UUID) (*model.AdmissionRequest, error) {
	var req model.AdmissionRequest
	err := r.db.Where("id = ?", id).First(&req).Error
	return &req, err
}

func (r *UserRepo) GetLatestAdmissionRequestByPubKey(pubKey string) (*model.AdmissionRequest, error) {
	var req model.AdmissionRequest
	err := r.db.Where("user_pub_key = ?", pubKey).Order("created_at DESC").First(&req).Error
	return &req, err
}

func (r *UserRepo) UpdateAdmissionRequest(req *model.AdmissionRequest) error {
	return r.db.Save(req).Error
}

func (r *UserRepo) GetPendingAdmissionRequests() ([]model.AdmissionRequest, error) {
	var reqs []model.AdmissionRequest
	err := r.db.Where("status = ?", "pending").Order("created_at ASC").Find(&reqs).Error
	return reqs, err
}
