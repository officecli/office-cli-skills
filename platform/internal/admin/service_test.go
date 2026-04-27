package admin

import (
	"context"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/apikey"
	"github.com/officecli/officecli/platform/internal/model"
	redisstore "github.com/officecli/officecli/platform/internal/store/redis"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type fakeCodec struct{}

func (fakeCodec) Encode(v string) (string, error) { return v, nil }
func (fakeCodec) Decode(v string) (string, error) { return v, nil }

type memoryRedis struct{ data map[string]any }

func TestNewServiceNormalizesAdminAllowlist(t *testing.T) {
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(nil, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, []string{
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
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(nil, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, []string{
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
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.DailyFreeQuota{}, &model.UsageEvent{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	_ = client
	redisRepo := redisstore.NewStore(redis.NewClient(&redis.Options{Addr: "localhost:6379"}))
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, redisRepo, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)

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
	require.True(t, keys[0].PlaintextAvailable)
	require.Equal(t, model.APIKeyStatusActive, keys[0].Status)
	require.Equal(t, "pro", keys[0].PlanName)
	require.NotNil(t, keys[0].QuotaTotal)
	require.Equal(t, 20, *keys[0].QuotaTotal)

	plaintext, err := svc.GetAPIKeyPlaintext(context.Background(), key.ID, "admin@example.com")
	require.NoError(t, err)
	require.Equal(t, result.PlaintextKey, plaintext)

	quota := &model.DailyFreeQuota{FingerprintHash: "fp-admin", UsageDate: "2026-04-16", DailyLimit: 2, DailyUsed: 1}
	require.NoError(t, db.Create(quota).Error)
	require.NoError(t, svc.UpdateFreeQuota(context.Background(), quota.ID, 5))

	var updated model.DailyFreeQuota
	require.NoError(t, db.First(&updated, quota.ID).Error)
	require.Equal(t, 5, updated.DailyLimit)
}

func TestGetAPIKeyPlaintextRejectsLegacyRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, redisstore.NewStore(redis.NewClient(&redis.Options{Addr: "localhost:6379"})), "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)

	key := &model.APIKey{KeyHash: "legacy-hash", KeyPrefix: "cop_legacy", Status: model.APIKeyStatusActive, PlanName: "Legacy"}
	require.NoError(t, store.CreateAPIKey(context.Background(), key))

	_, err = svc.GetAPIKeyPlaintext(context.Background(), key.ID, "admin@example.com")
	require.ErrorIs(t, err, apikey.ErrPlaintextUnavailable)
}
