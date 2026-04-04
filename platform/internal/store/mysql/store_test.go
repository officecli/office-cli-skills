package mysqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
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
