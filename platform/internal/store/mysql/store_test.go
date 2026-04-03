package mysqlstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/model"
)

func TestSaveGoogleUserGeneratesStableInviteCode(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
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
