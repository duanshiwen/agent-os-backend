package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrSkillNotFound          = errors.New("skill not found")
	ErrSkillVersionNotFound   = errors.New("skill version not found")
	ErrSkillInstallationFound = errors.New("skill installation not found")
	ErrSkillInvalid           = errors.New("skill request invalid")
	ErrSkillForbidden         = errors.New("skill operation forbidden")
	ErrSkillPublisherBlocked  = errors.New("skill publisher is restricted")
)

type SkillHubService struct {
	repo      *repository.SkillHubRepo
	validator *SkillManifestValidator
	syncSvc   *SyncService
	auditSvc  *AuditService
	objectSvc *ObjectService
}

func NewSkillHubService(repo *repository.SkillHubRepo) *SkillHubService {
	return &SkillHubService{repo: repo, validator: NewSkillManifestValidator()}
}
func (s *SkillHubService) SetSyncService(syncSvc *SyncService)       { s.syncSvc = syncSvc }
func (s *SkillHubService) SetAuditService(auditSvc *AuditService)    { s.auditSvc = auditSvc }
func (s *SkillHubService) SetObjectService(objectSvc *ObjectService) { s.objectSvc = objectSvc }

type CreateSkillInput struct {
	SkillKey     string     `json:"skill_key"`
	Name         string     `json:"name"`
	Summary      string     `json:"summary"`
	Description  string     `json:"description"`
	Category     string     `json:"category"`
	Tags         []string   `json:"tags"`
	IconObjectID *uuid.UUID `json:"icon_object_id"`
	HomepageURL  string     `json:"homepage_url"`
}

type SubmitSkillVersionInput struct {
	Version              string         `json:"version"`
	Manifest             map[string]any `json:"manifest"`
	PackageObjectID      *uuid.UUID     `json:"package_object_id"`
	InstructionsObjectID *uuid.UUID     `json:"instructions_object_id"`
	ExamplesObjectIDs    []string       `json:"examples_object_ids"`
}

type InstallSkillInput struct {
	TrackMode string         `json:"track_mode"`
	Config    map[string]any `json:"config"`
}

type UpdateSkillInstallationInput struct {
	TrackMode string         `json:"track_mode"`
	VersionID *uuid.UUID     `json:"version_id"`
	Config    map[string]any `json:"config"`
}

type SkillCatalogAsset struct {
	ObjectID    uuid.UUID `json:"object_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	ContentHash string    `json:"content_hash"`
	ContentSize int64     `json:"content_size"`
}

type SkillCatalogItem struct {
	model.Skill
	PublishedVersion *model.SkillVersion `json:"published_version,omitempty"`
	IconAsset        *SkillCatalogAsset  `json:"icon_asset,omitempty"`
	PackageAsset     *SkillCatalogAsset  `json:"package_asset,omitempty"`
}

type SkillCatalogPage struct {
	Items         []SkillCatalogItem `json:"items"`
	Limit, Offset int                `json:"limit"`
	Total         int64              `json:"total"`
	Sort          string             `json:"sort"`
}

type RateSkillInput struct {
	Rating int `json:"rating"`
}

type SkillRatingAggregate struct {
	SkillID       uuid.UUID `json:"skill_id"`
	SkillKey      string    `json:"skill_key"`
	RatingCount   int64     `json:"rating_count"`
	RatingAverage float64   `json:"rating_average"`
	DownloadCount int64     `json:"download_count"`
	UserRating    *int      `json:"user_rating,omitempty"`
}

func (s *SkillHubService) CreateSkill(publisherID uuid.UUID, input CreateSkillInput) (*model.Skill, error) {
	if err := s.ensurePublisherAllowed(publisherID); err != nil {
		return nil, err
	}
	skillKey := strings.TrimSpace(input.SkillKey)
	if err := validateSkillKey(skillKey); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSkillInvalid, err.Error())
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrSkillInvalid)
	}
	if input.IconObjectID != nil {
		icon, err := s.requireActiveOwnedAsset(publisherID, *input.IconObjectID, "icon", []string{"image/"})
		if err != nil {
			return nil, err
		}
		input.IconObjectID = &icon.ID
	}
	skill := &model.Skill{SkillKey: skillKey, PublisherID: publisherID, Name: name, Summary: strings.TrimSpace(input.Summary), Description: strings.TrimSpace(input.Description), Category: strings.TrimSpace(input.Category), Tags: datatypes.JSONSlice[string](cleanStrings(input.Tags)), IconObjectID: input.IconObjectID, HomepageURL: strings.TrimSpace(input.HomepageURL), Status: repository.SkillStatusDraft, Visibility: repository.SkillVisibilityPrivate}
	if err := s.repo.CreateSkill(skill); err != nil {
		return nil, err
	}
	s.recordAudit(&publisherID, "", AuditActionSkillCreated, "skill", skill.ID.String(), AuditOutcomeSuccess, map[string]any{"skill_key": skill.SkillKey})
	return skill, nil
}

func (s *SkillHubService) SubmitVersion(publisherID, skillID uuid.UUID, input SubmitSkillVersionInput) (*model.SkillVersion, *SkillManifestValidationResult, error) {
	if err := s.ensurePublisherAllowed(publisherID); err != nil {
		return nil, nil, err
	}
	skill, err := s.repo.GetSkill(skillID)
	if err != nil {
		return nil, nil, ErrSkillNotFound
	}
	if skill.PublisherID != publisherID {
		return nil, nil, ErrSkillForbidden
	}
	result, err := s.validator.Validate(skill.SkillKey, input.Manifest)
	if err != nil {
		return nil, nil, err
	}
	version := strings.TrimSpace(input.Version)
	if version == "" {
		version = result.Version
	}
	if version == "" {
		return nil, nil, fmt.Errorf("%w: version is required", ErrSkillInvalid)
	}
	if input.PackageObjectID != nil {
		pkg, err := s.requireActiveOwnedAsset(publisherID, *input.PackageObjectID, "package", []string{"application/zip", "application/gzip", "application/x-tar", "application/octet-stream"})
		if err != nil {
			return nil, nil, err
		}
		input.PackageObjectID = &pkg.ID
	}
	if input.InstructionsObjectID != nil {
		inst, err := s.requireActiveOwnedAsset(publisherID, *input.InstructionsObjectID, "instructions", []string{"text/", "application/json", "application/octet-stream"})
		if err != nil {
			return nil, nil, err
		}
		input.InstructionsObjectID = &inst.ID
	}
	now := time.Now().UTC()
	status := repository.SkillVersionStatusPublished
	validationStatus := repository.SkillValidationStatusValid
	var publishedAt *time.Time = &now
	if !result.Valid {
		status = repository.SkillVersionStatusDraft
		validationStatus = repository.SkillValidationStatusInvalid
		publishedAt = nil
	}
	v := &model.SkillVersion{SkillID: skill.ID, Version: version, ManifestHash: result.ManifestHash, ManifestSnapshot: datatypes.JSONMap(result.Snapshot), PackageObjectID: input.PackageObjectID, InstructionsObjectID: input.InstructionsObjectID, ExamplesObjectIDs: datatypes.JSONSlice[string](cleanStrings(input.ExamplesObjectIDs)), ValidationStatus: validationStatus, ValidationErrors: datatypes.JSONSlice[string](result.Errors), ValidationWarnings: datatypes.JSONSlice[string](result.Warnings), Status: status, PublishedAt: publishedAt}
	err = s.repo.Transaction(func(tx *repository.SkillHubRepo) error {
		if err := tx.CreateVersion(v); err != nil {
			return err
		}
		skill.LatestVersionID = &v.ID
		if result.Valid {
			skill.Name = firstNonBlank(result.Name, skill.Name)
			skill.Summary = firstNonBlank(result.Summary, skill.Summary)
			skill.Description = firstNonBlank(result.Description, skill.Description)
			skill.Category = firstNonBlank(result.Category, skill.Category)
			if len(result.Tags) > 0 {
				skill.Tags = datatypes.JSONSlice[string](result.Tags)
			}
			skill.HomepageURL = firstNonBlank(result.HomepageURL, skill.HomepageURL)
			skill.Status = repository.SkillStatusPublished
			skill.Visibility = repository.SkillVisibilityPublic
			skill.PublishedVersionID = &v.ID
		}
		return tx.UpdateSkill(skill)
	})
	if err != nil {
		return nil, nil, err
	}
	if result.Valid {
		s.recordAudit(&publisherID, "", AuditActionSkillVersionPublished, "skill_version", v.ID.String(), AuditOutcomeSuccess, map[string]any{"skill_id": skill.ID.String(), "skill_key": skill.SkillKey, "version": v.Version})
	}
	return v, result, nil
}

func (s *SkillHubService) GetValidation(publisherID, skillID, versionID uuid.UUID) (*model.SkillVersion, error) {
	skill, err := s.repo.GetSkill(skillID)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	if skill.PublisherID != publisherID {
		return nil, ErrSkillForbidden
	}
	version, err := s.repo.GetVersion(versionID)
	if err != nil || version.SkillID != skillID {
		return nil, ErrSkillVersionNotFound
	}
	return version, nil
}

func (s *SkillHubService) SearchCatalog(q, category string, limit, offset int) (*SkillCatalogPage, error) {
	return s.SearchCatalogSorted(q, category, repository.SkillCatalogSortRecommended, limit, offset)
}

func (s *SkillHubService) SearchCatalogSorted(q, category, sort string, limit, offset int) (*SkillCatalogPage, error) {
	sort = normalizeSkillCatalogSort(sort)
	items, err := s.repo.SearchPublishedSkillsSorted(q, category, sort, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.repo.CountPublishedSkills(q, category)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	out := make([]SkillCatalogItem, 0, len(items))
	for _, item := range items {
		enriched, err := s.enrichCatalogSkill(&item)
		if err != nil {
			return nil, err
		}
		out = append(out, *enriched)
	}
	return &SkillCatalogPage{Items: out, Limit: limit, Offset: offset, Total: total, Sort: sort}, nil
}

func (s *SkillHubService) GetCatalogSkill(skillKey string) (*SkillCatalogItem, error) {
	skill, err := s.repo.GetPublishedSkillByKey(skillKey)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	if skill.Visibility != repository.SkillVisibilityPublic && skill.Visibility != repository.SkillVisibilityUnlisted {
		return nil, ErrSkillNotFound
	}
	return s.enrichCatalogSkill(skill)
}

func (s *SkillHubService) RateSkill(userID uuid.UUID, skillKey string, input RateSkillInput) (*SkillRatingAggregate, error) {
	if input.Rating < 1 || input.Rating > 5 {
		return nil, fmt.Errorf("%w: rating must be between 1 and 5", ErrSkillInvalid)
	}
	skill, err := s.repo.GetPublishedSkillByKey(skillKey)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	err = s.repo.Transaction(func(tx *repository.SkillHubRepo) error {
		if err := tx.UpsertRating(&model.SkillRating{SkillID: skill.ID, UserID: userID, Rating: input.Rating}); err != nil {
			return err
		}
		return tx.RecalculateSkillRatingAggregate(skill.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.ratingAggregate(userID, skill.ID)
}

func (s *SkillHubService) DeleteRating(userID uuid.UUID, skillKey string) (*SkillRatingAggregate, error) {
	skill, err := s.repo.GetPublishedSkillByKey(skillKey)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	err = s.repo.Transaction(func(tx *repository.SkillHubRepo) error {
		if err := tx.DeleteRating(skill.ID, userID); err != nil {
			return err
		}
		return tx.RecalculateSkillRatingAggregate(skill.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.ratingAggregate(userID, skill.ID)
}

func (s *SkillHubService) InstallSkill(userID uuid.UUID, deviceID, skillKey string, input InstallSkillInput) (*model.SkillInstallation, error) {
	skill, err := s.repo.GetPublishedSkillByKey(skillKey)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	if skill.PublishedVersionID == nil {
		return nil, fmt.Errorf("%w: published version required", ErrSkillInvalid)
	}
	if existing, err := s.repo.GetInstallationByUserSkill(userID, skill.ID); err == nil {
		wasUninstalled := existing.Status == repository.SkillInstallationStatusUninstalled
		existing.VersionID = *skill.PublishedVersionID
		if strings.TrimSpace(input.TrackMode) != "" {
			existing.TrackMode = normalizeSkillTrackMode(input.TrackMode)
		}
		if input.Config != nil {
			existing.Config = datatypes.JSONMap(input.Config)
		}
		existing.Status = repository.SkillInstallationStatusActive
		existing.DisabledAt = nil
		if err := s.repo.UpdateInstallation(existing); err != nil {
			return nil, err
		}
		if wasUninstalled {
			_ = s.repo.IncrementDownloadCount(skill.ID)
		}
		s.recordInstallationSync(userID, deviceID, existing, skill, SyncOperationInstalled)
		return existing, nil
	}
	track := normalizeSkillTrackMode(input.TrackMode)
	config := datatypes.JSONMap(input.Config)
	if config == nil {
		config = datatypes.JSONMap{}
	}
	installation := &model.SkillInstallation{UserID: userID, SkillID: skill.ID, VersionID: *skill.PublishedVersionID, Status: repository.SkillInstallationStatusActive, InstallSource: "catalog", TrackMode: track, Config: config, InstalledAt: time.Now().UTC()}
	if err := s.repo.CreateInstallation(installation); err != nil {
		return nil, err
	}
	_ = s.repo.IncrementDownloadCount(skill.ID)
	s.recordInstallationSync(userID, deviceID, installation, skill, SyncOperationInstalled)
	s.recordAudit(&userID, deviceID, AuditActionSkillInstalled, "skill_installation", installation.ID.String(), AuditOutcomeSuccess, map[string]any{"skill_key": skill.SkillKey})
	return installation, nil
}

func (s *SkillHubService) ListInstallations(userID uuid.UUID) ([]model.SkillInstallation, error) {
	return s.repo.ListInstallations(userID)
}

func (s *SkillHubService) SetInstallationStatus(userID uuid.UUID, deviceID string, installationID uuid.UUID, status string) (*model.SkillInstallation, error) {
	inst, err := s.repo.GetInstallation(installationID)
	if err != nil || inst.UserID != userID {
		return nil, ErrSkillInstallationFound
	}
	inst.Status = status
	now := time.Now().UTC()
	if status == repository.SkillInstallationStatusDisabled || status == repository.SkillInstallationStatusUninstalled {
		inst.DisabledAt = &now
	} else {
		inst.DisabledAt = nil
	}
	if err := s.repo.UpdateInstallation(inst); err != nil {
		return nil, err
	}
	skill, _ := s.repo.GetSkill(inst.SkillID)
	operation := SyncOperationEnabled
	switch status {
	case repository.SkillInstallationStatusDisabled:
		operation = SyncOperationDisabled
	case repository.SkillInstallationStatusUninstalled:
		operation = SyncOperationUninstalled
	}
	s.recordInstallationSync(userID, deviceID, inst, skill, operation)
	return inst, nil
}

func (s *SkillHubService) UpdateInstallation(userID uuid.UUID, deviceID string, installationID uuid.UUID, input UpdateSkillInstallationInput) (*model.SkillInstallation, error) {
	inst, err := s.repo.GetInstallation(installationID)
	if err != nil || inst.UserID != userID {
		return nil, ErrSkillInstallationFound
	}
	if input.VersionID != nil {
		version, err := s.repo.GetVersion(*input.VersionID)
		if err != nil || version.SkillID != inst.SkillID || version.Status != repository.SkillVersionStatusPublished {
			return nil, ErrSkillVersionNotFound
		}
		inst.VersionID = version.ID
	}
	if strings.TrimSpace(input.TrackMode) != "" {
		inst.TrackMode = normalizeSkillTrackMode(input.TrackMode)
	}
	if input.Config != nil {
		inst.Config = datatypes.JSONMap(input.Config)
	}
	if inst.Config == nil {
		inst.Config = datatypes.JSONMap{}
	}
	if err := s.repo.UpdateInstallation(inst); err != nil {
		return nil, err
	}
	skill, _ := s.repo.GetSkill(inst.SkillID)
	s.recordInstallationSync(userID, deviceID, inst, skill, SyncOperationUpdated)
	return inst, nil
}

func (s *SkillHubService) TakedownSkill(actorID, skillID uuid.UUID, reason string) (*model.Skill, error) {
	skill, err := s.repo.GetSkill(skillID)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	skill.Status = repository.SkillStatusSuspended
	skill.Visibility = repository.SkillVisibilityPrivate
	if err := s.repo.UpdateSkill(skill); err != nil {
		return nil, err
	}
	s.recordAudit(&actorID, "", AuditActionSkillTakedown, "skill", skill.ID.String(), AuditOutcomeSuccess, map[string]any{"reason": reason, "skill_key": skill.SkillKey})
	return skill, nil
}

func (s *SkillHubService) RestrictPublisher(actorID, publisherID uuid.UUID, reason string) (*model.SkillPublisherRestriction, error) {
	if existing, err := s.repo.GetActiveRestriction(publisherID); err == nil {
		return existing, nil
	}
	restriction := &model.SkillPublisherRestriction{PublisherID: publisherID, Status: repository.SkillPublisherRestrictionActive, Reason: strings.TrimSpace(reason), CreatedBy: actorID}
	if err := s.repo.CreateRestriction(restriction); err != nil {
		return nil, err
	}
	s.recordAudit(&actorID, "", AuditActionSkillPublisherRestricted, "skill_publisher", publisherID.String(), AuditOutcomeSuccess, map[string]any{"reason": reason})
	return restriction, nil
}

func (s *SkillHubService) LiftPublisherRestriction(actorID, publisherID uuid.UUID) (*model.SkillPublisherRestriction, error) {
	restriction, err := s.repo.GetActiveRestriction(publisherID)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	now := time.Now().UTC()
	restriction.Status = repository.SkillPublisherRestrictionLifted
	restriction.LiftedBy = &actorID
	restriction.LiftedAt = &now
	if err := s.repo.UpdateRestriction(restriction); err != nil {
		return nil, err
	}
	s.recordAudit(&actorID, "", AuditActionSkillPublisherRestrictionLift, "skill_publisher", publisherID.String(), AuditOutcomeSuccess, nil)
	return restriction, nil
}

func (s *SkillHubService) ensurePublisherAllowed(publisherID uuid.UUID) error {
	if publisherID == uuid.Nil {
		return ErrSkillForbidden
	}
	if _, err := s.repo.GetActiveRestriction(publisherID); err == nil {
		return ErrSkillPublisherBlocked
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func (s *SkillHubService) requireActiveOwnedAsset(ownerID uuid.UUID, objectID uuid.UUID, assetKind string, allowedContentTypes []string) (*model.ObjectRecord, error) {
	if s.objectSvc == nil {
		return nil, fmt.Errorf("%w: object service unavailable", ErrSkillInvalid)
	}
	record, err := s.objectSvc.RequireActiveOwnedObject(ownerID, objectID)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.TrimSpace(record.Scope), "skill-") {
		return nil, fmt.Errorf("%w: %s object scope must start with skill-", ErrSkillInvalid, assetKind)
	}
	for _, allowed := range allowedContentTypes {
		if strings.HasSuffix(allowed, "/") {
			if strings.HasPrefix(record.ContentType, allowed) {
				return record, nil
			}
			continue
		}
		if record.ContentType == allowed {
			return record, nil
		}
	}
	return nil, fmt.Errorf("%w: unsupported %s content type %s", ErrSkillInvalid, assetKind, record.ContentType)
}

func (s *SkillHubService) enrichCatalogSkill(skill *model.Skill) (*SkillCatalogItem, error) {
	item := &SkillCatalogItem{Skill: *skill}
	if skill.PublishedVersionID != nil {
		if version, err := s.repo.GetVersion(*skill.PublishedVersionID); err == nil {
			item.PublishedVersion = version
			if s.objectSvc != nil && version.PackageObjectID != nil {
				if record, err := s.objectSvc.RequireActiveOwnedObject(skill.PublisherID, *version.PackageObjectID); err == nil {
					item.PackageAsset = skillCatalogAssetFromObject(record)
				}
			}
		}
	}
	if s.objectSvc != nil && skill.IconObjectID != nil {
		if record, err := s.objectSvc.RequireActiveOwnedObject(skill.PublisherID, *skill.IconObjectID); err == nil {
			item.IconAsset = skillCatalogAssetFromObject(record)
		}
	}
	return item, nil
}

func skillCatalogAssetFromObject(record *model.ObjectRecord) *SkillCatalogAsset {
	return &SkillCatalogAsset{ObjectID: record.ID, Filename: record.Filename, ContentType: record.ContentType, ContentHash: record.ContentHash, ContentSize: record.ContentSize}
}

func (s *SkillHubService) recordInstallationSync(userID uuid.UUID, deviceID string, inst *model.SkillInstallation, skill *model.Skill, operation string) {
	if s.syncSvc == nil || inst == nil {
		return
	}
	payload := datatypes.JSONMap{"object_id": inst.ID.String(), "installation_id": inst.ID.String(), "skill_id": inst.SkillID.String(), "version_id": inst.VersionID.String(), "track_mode": inst.TrackMode, "status": inst.Status, "config": inst.Config, "updated_by_device_id": deviceID}
	if skill != nil {
		payload["skill_key"] = skill.SkillKey
	}
	_, _ = s.syncSvc.RecordEnvelope(SyncEnvelope{UserID: userID, SourceDeviceID: deviceID, ObjectType: SyncObjectSkill, ObjectID: inst.ID.String(), Operation: operation, ClientEventID: "", Payload: payload})
}

func (s *SkillHubService) recordAudit(actorID *uuid.UUID, actorDeviceID, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	if s.auditSvc == nil {
		return
	}
	_, _ = s.auditSvc.Record(RecordAuditEventInput{ActorUserID: actorID, ActorDeviceID: actorDeviceID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, Metadata: metadata})
}

func (s *SkillHubService) ratingAggregate(userID, skillID uuid.UUID) (*SkillRatingAggregate, error) {
	skill, err := s.repo.GetSkill(skillID)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	agg := &SkillRatingAggregate{SkillID: skill.ID, SkillKey: skill.SkillKey, RatingCount: skill.RatingCount, RatingAverage: skill.RatingAverage, DownloadCount: skill.DownloadCount}
	if rating, err := s.repo.GetRating(skill.ID, userID); err == nil {
		r := rating.Rating
		agg.UserRating = &r
	}
	return agg, nil
}

func normalizeSkillCatalogSort(sort string) string {
	switch strings.TrimSpace(sort) {
	case repository.SkillCatalogSortDownloads:
		return repository.SkillCatalogSortDownloads
	case repository.SkillCatalogSortRating:
		return repository.SkillCatalogSortRating
	case repository.SkillCatalogSortRecent:
		return repository.SkillCatalogSortRecent
	default:
		return repository.SkillCatalogSortRecommended
	}
}

func normalizeSkillTrackMode(track string) string {
	track = strings.TrimSpace(track)
	if track == repository.SkillTrackModePinned {
		return repository.SkillTrackModePinned
	}
	return repository.SkillTrackModeLatest
}

func cleanStrings(values []string) []string {
	out := []string{}
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
