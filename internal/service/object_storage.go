package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrObjectNotFound      = errors.New("object not found")
	ErrObjectForbidden     = errors.New("object forbidden")
	ErrObjectInvalid       = errors.New("object invalid")
	ErrObjectNotActive     = errors.New("object is not active")
	ErrObjectHashMismatch  = errors.New("object hash mismatch")
	ErrObjectSizeMismatch  = errors.New("object size mismatch")
	ErrObjectTypeMismatch  = errors.New("object content type mismatch")
	ErrObjectHasReferences = errors.New("object has references")
)

type ObjectStorageBackend interface {
	EnsureBucket(ctx context.Context, bucket string) error
	PutObject(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string, metadata map[string]string) error
	PresignedPutURL(ctx context.Context, bucket, key string, ttl time.Duration, contentType string) (string, error)
	PresignedGetURL(ctx context.Context, bucket, key string, ttl time.Duration, disposition string) (string, error)
	HeadObject(ctx context.Context, bucket, key string) (ObjectHead, error)
	ReadObject(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, error)
}

type ObjectHead struct {
	ContentHash string
	ContentSize int64
	ContentType string
}

type ObjectService struct {
	repo               *repository.ObjectRecordsRepo
	storage            ObjectStorageBackend
	cfg                config.ObjectStorageConfig
	governanceEnforcer *GovernanceEnforcer
}

type CreateUploadIntentInput struct {
	Scope       string `json:"scope"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ContentSize int64  `json:"content_size"`
	SHA256      string `json:"sha256"`
}

type UploadIntentResponse struct {
	Object    *model.ObjectRecord `json:"object"`
	UploadURL string              `json:"upload_url"`
	Method    string              `json:"method"`
	ExpiresAt time.Time           `json:"expires_at"`
}

type CompleteUploadInput struct {
	ObservedHash string `json:"observed_hash"`
	ObservedSize int64  `json:"observed_size"`
}

type CreateDownloadURLInput struct {
	Disposition string `json:"disposition"`
}

type DownloadURLResponse struct {
	Object      *model.ObjectRecord `json:"object"`
	DownloadURL string              `json:"download_url"`
	ExpiresAt   time.Time           `json:"expires_at"`
}

type StoreObjectInput struct {
	Scope       string
	Filename    string
	ContentType string
	Content     []byte
}

func NewObjectService(repo *repository.ObjectRecordsRepo, storage ObjectStorageBackend, cfg config.ObjectStorageConfig) *ObjectService {
	return &ObjectService{repo: repo, storage: storage, cfg: cfg}
}

func (s *ObjectService) SetGovernanceEnforcer(enforcer *GovernanceEnforcer) {
	s.governanceEnforcer = enforcer
}

func (s *ObjectService) StoreObject(ctx context.Context, ownerID uuid.UUID, input StoreObjectInput) (*model.ObjectRecord, error) {
	if ownerID == uuid.Nil {
		return nil, ErrObjectForbidden
	}
	if strings.TrimSpace(input.Scope) == "" || strings.Contains(input.Scope, "..") {
		return nil, fmt.Errorf("%w: invalid scope", ErrObjectInvalid)
	}
	if strings.TrimSpace(input.Filename) == "" {
		return nil, fmt.Errorf("%w: filename is required", ErrObjectInvalid)
	}
	if len(input.Content) == 0 {
		return nil, fmt.Errorf("%w: content is required", ErrObjectInvalid)
	}
	if err := s.storage.EnsureBucket(ctx, s.cfg.Bucket); err != nil {
		return nil, err
	}
	recordID := uuid.New()
	sum := sha256.Sum256(input.Content)
	contentHash := hex.EncodeToString(sum[:])
	objectKey := buildObjectKey(ownerID, recordID, input.Scope, input.Filename)
	objectURI := fmt.Sprintf("minio://%s/%s", s.cfg.Bucket, objectKey)
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	metadata := map[string]string{"sha256": contentHash}
	if err := s.storage.PutObject(ctx, s.cfg.Bucket, objectKey, bytes.NewReader(input.Content), int64(len(input.Content)), contentType, metadata); err != nil {
		return nil, err
	}
	now := time.Now()
	record := &model.ObjectRecord{
		Base:        model.Base{ID: recordID},
		OwnerID:     ownerID,
		Scope:       strings.TrimSpace(input.Scope),
		Bucket:      s.cfg.Bucket,
		ObjectKey:   objectKey,
		ObjectURI:   objectURI,
		Filename:    strings.TrimSpace(input.Filename),
		ContentType: contentType,
		ContentHash: contentHash,
		ContentSize: int64(len(input.Content)),
		Status:      repository.ObjectStatusActive,
		CompletedAt: &now,
	}
	if err := s.repo.Create(record); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *ObjectService) CreateUploadIntent(ctx context.Context, ownerID uuid.UUID, input CreateUploadIntentInput) (*UploadIntentResponse, error) {
	if ownerID == uuid.Nil {
		return nil, ErrObjectForbidden
	}
	if err := validateUploadIntentInput(input); err != nil {
		return nil, err
	}
	if err := s.enforceObjectGovernance(ownerID, "upload", input.Scope, strings.TrimSpace(input.Scope), map[string]any{"scope": input.Scope, "filename": input.Filename, "content_type": input.ContentType, "content_size": input.ContentSize}); err != nil {
		return nil, err
	}
	if err := s.storage.EnsureBucket(ctx, s.cfg.Bucket); err != nil {
		return nil, err
	}
	recordID := uuid.New()
	ttl := time.Duration(s.cfg.UploadTTLSecs) * time.Second
	expiresAt := time.Now().Add(ttl)
	objectKey := buildObjectKey(ownerID, recordID, input.Scope, input.Filename)
	objectURI := fmt.Sprintf("minio://%s/%s", s.cfg.Bucket, objectKey)
	uploadURL, err := s.storage.PresignedPutURL(ctx, s.cfg.Bucket, objectKey, ttl, strings.TrimSpace(input.ContentType))
	if err != nil {
		return nil, err
	}
	record := &model.ObjectRecord{
		Base:        model.Base{ID: recordID},
		OwnerID:     ownerID,
		Scope:       strings.TrimSpace(input.Scope),
		Bucket:      s.cfg.Bucket,
		ObjectKey:   objectKey,
		ObjectURI:   objectURI,
		Filename:    strings.TrimSpace(input.Filename),
		ContentType: strings.TrimSpace(input.ContentType),
		ContentHash: strings.ToLower(strings.TrimSpace(input.SHA256)),
		ContentSize: input.ContentSize,
		Status:      repository.ObjectStatusPending,
		ExpiresAt:   &expiresAt,
	}
	if err := s.repo.Create(record); err != nil {
		return nil, err
	}
	return &UploadIntentResponse{Object: record, UploadURL: uploadURL, Method: "PUT", ExpiresAt: expiresAt}, nil
}

func (s *ObjectService) CompleteUpload(ctx context.Context, ownerID uuid.UUID, objectID uuid.UUID, input CompleteUploadInput) (*model.ObjectRecord, error) {
	record, err := s.getOwned(ownerID, objectID)
	if err != nil {
		return nil, err
	}
	if record.Status != repository.ObjectStatusPending {
		return nil, ErrObjectInvalid
	}
	if record.ExpiresAt != nil && time.Now().After(*record.ExpiresAt) {
		return nil, fmt.Errorf("%w: upload intent expired", ErrObjectInvalid)
	}
	head, err := s.storage.HeadObject(ctx, record.Bucket, record.ObjectKey)
	if err != nil {
		return nil, err
	}
	observedHash := firstNonEmpty(strings.ToLower(strings.TrimSpace(input.ObservedHash)), strings.ToLower(strings.TrimSpace(head.ContentHash)))
	observedSize := input.ObservedSize
	if observedSize == 0 {
		observedSize = head.ContentSize
	}
	if observedHash != "" && observedHash != strings.ToLower(record.ContentHash) {
		return nil, ErrObjectHashMismatch
	}
	if observedSize != 0 && observedSize != record.ContentSize {
		return nil, ErrObjectSizeMismatch
	}
	if head.ContentType != "" && record.ContentType != "" && head.ContentType != record.ContentType {
		return nil, ErrObjectTypeMismatch
	}
	if err := s.repo.MarkActive(record.ID); err != nil {
		return nil, err
	}
	return s.repo.GetByIDForOwner(ownerID, objectID)
}

func (s *ObjectService) GetObject(ownerID uuid.UUID, objectID uuid.UUID) (*model.ObjectRecord, error) {
	return s.getOwned(ownerID, objectID)
}

func (s *ObjectService) RequireActiveOwnedObject(ownerID uuid.UUID, objectID uuid.UUID) (*model.ObjectRecord, error) {
	record, err := s.getOwned(ownerID, objectID)
	if err != nil {
		return nil, err
	}
	if record.Status != repository.ObjectStatusActive {
		return nil, ErrObjectNotActive
	}
	return record, nil
}

func (s *ObjectService) CreateDownloadURL(ctx context.Context, ownerID uuid.UUID, objectID uuid.UUID, input CreateDownloadURLInput) (*DownloadURLResponse, error) {
	record, err := s.getOwned(ownerID, objectID)
	if err != nil {
		return nil, err
	}
	if err := s.enforceObjectGovernance(ownerID, "download", record.Scope, record.ID.String(), map[string]any{"object_id": record.ID.String(), "scope": record.Scope, "content_type": record.ContentType}); err != nil {
		return nil, err
	}
	return s.createDownloadURLForRecord(ctx, record, input)
}

func (s *ObjectService) CreateDownloadURLByObjectURI(ctx context.Context, objectURI string, input CreateDownloadURLInput) (*DownloadURLResponse, error) {
	record, err := s.repo.GetByObjectURI(objectURI)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	return s.createDownloadURLForRecord(ctx, record, input)
}

func (s *ObjectService) ReadObjectByObjectURI(ctx context.Context, objectURI string, maxBytes int64) (*model.ObjectRecord, []byte, error) {
	record, err := s.repo.GetByObjectURI(objectURI)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, nil, ErrObjectNotFound
		}
		return nil, nil, err
	}
	if record.Status != repository.ObjectStatusActive {
		return nil, nil, ErrObjectNotActive
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	if record.ContentSize > maxBytes {
		return nil, nil, fmt.Errorf("%w: object exceeds max read size", ErrObjectInvalid)
	}
	content, err := s.storage.ReadObject(ctx, record.Bucket, record.ObjectKey, maxBytes)
	if err != nil {
		return nil, nil, err
	}
	return record, content, nil
}

func (s *ObjectService) createDownloadURLForRecord(ctx context.Context, record *model.ObjectRecord, input CreateDownloadURLInput) (*DownloadURLResponse, error) {
	if record.Status != repository.ObjectStatusActive {
		return nil, ErrObjectNotActive
	}
	disposition := strings.TrimSpace(input.Disposition)
	if disposition == "" {
		disposition = "attachment"
	}
	if disposition != "attachment" && disposition != "inline" {
		return nil, fmt.Errorf("%w: disposition must be attachment or inline", ErrObjectInvalid)
	}
	ttl := time.Duration(s.cfg.DownloadTTLSecs) * time.Second
	expiresAt := time.Now().Add(ttl)
	downloadURL, err := s.storage.PresignedGetURL(ctx, record.Bucket, record.ObjectKey, ttl, disposition)
	if err != nil {
		return nil, err
	}
	return &DownloadURLResponse{Object: record, DownloadURL: downloadURL, ExpiresAt: expiresAt}, nil
}

func (s *ObjectService) DeleteObject(ownerID uuid.UUID, objectID uuid.UUID) (*model.ObjectRecord, error) {
	record, err := s.getOwned(ownerID, objectID)
	if err != nil {
		return nil, err
	}
	if err := s.enforceObjectGovernance(ownerID, "delete", record.Scope, record.ID.String(), map[string]any{"object_id": record.ID.String(), "scope": record.Scope}); err != nil {
		return nil, err
	}
	if record.RefCount > 0 {
		return nil, ErrObjectHasReferences
	}
	if err := s.repo.SoftDelete(record.ID); err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	return s.repo.GetByIDForOwner(ownerID, objectID)
}

func (s *ObjectService) enforceObjectGovernance(ownerID uuid.UUID, operation, scope, subjectID string, context map[string]any) error {
	if s.governanceEnforcer == nil {
		return nil
	}
	actorID := ownerID
	_, err := s.governanceEnforcer.Enforce(GovernanceEnforcementInput{ActorUserID: &actorID, SubjectType: GovernanceSubjectObjectOperation, SubjectID: subjectID, CapabilityKey: objectCapability(operation, scope), RiskLevel: GovernanceRiskMedium, Context: context})
	return err
}

func objectCapability(operation, scope string) string {
	normalized := strings.TrimSpace(scope)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, "/", "_")
	if normalized == "" {
		return "object." + strings.TrimSpace(operation)
	}
	return "object." + strings.TrimSpace(operation) + "." + normalized
}

func (s *ObjectService) getOwned(ownerID uuid.UUID, objectID uuid.UUID) (*model.ObjectRecord, error) {
	if ownerID == uuid.Nil {
		return nil, ErrObjectForbidden
	}
	record, err := s.repo.GetByIDForOwner(ownerID, objectID)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	return record, nil
}

func validateUploadIntentInput(input CreateUploadIntentInput) error {
	if strings.TrimSpace(input.Scope) == "" {
		return fmt.Errorf("%w: scope is required", ErrObjectInvalid)
	}
	if strings.Contains(input.Scope, "..") {
		return fmt.Errorf("%w: invalid scope", ErrObjectInvalid)
	}
	if strings.TrimSpace(input.Filename) == "" {
		return fmt.Errorf("%w: filename is required", ErrObjectInvalid)
	}
	if input.ContentSize <= 0 {
		return fmt.Errorf("%w: content_size must be positive", ErrObjectInvalid)
	}
	if strings.TrimSpace(input.SHA256) == "" {
		return fmt.Errorf("%w: sha256 is required", ErrObjectInvalid)
	}
	if len(strings.TrimSpace(input.SHA256)) != 64 {
		return fmt.Errorf("%w: sha256 must be 64 hex characters", ErrObjectInvalid)
	}
	return nil
}

func buildObjectKey(ownerID uuid.UUID, recordID uuid.UUID, scope, filename string) string {
	safeScope := sanitizePathSegment(scope)
	safeName := sanitizePathSegment(filepath.Base(filename))
	if safeName == "." || safeName == "/" || safeName == "" {
		safeName = "object.bin"
	}
	return fmt.Sprintf("objects/%s/%s/%s/%s", ownerID.String(), safeScope, recordID.String(), safeName)
}

func sanitizePathSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "/")
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == ':' || r == '?' || r == '#' || r == '&'
	})
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." || p == ".." {
			continue
		}
		clean = append(clean, url.PathEscape(p))
	}
	if len(clean) == 0 {
		return "default"
	}
	return strings.Join(clean, "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
