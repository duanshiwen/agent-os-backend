package repository

import (
	"strings"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	SkillStatusDraft     = "draft"
	SkillStatusPublished = "published"
	SkillStatusSuspended = "suspended"
	SkillStatusArchived  = "archived"
	SkillStatusRemoved   = "removed"

	SkillVisibilityPrivate  = "private"
	SkillVisibilityPublic   = "public"
	SkillVisibilityUnlisted = "unlisted"

	SkillVersionStatusDraft     = "draft"
	SkillVersionStatusPublished = "published"
	SkillVersionStatusRejected  = "rejected"
	SkillVersionStatusArchived  = "archived"
	SkillVersionStatusSuspended = "suspended"

	SkillValidationStatusValid   = "valid"
	SkillValidationStatusInvalid = "invalid"

	SkillInstallationStatusActive      = "active"
	SkillInstallationStatusDisabled    = "disabled"
	SkillInstallationStatusUninstalled = "uninstalled"

	SkillTrackModeLatest = "latest"
	SkillTrackModePinned = "pinned"

	SkillPublisherRestrictionActive = "active"
	SkillPublisherRestrictionLifted = "lifted"
)

type SkillHubRepo struct{ db *gorm.DB }

func NewSkillHubRepo(db *gorm.DB) *SkillHubRepo { return &SkillHubRepo{db: db} }

func (r *SkillHubRepo) Transaction(fn func(txRepo *SkillHubRepo) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error { return fn(&SkillHubRepo{db: tx}) })
}

func (r *SkillHubRepo) CreateSkill(skill *model.Skill) error { return r.db.Create(skill).Error }
func (r *SkillHubRepo) UpdateSkill(skill *model.Skill) error { return r.db.Save(skill).Error }
func (r *SkillHubRepo) CreateVersion(version *model.SkillVersion) error {
	return r.db.Create(version).Error
}
func (r *SkillHubRepo) UpdateVersion(version *model.SkillVersion) error {
	return r.db.Save(version).Error
}
func (r *SkillHubRepo) CreateInstallation(installation *model.SkillInstallation) error {
	return r.db.Create(installation).Error
}
func (r *SkillHubRepo) UpdateInstallation(installation *model.SkillInstallation) error {
	return r.db.Save(installation).Error
}
func (r *SkillHubRepo) CreateRestriction(restriction *model.SkillPublisherRestriction) error {
	return r.db.Create(restriction).Error
}
func (r *SkillHubRepo) UpdateRestriction(restriction *model.SkillPublisherRestriction) error {
	return r.db.Save(restriction).Error
}

func (r *SkillHubRepo) GetSkill(id uuid.UUID) (*model.Skill, error) {
	var skill model.Skill
	err := r.db.First(&skill, "id = ?", id).Error
	return &skill, err
}

func (r *SkillHubRepo) GetSkillByKey(skillKey string) (*model.Skill, error) {
	var skill model.Skill
	err := r.db.First(&skill, "skill_key = ?", strings.TrimSpace(skillKey)).Error
	return &skill, err
}

func (r *SkillHubRepo) GetPublishedSkillByKey(skillKey string) (*model.Skill, error) {
	var skill model.Skill
	err := r.db.First(&skill, "skill_key = ? AND status = ? AND visibility IN ?", strings.TrimSpace(skillKey), SkillStatusPublished, []string{SkillVisibilityPublic, SkillVisibilityUnlisted}).Error
	return &skill, err
}

func (r *SkillHubRepo) GetVersion(id uuid.UUID) (*model.SkillVersion, error) {
	var version model.SkillVersion
	err := r.db.First(&version, "id = ?", id).Error
	return &version, err
}

func (r *SkillHubRepo) GetVersionBySkillAndVersion(skillID uuid.UUID, version string) (*model.SkillVersion, error) {
	var v model.SkillVersion
	err := r.db.First(&v, "skill_id = ? AND version = ?", skillID, strings.TrimSpace(version)).Error
	return &v, err
}

func (r *SkillHubRepo) SearchPublishedSkills(q, category string, limit, offset int) ([]model.Skill, error) {
	var skills []model.Skill
	db := r.db.Model(&model.Skill{}).Where("status = ? AND visibility = ?", SkillStatusPublished, SkillVisibilityPublic)
	if strings.TrimSpace(category) != "" {
		db = db.Where("category = ?", strings.TrimSpace(category))
	}
	if strings.TrimSpace(q) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
		db = db.Where("LOWER(name) LIKE ? OR LOWER(summary) LIKE ? OR LOWER(description) LIKE ? OR LOWER(skill_key) LIKE ?", like, like, like, like)
	}
	if offset < 0 {
		offset = 0
	}
	err := db.Order("updated_at DESC").Limit(normalizeLimit(limit)).Offset(offset).Find(&skills).Error
	return skills, err
}

func (r *SkillHubRepo) CountPublishedSkills(q, category string) (int64, error) {
	var count int64
	db := r.db.Model(&model.Skill{}).Where("status = ? AND visibility = ?", SkillStatusPublished, SkillVisibilityPublic)
	if strings.TrimSpace(category) != "" {
		db = db.Where("category = ?", strings.TrimSpace(category))
	}
	if strings.TrimSpace(q) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
		db = db.Where("LOWER(name) LIKE ? OR LOWER(summary) LIKE ? OR LOWER(description) LIKE ? OR LOWER(skill_key) LIKE ?", like, like, like, like)
	}
	return count, db.Count(&count).Error
}

func (r *SkillHubRepo) GetInstallation(id uuid.UUID) (*model.SkillInstallation, error) {
	var installation model.SkillInstallation
	err := r.db.First(&installation, "id = ?", id).Error
	return &installation, err
}

func (r *SkillHubRepo) GetInstallationByUserSkill(userID, skillID uuid.UUID) (*model.SkillInstallation, error) {
	var installation model.SkillInstallation
	err := r.db.First(&installation, "user_id = ? AND skill_id = ?", userID, skillID).Error
	return &installation, err
}

func (r *SkillHubRepo) ListInstallations(userID uuid.UUID) ([]model.SkillInstallation, error) {
	var installations []model.SkillInstallation
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&installations).Error
	return installations, err
}

func (r *SkillHubRepo) GetActiveRestriction(publisherID uuid.UUID) (*model.SkillPublisherRestriction, error) {
	var restriction model.SkillPublisherRestriction
	err := r.db.First(&restriction, "publisher_id = ? AND status = ?", publisherID, SkillPublisherRestrictionActive).Error
	return &restriction, err
}
