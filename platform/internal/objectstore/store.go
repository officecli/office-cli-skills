package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint        string
	AccessKey       string
	SecretKey       string
	Bucket          string
	UseSSL          bool
}

type Store struct {
	client *minio.Client
	bucket string
}

func New(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("PREVIEW_OBJECT_ENDPOINT is required")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" {
		return nil, fmt.Errorf("PREVIEW_OBJECT_ACCESS_KEY is required")
	}
	if strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, fmt.Errorf("PREVIEW_OBJECT_SECRET_KEY is required")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("PREVIEW_OBJECT_BUCKET is required")
	}
	client, err := minio.New(strings.TrimSpace(cfg.Endpoint), &minio.Options{
		Creds:  credentials.NewStaticV4(strings.TrimSpace(cfg.AccessKey), strings.TrimSpace(cfg.SecretKey), ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	store := &Store{client: client, bucket: strings.TrimSpace(cfg.Bucket)}
	if err := store.ensureBucket(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

func (s *Store) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, strings.TrimSpace(key), reader, size, minio.PutObjectOptions{ContentType: strings.TrimSpace(contentType)})
	return err
}

func (s *Store) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, s.bucket, strings.TrimSpace(key), minio.GetObjectOptions{})
}

func (s *Store) DeleteObject(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, strings.TrimSpace(key), minio.RemoveObjectOptions{})
}

func (s *Store) PresignPutURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	value, err := s.client.PresignedPutObject(ctx, s.bucket, strings.TrimSpace(key), expiry)
	if err != nil {
		return "", err
	}
	return value.String(), nil
}

func (s *Store) PresignGetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	value, err := s.client.PresignedGetObject(ctx, s.bucket, strings.TrimSpace(key), expiry, url.Values{})
	if err != nil {
		return "", err
	}
	return value.String(), nil
}
