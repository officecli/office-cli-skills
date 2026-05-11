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

func TestCreateAPIKeyPersistsHostedOnlyFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:create_hosted_only_key?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)

	allowedModes := "hosted_only"
	hostedEnabled := true
	defaultRuntimeMode := "hosted"
	creditBalance := 120
	result, key, err := svc.CreateAPIKey(context.Background(), CreateAPIKeyRequest{
		PlanName:           "Hosted",
		AllowedModes:       &allowedModes,
		HostedEnabled:      &hostedEnabled,
		DefaultRuntimeMode: &defaultRuntimeMode,
		CreditBalance:      &creditBalance,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.PlaintextKey)

	saved, err := store.FindAPIKeyByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.Equal(t, "hosted_only", saved.AllowedModes)
	require.True(t, saved.HostedEnabled)
	require.NotNil(t, saved.DefaultRuntimeMode)
	require.Equal(t, "hosted", *saved.DefaultRuntimeMode)
	require.Equal(t, 120, saved.CreditBalance)
	require.Nil(t, saved.QuotaTotal)
}

func TestUpdateAPIKeyPersistsHostedEntitlementFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:update_hosted_entitlement_fields?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	defaultRuntimeMode := "external"
	key := &model.APIKey{
		KeyHash:            "hash-hosted-update",
		KeyPrefix:          "cop_hosted_update",
		Status:             model.APIKeyStatusActive,
		PlanName:           "External",
		AllowedModes:       "external_only",
		HostedEnabled:      false,
		DefaultRuntimeMode: &defaultRuntimeMode,
	}
	require.NoError(t, db.Create(key).Error)

	allowedModes := "hosted_only"
	hostedEnabled := true
	hostedRuntime := "hosted"
	creditBalance := 75
	require.NoError(t, svc.UpdateAPIKey(context.Background(), key.ID, UpdateAPIKeyRequest{
		AllowedModes:       &allowedModes,
		HostedEnabled:      &hostedEnabled,
		DefaultRuntimeMode: &hostedRuntime,
		CreditBalance:      &creditBalance,
	}))

	saved, err := store.FindAPIKeyByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.Equal(t, "hosted_only", saved.AllowedModes)
	require.True(t, saved.HostedEnabled)
	require.NotNil(t, saved.DefaultRuntimeMode)
	require.Equal(t, "hosted", *saved.DefaultRuntimeMode)
	require.Equal(t, 75, saved.CreditBalance)
}

func TestUpdateUserDisablesOwnedAPIKeysWhenUserIsDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:update_user_disables_owned_api_keys?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	user := &model.User{
		GoogleSub:  "user-sub",
		Email:      "demo@example.com",
		Name:       "Demo",
		InviteCode: "invite-000001",
		Status:     model.UserStatusActive,
	}
	require.NoError(t, db.Create(user).Error)

	otherUserID := uint64(999)
	activeQuota := 10
	keys := []model.APIKey{
		{OwnerUserID: &user.ID, KeyHash: "hash-1", KeyPrefix: "cop_live_1", Status: model.APIKeyStatusActive, PlanName: "Starter", QuotaTotal: &activeQuota},
		{OwnerUserID: &user.ID, KeyHash: "hash-2", KeyPrefix: "cop_live_2", Status: model.APIKeyStatusDisabled, PlanName: "Starter", QuotaTotal: &activeQuota},
		{OwnerUserID: &otherUserID, KeyHash: "hash-3", KeyPrefix: "cop_live_3", Status: model.APIKeyStatusActive, PlanName: "Starter", QuotaTotal: &activeQuota},
	}
	for i := range keys {
		require.NoError(t, db.Create(&keys[i]).Error)
	}

	status := string(model.UserStatusDisabled)
	require.NoError(t, svc.UpdateUser(context.Background(), user.ID, UpdateUserRequest{Status: &status}))

	savedUser, err := store.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusDisabled, savedUser.Status)

	var owned []model.APIKey
	require.NoError(t, db.Where("owner_user_id = ?", user.ID).Order("id asc").Find(&owned).Error)
	require.Len(t, owned, 2)
	require.Equal(t, model.APIKeyStatusDisabled, owned[0].Status)
	require.Equal(t, model.APIKeyStatusDisabled, owned[1].Status)

	var other model.APIKey
	require.NoError(t, db.Where("key_hash = ?", "hash-3").First(&other).Error)
	require.Equal(t, model.APIKeyStatusActive, other.Status)
}

func TestUpdateUserReEnableDoesNotRestoreDisabledAPIKeys(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:update_user_reenable_does_not_restore_disabled_api_keys?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	user := &model.User{
		GoogleSub:  "user-sub",
		Email:      "demo@example.com",
		Name:       "Demo",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(user).Error)

	quota := 10
	key := &model.APIKey{
		OwnerUserID: &user.ID,
		KeyHash:     "hash-1",
		KeyPrefix:   "cop_live_1",
		Status:      model.APIKeyStatusDisabled,
		PlanName:    "Starter",
		QuotaTotal:  &quota,
	}
	require.NoError(t, db.Create(key).Error)

	status := string(model.UserStatusActive)
	require.NoError(t, svc.UpdateUser(context.Background(), user.ID, UpdateUserRequest{Status: &status}))

	savedUser, err := store.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusActive, savedUser.Status)

	savedKey, err := store.FindAPIKeyByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.Equal(t, model.APIKeyStatusDisabled, savedKey.Status)
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
