package service

import (
	"context"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/agent-os/backend/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStorageService struct {
	client         *minio.Client
	publicEndpoint string
	useSSL         bool
}

func NewMinIOStorageService(cfg config.ObjectStorageConfig) (*MinIOStorageService, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	return &MinIOStorageService{client: client, publicEndpoint: cfg.PublicEndpoint, useSSL: cfg.UseSSL}, nil
}

func (s *MinIOStorageService) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
}

func (s *MinIOStorageService) PutObject(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string, metadata map[string]string) error {
	_, err := s.client.PutObject(ctx, bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType, UserMetadata: metadata})
	return err
}

func (s *MinIOStorageService) PresignedPutURL(ctx context.Context, bucket, key string, ttl time.Duration, contentType string) (string, error) {
	_ = contentType
	u, err := s.client.PresignedPutObject(ctx, bucket, key, ttl)
	if err != nil {
		return "", err
	}
	return s.rewritePublicEndpoint(u).String(), nil
}

func (s *MinIOStorageService) PresignedGetURL(ctx context.Context, bucket, key string, ttl time.Duration, disposition string) (string, error) {
	params := make(url.Values)
	if strings.TrimSpace(disposition) != "" {
		params.Set("response-content-disposition", strings.TrimSpace(disposition))
	}
	u, err := s.client.PresignedGetObject(ctx, bucket, key, ttl, params)
	if err != nil {
		return "", err
	}
	return s.rewritePublicEndpoint(u).String(), nil
}

func (s *MinIOStorageService) HeadObject(ctx context.Context, bucket, key string) (ObjectHead, error) {
	info, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectHead{}, err
	}
	contentHash := info.UserMetadata["X-Amz-Meta-Sha256"]
	if contentHash == "" {
		contentHash = info.UserMetadata["x-amz-meta-sha256"]
	}
	return ObjectHead{
		ContentHash: strings.Trim(contentHash, "\""),
		ContentSize: info.Size,
		ContentType: info.ContentType,
	}, nil
}

func (s *MinIOStorageService) ReadObject(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, error) {
	object, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	return io.ReadAll(io.LimitReader(object, maxBytes+1))
}

func (s *MinIOStorageService) rewritePublicEndpoint(u *url.URL) *url.URL {
	if strings.TrimSpace(s.publicEndpoint) == "" {
		return u
	}
	out := *u
	endpoint := strings.TrimPrefix(strings.TrimPrefix(s.publicEndpoint, "https://"), "http://")
	out.Host = endpoint
	if s.useSSL {
		out.Scheme = "https"
	} else {
		out.Scheme = "http"
	}
	return &out
}

func mergeQuery(base url.Values, extra url.Values) url.Values {
	for key, values := range extra {
		for _, value := range values {
			base.Add(key, value)
		}
	}
	return base
}

var _ ObjectStorageBackend = (*MinIOStorageService)(nil)
