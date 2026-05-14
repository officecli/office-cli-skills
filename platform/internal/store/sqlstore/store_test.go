package sqlstore

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	growthsvc "github.com/officecli/officecli-internal/platform/internal/growth"
	"github.com/officecli/officecli-internal/platform/internal/model"
)

func TestSaveGoogleUserGeneratesStableInviteCode(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:save_google_user_generates_stable_invite_code?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	store := NewWithDB(db)

	user, err := store.SaveGoogleUser(context.Background(), "google-sub-1", "demo@example.com", "Demo User", nil)
	require.NoError(t, err)
	require.Equal(t, "invite-000001", user.InviteCode)

	saved, err := store.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "invite-000001", saved.InviteCode)

	updated, err := store.SaveGoogleUser(context.Background(), "google-sub-1", "demo@example.com", "Renamed User", nil)
	require.NoError(t, err)
	require.Equal(t, user.ID, updated.ID)
	require.Equal(t, "invite-000001", updated.InviteCode)
	require.Equal(t, "Renamed User", updated.Name)
}

func TestSaveGoogleUserBackfillsMissingInviteCode(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:save_google_user_backfills_missing_invite_code?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	legacy := &model.User{
		GoogleSub:  "legacy-sub",
		Email:      "legacy@example.com",
		Name:       "Legacy User",
		InviteCode: "",
		Status:     model.UserStatusActive,
	}
	require.NoError(t, db.Create(legacy).Error)

	store := NewWithDB(db)
	user, err := store.SaveGoogleUser(context.Background(), "legacy-sub", "legacy@example.com", "Legacy User", nil)
	require.NoError(t, err)
	require.Equal(t, buildInviteCode(legacy.ID), user.InviteCode)
}

func TestSaveGoogleUserKeepsDisabledStatusForExistingUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:save_google_user_keeps_disabled_status?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	legacy := &model.User{
		GoogleSub:  "legacy-sub",
		Email:      "legacy@example.com",
		Name:       "Legacy User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(legacy).Error)

	store := NewWithDB(db)
	user, err := store.SaveGoogleUser(context.Background(), "legacy-sub", "legacy@example.com", "Renamed User", nil)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusDisabled, user.Status)

	saved, err := store.GetUserByID(context.Background(), legacy.ID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusDisabled, saved.Status)
	require.Equal(t, "Renamed User", saved.Name)
}

func TestListUsersFiltersByIDAndEmail(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:list_users_filters_by_id_and_email?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	users := []model.User{
		{GoogleSub: "sub-1", Email: "alpha@example.com", Name: "Alpha", InviteCode: "invite-alpha", Status: model.UserStatusActive},
		{GoogleSub: "sub-2", Email: "beta@example.com", Name: "Beta", InviteCode: "invite-beta", Status: model.UserStatusActive},
		{GoogleSub: "sub-3", Email: "ops@example.org", Name: "Ops", InviteCode: "invite-ops", Status: model.UserStatusActive},
	}
	for i := range users {
		require.NoError(t, db.Create(&users[i]).Error)
	}

	store := NewWithDB(db)
	all, err := store.ListUsers(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, all, 3)

	byID, err := store.ListUsers(context.Background(), strconv.FormatUint(users[1].ID, 10))
	require.NoError(t, err)
	require.Len(t, byID, 1)
	require.Equal(t, "beta@example.com", byID[0].Email)

	byEmail, err := store.ListUsers(context.Background(), "example.com")
	require.NoError(t, err)
	require.Len(t, byEmail, 2)
	require.Equal(t, "beta@example.com", byEmail[0].Email)
	require.Equal(t, "alpha@example.com", byEmail[1].Email)
}

func TestCountReferralsByInviterUserID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:count_referrals_by_inviter_user_id?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserReferral{}))

	store := NewWithDB(db)
	require.NoError(t, db.Create(&model.UserReferral{InviterUserID: 7, InvitedUserID: 100, InviteCode: "invite-007"}).Error)
	require.NoError(t, db.Create(&model.UserReferral{InviterUserID: 7, InvitedUserID: 101, InviteCode: "invite-007"}).Error)
	require.NoError(t, db.Create(&model.UserReferral{InviterUserID: 8, InvitedUserID: 102, InviteCode: "invite-008"}).Error)

	count, err := store.CountReferralsByInviterUserID(context.Background(), 7)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
}

func TestRegisterReferralWithinLimitHonorsCapAndIdempotency(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:register_referral_within_limit?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserReferral{}))

	store := NewWithDB(db)
	inviter := &model.User{
		GoogleSub:  "google-sub-1",
		Email:      "demo@example.com",
		Name:       "Demo User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusActive,
	}
	require.NoError(t, db.Create(inviter).Error)

	registeredAt := time.Now().UTC()
	for i := uint64(0); i < 5; i++ {
		referral, err := store.RegisterReferralWithinLimit(context.Background(), inviter.ID, 100+i, inviter.InviteCode, registeredAt)
		require.NoError(t, err)
		require.Equal(t, inviter.ID, referral.InviterUserID)
	}

	idempotent, err := store.RegisterReferralWithinLimit(context.Background(), inviter.ID, 100, inviter.InviteCode, registeredAt.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, uint64(100), idempotent.InvitedUserID)

	_, err = store.RegisterReferralWithinLimit(context.Background(), inviter.ID, 999, inviter.InviteCode, registeredAt.Add(2*time.Minute))
	require.ErrorIs(t, err, growthsvc.ErrInviteLimitReached)

	count, err := store.CountReferralsByInviterUserID(context.Background(), inviter.ID)
	require.NoError(t, err)
	require.EqualValues(t, 5, count)
}

func TestAppCreateAPIKeyUsesLeastPrivilegeDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:app_create_api_key_defaults?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}))

	store := NewWithDB(db)
	key, err := store.AppCreateAPIKey(context.Background(), 42, "Starter", "hash-1", "cop_test", "ciphertext-1")
	require.NoError(t, err)
	require.Equal(t, "external_only", key.AllowedModes)
	require.False(t, key.HostedEnabled)
	require.NotNil(t, key.DefaultRuntimeMode)
	require.Equal(t, "external", *key.DefaultRuntimeMode)
	require.NotNil(t, key.KeyCiphertext)
	require.Equal(t, "ciphertext-1", *key.KeyCiphertext)
}

func TestAdminCreateAPIKeyUsesLeastPrivilegeDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:admin_create_api_key_defaults?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}))

	store := NewWithDB(db)
	key, err := store.AdminCreateAPIKey(context.Background(), nil, "Ops", nil, nil, nil, "hash-2", "cop_admin", "ciphertext-2", nil)
	require.NoError(t, err)
	require.Equal(t, "external_only", key.AllowedModes)
	require.False(t, key.HostedEnabled)
	require.NotNil(t, key.DefaultRuntimeMode)
	require.Equal(t, "external", *key.DefaultRuntimeMode)
	require.NotNil(t, key.KeyCiphertext)
	require.Equal(t, "ciphertext-2", *key.KeyCiphertext)
}

func TestAddCreditBalanceToAPIKeyEnablesHostedEntitlement(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:add_credit_balance_enables_hosted?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}))

	store := NewWithDB(db)
	defaultRuntimeMode := "external"
	key := &model.APIKey{
		KeyHash:            "hash-3",
		KeyPrefix:          "cop_credit",
		Status:             model.APIKeyStatusActive,
		PlanName:           "Starter",
		AllowedModes:       "external_only",
		HostedEnabled:      false,
		DefaultRuntimeMode: &defaultRuntimeMode,
		CreditBalance:      0,
	}
	require.NoError(t, store.CreateAPIKey(context.Background(), key))

	updated, err := store.AddCreditBalanceToAPIKey(context.Background(), key.ID, 300)
	require.NoError(t, err)
	require.Equal(t, 300, updated.CreditBalance)
	require.True(t, updated.HostedEnabled)
	require.Equal(t, "hybrid", updated.AllowedModes)
	require.NotNil(t, updated.DefaultRuntimeMode)
	require.Equal(t, "hosted", *updated.DefaultRuntimeMode)
}

func TestGrantHostedCreditsToAPIKeyIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:grant_hosted_credits_idempotent?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.HostedCreditGrant{}))

	store := NewWithDB(db)
	user := &model.User{GoogleSub: "sub-hosted", Email: "hosted@example.com", Name: "Hosted", InviteCode: "invite-hosted", Status: model.UserStatusActive}
	require.NoError(t, db.Create(user).Error)
	defaultRuntimeMode := "external"
	key := &model.APIKey{
		OwnerUserID:        &user.ID,
		KeyHash:            "hash-hosted-grant",
		KeyPrefix:          "cop_hosted_grant",
		Status:             model.APIKeyStatusActive,
		PlanName:           "Starter",
		AllowedModes:       "external_only",
		HostedEnabled:      false,
		DefaultRuntimeMode: &defaultRuntimeMode,
	}
	require.NoError(t, store.CreateAPIKey(context.Background(), key))

	grant, created, err := store.GrantHostedCreditsToAPIKey(context.Background(), key.ID, user.ID, model.HostedCreditGrantSourceSignup, "signup-hosted-credits:1", 30, "new user signup hosted credits", "{}")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, 30, grant.CreditAmount)

	grant, created, err = store.GrantHostedCreditsToAPIKey(context.Background(), key.ID, user.ID, model.HostedCreditGrantSourceSignup, "signup-hosted-credits:1", 30, "new user signup hosted credits", "{}")
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, 30, grant.CreditAmount)

	updated, err := store.FindAPIKeyByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.Equal(t, 30, updated.CreditBalance)
	require.True(t, updated.HostedEnabled)
	require.Equal(t, "hybrid", updated.AllowedModes)
	require.NotNil(t, updated.DefaultRuntimeMode)
	require.Equal(t, "hosted", *updated.DefaultRuntimeMode)
}

func TestFindAPIKeyByHashTreatsDisabledOwnerAsDisabledKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:find_api_key_by_hash_treats_disabled_owner_as_disabled_key?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}))

	store := NewWithDB(db)
	user := &model.User{
		GoogleSub:  "google-sub-1",
		Email:      "demo@example.com",
		Name:       "Demo User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(user).Error)

	quotaTotal := 10
	key := &model.APIKey{
		OwnerUserID: &user.ID,
		KeyHash:     "hash-owner-disabled",
		KeyPrefix:   "cop_live_disabled",
		Status:      model.APIKeyStatusActive,
		PlanName:    "Starter",
		QuotaTotal:  &quotaTotal,
	}
	require.NoError(t, db.Create(key).Error)

	saved, err := store.FindAPIKeyByHash(context.Background(), key.KeyHash)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, model.APIKeyStatusDisabled, saved.Status)
}

func TestConsumePaidQuotaByHashRejectsDisabledOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:consume_paid_quota_by_hash_rejects_disabled_owner?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}))

	store := NewWithDB(db)
	user := &model.User{
		GoogleSub:  "google-sub-1",
		Email:      "demo@example.com",
		Name:       "Demo User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(user).Error)

	quotaTotal := 10
	key := &model.APIKey{
		OwnerUserID: &user.ID,
		KeyHash:     "hash-owner-disabled",
		KeyPrefix:   "cop_live_disabled",
		Status:      model.APIKeyStatusActive,
		PlanName:    "Starter",
		QuotaTotal:  &quotaTotal,
	}
	require.NoError(t, db.Create(key).Error)

	_, err = store.ConsumePaidQuotaByHash(context.Background(), key.KeyHash)
	require.Error(t, err)
	require.Contains(t, err.Error(), "api key is disabled")

	var saved model.APIKey
	require.NoError(t, db.First(&saved, key.ID).Error)
	require.Equal(t, 0, saved.QuotaUsed)
}

func TestReserveCreditsByHashRejectsDisabledOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:reserve_credits_by_hash_rejects_disabled_owner?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}))

	store := NewWithDB(db)
	user := &model.User{
		GoogleSub:  "google-sub-1",
		Email:      "demo@example.com",
		Name:       "Demo User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(user).Error)

	defaultRuntimeMode := "hosted"
	key := &model.APIKey{
		OwnerUserID:        &user.ID,
		KeyHash:            "hash-hosted-disabled",
		KeyPrefix:          "cop_live_hosted",
		Status:             model.APIKeyStatusActive,
		PlanName:           "Starter",
		AllowedModes:       "hybrid",
		HostedEnabled:      true,
		DefaultRuntimeMode: &defaultRuntimeMode,
		CreditBalance:      20,
	}
	require.NoError(t, db.Create(key).Error)

	_, err = store.ReserveCreditsByHash(context.Background(), key.KeyHash, 5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "api key is disabled")

	var saved model.APIKey
	require.NoError(t, db.First(&saved, key.ID).Error)
	require.Equal(t, 0, saved.CreditReserved)
}

func TestUserAIGatewayAPIKeyLifecycleIsUniquePerUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:user_aigateway_api_key_lifecycle?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserAIGatewayAPIKey{}))

	store := NewWithDB(db)
	ctx := context.Background()

	creating, err := store.ClaimUserAIGatewayAPIKeyCreation(ctx, 42, "officecli-user-42")
	require.NoError(t, err)
	require.Equal(t, uint64(42), creating.UserID)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusCreating, creating.Status)
	require.Equal(t, "officecli-user-42", creating.UpstreamName)
	require.True(t, creating.CreationClaimed)

	duplicate, err := store.ClaimUserAIGatewayAPIKeyCreation(ctx, 42, "officecli-user-42")
	require.NoError(t, err)
	require.Equal(t, creating.ID, duplicate.ID)
	require.False(t, duplicate.CreationClaimed)

	active, err := store.ActivateUserAIGatewayAPIKey(ctx, 42, "ciphertext-value", "sk-user", "upstream-123", "officecli-user-42")
	require.NoError(t, err)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusActive, active.Status)
	require.Equal(t, "ciphertext-value", active.KeyCiphertext)
	require.Equal(t, "sk-user", active.KeyPrefix)
	require.Equal(t, "upstream-123", active.UpstreamID)
	require.Empty(t, active.LastError)

	found, err := store.FindUserAIGatewayAPIKeyByUserID(ctx, 42)
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, active.ID, found.ID)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusActive, found.Status)
	require.Equal(t, "ciphertext-value", found.KeyCiphertext)
}

func TestUserAIGatewayAPIKeyCreationErrorCanBeRetried(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:user_aigateway_api_key_error_retry?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserAIGatewayAPIKey{}))

	store := NewWithDB(db)
	ctx := context.Background()

	_, err = store.ClaimUserAIGatewayAPIKeyCreation(ctx, 42, "officecli-user-42")
	require.NoError(t, err)

	failed, err := store.MarkUserAIGatewayAPIKeyCreationError(ctx, 42, "officecli-user-42", "gateway unavailable")
	require.NoError(t, err)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusError, failed.Status)
	require.Equal(t, "gateway unavailable", failed.LastError)

	retry, err := store.ClaimUserAIGatewayAPIKeyCreation(ctx, 42, "officecli-user-42")
	require.NoError(t, err)
	require.Equal(t, failed.ID, retry.ID)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusCreating, retry.Status)
	require.True(t, retry.CreationClaimed)
	require.Empty(t, retry.LastError)
}
