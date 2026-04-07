package admin

import (
	"context"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/model"
	redisstore "github.com/officecli/officecli/platform/internal/store/redis"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type fakeCodec struct{}

func (fakeCodec) Encode(v string) (string, error) { return v, nil }
func (fakeCodec) Decode(v string) (string, error) { return v, nil }

type memoryRedis struct{ data map[string]any }

func TestNewServiceNormalizesAdminAllowlist(t *testing.T) {
	svc := NewService(nil, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, []string{
		"  LUYANG950@GMAIL.COM  ",
		"",
		"luyang950@gmail.com",
	})

	require.Len(t, svc.adminAllowlist, 1)
	_, ok := svc.adminAllowlist["luyang950@gmail.com"]
	require.True(t, ok)
	_, ok = svc.adminAllowlist["LUYANG950@GMAIL.COM"]
	require.False(t, ok)
}

func TestNewServiceRejectsNonAllowlistedEmailAfterNormalization(t *testing.T) {
	svc := NewService(nil, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, []string{
		" luyang950@gmail.com ",
	})

	_, allowed := svc.adminAllowlist["someone@example.com"]
	require.False(t, allowed)
	_, allowed = svc.adminAllowlist["luyang950@gmail.com"]
	require.True(t, allowed)
}

func TestCreateKeyAndUpdateQuota(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.FreeQuota{}, &model.UsageEvent{}, &model.AdminAuditLog{}))
	mysqlRepo := sqlstore.NewWithDB(db)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	_ = client
	redisRepo := redisstore.NewStore(redis.NewClient(&redis.Options{Addr: "localhost:6379"}))
	svc := NewService(mysqlRepo, redisRepo, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil)

	note := "created by test"
	quotaTotal := 20
	result, key, err := svc.CreateAPIKey(context.Background(), CreateAPIKeyRequest{PlanName: "pro", Note: &note, QuotaTotal: &quotaTotal})
	require.NoError(t, err)
	require.NotEmpty(t, result.PlaintextKey)
	require.NotEmpty(t, result.KeyPrefix)

	keys, err := svc.ListAPIKeys(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, key.KeyPrefix, keys[0].KeyPrefix)
	require.Equal(t, model.APIKeyStatusActive, keys[0].Status)
	require.Equal(t, "pro", keys[0].PlanName)
	require.NotNil(t, keys[0].QuotaTotal)
	require.Equal(t, 20, *keys[0].QuotaTotal)

	quota := &model.FreeQuota{FingerprintHash: "fp-admin", FreeLimit: 2, FreeUsed: 1}
	require.NoError(t, db.Create(quota).Error)
	require.NoError(t, svc.UpdateFreeQuota(context.Background(), quota.ID, 5))

	var updated model.FreeQuota
	require.NoError(t, db.First(&updated, quota.ID).Error)
	require.Equal(t, 5, updated.FreeLimit)
}
