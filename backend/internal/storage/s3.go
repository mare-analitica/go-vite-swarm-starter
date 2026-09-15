// Package storage issues presigned S3 URLs for note attachments.
//
// Only the S3 API is used, so any S3-compatible server works (MinIO, Garage,
// SeaweedFS, AWS S3). Browsers upload and download directly with presigned
// requests; file bytes never pass through the API.
package storage

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/config"
)

// Upload describes a presigned POST: the browser sends a multipart form with
// Fields plus the file to URL. The policy enforces the exact key and size.
type Upload struct {
	URL    string            `json:"url"`
	Fields map[string]string `json:"fields"`
	Key    string            `json:"key"`
}

// S3 talks to the bucket through the internal endpoint and signs browser
// requests for the public endpoint.
type S3 struct {
	internal  *minio.Client
	public    *minio.Client
	bucket    string
	maxUpload int64
	expiry    time.Duration
}

// New creates the internal and presigning clients. Presigning is local: with
// the region set, no network call is made to the public endpoint.
func New(cfg config.S3) (*S3, error) {
	if cfg.PublicEndpoint == nil {
		return nil, errors.New("public endpoint is required")
	}
	if cfg.PublicEndpoint.Path != "" && cfg.PublicEndpoint.Path != "/" {
		return nil, fmt.Errorf("public endpoint must not have a path: %s", cfg.PublicEndpoint)
	}
	creds := credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")

	internal, err := minio.New(cfg.Endpoint, &minio.Options{Creds: creds, Secure: cfg.UseSSL, Region: cfg.Region})
	if err != nil {
		return nil, fmt.Errorf("internal client: %w", err)
	}
	public, err := minio.New(cfg.PublicEndpoint.Host, &minio.Options{
		Creds:  creds,
		Secure: cfg.PublicEndpoint.Scheme == "https",
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("public client: %w", err)
	}
	return &S3{internal: internal, public: public, bucket: cfg.Bucket, maxUpload: cfg.MaxUploadBytes, expiry: cfg.URLExpiry}, nil
}

// Ping verifies the bucket exists and the credentials can reach it.
func (s *S3) Ping(ctx context.Context) error {
	ok, err := s.internal.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bucket %q does not exist", s.bucket)
	}
	return nil
}

// PresignUpload returns a presigned POST limited to key and to 1..maxUpload bytes.
func (s *S3) PresignUpload(ctx context.Context, key string) (Upload, error) {
	policy := minio.NewPostPolicy()
	if err := policy.SetBucket(s.bucket); err != nil {
		return Upload{}, err
	}
	if err := policy.SetKey(key); err != nil {
		return Upload{}, err
	}
	if err := policy.SetExpires(time.Now().UTC().Add(s.expiry)); err != nil {
		return Upload{}, err
	}
	if err := policy.SetContentLengthRange(1, s.maxUpload); err != nil {
		return Upload{}, err
	}
	u, fields, err := s.public.PresignedPostPolicy(ctx, policy)
	if err != nil {
		return Upload{}, err
	}
	return Upload{URL: u.String(), Fields: fields, Key: key}, nil
}

// PresignDownload returns a presigned GET that downloads the object as filename.
func (s *S3) PresignDownload(ctx context.Context, key, filename string) (string, error) {
	params := url.Values{}
	params.Set("response-content-disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	u, err := s.public.PresignedGetObject(ctx, s.bucket, key, s.expiry, params)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Exists reports whether an object is present in the bucket.
func (s *S3) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.internal.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return false, nil
	}
	return false, err
}

// Delete removes an object; deleting a missing object is not an error in S3.
func (s *S3) Delete(ctx context.Context, key string) error {
	return s.internal.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// MaxUploadBytes is the upload size limit enforced by the policy.
func (s *S3) MaxUploadBytes() int64 { return s.maxUpload }
