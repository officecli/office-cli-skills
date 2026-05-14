package officesdk

import (
	"context"
	"fmt"
	"time"

	redisstore "github.com/officecli/officecli-internal/platform/internal/store/redis"
)

const (
	fileMetaKeyPrefix       = "officesdk:file:"
	uploadInfoKeyPrefix     = "officesdk:upload:"
	pendingContentKeyPrefix = "officesdk:pending-content:"

	fileMetaTTL       = 7 * 24 * time.Hour
	uploadInfoTTL     = time.Hour
	pendingContentTTL = time.Hour
)

type FileStore struct {
	redis *redisstore.Store
}

func NewFileStore(redis *redisstore.Store) *FileStore {
	return &FileStore{redis: redis}
}

func (s *FileStore) SetFileMeta(ctx context.Context, meta *FileMeta) error {
	if s == nil || s.redis == nil {
		return fmt.Errorf("file store unavailable")
	}
	return s.redis.SetJSON(ctx, fileMetaKeyPrefix+meta.ID, meta, fileMetaTTL)
}

func (s *FileStore) GetFileMeta(ctx context.Context, fileID string) (*FileMeta, error) {
	if s == nil || s.redis == nil {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}
	var meta FileMeta
	ok, err := s.redis.GetJSON(ctx, fileMetaKeyPrefix+fileID, &meta)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}
	return &meta, nil
}

func (s *FileStore) DeleteFileMeta(ctx context.Context, fileID string) error {
	if s == nil || s.redis == nil {
		return fmt.Errorf("file store unavailable")
	}
	return s.redis.Del(ctx, fileMetaKeyPrefix+fileID)
}

func (s *FileStore) SetUploadInfo(ctx context.Context, storageKey string, info *UploadInfo) error {
	if s == nil || s.redis == nil {
		return fmt.Errorf("file store unavailable")
	}
	return s.redis.SetJSON(ctx, uploadInfoKeyPrefix+storageKey, info, uploadInfoTTL)
}

func (s *FileStore) GetUploadInfo(ctx context.Context, storageKey string) (*UploadInfo, error) {
	if s == nil || s.redis == nil {
		return nil, fmt.Errorf("upload info not found: %s", storageKey)
	}
	var info UploadInfo
	ok, err := s.redis.GetJSON(ctx, uploadInfoKeyPrefix+storageKey, &info)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("upload info not found: %s", storageKey)
	}
	return &info, nil
}

func (s *FileStore) SetPendingContentStorageKey(ctx context.Context, fileID, storageKey string) error {
	if s == nil || s.redis == nil {
		return fmt.Errorf("file store unavailable")
	}
	return s.redis.SetString(ctx, pendingContentKeyPrefix+fileID, storageKey, pendingContentTTL)
}

func (s *FileStore) GetPendingContentStorageKey(ctx context.Context, fileID string) (string, error) {
	if s == nil || s.redis == nil {
		return "", fmt.Errorf("pending content not found: %s", fileID)
	}
	return s.redis.GetString(ctx, pendingContentKeyPrefix+fileID)
}
