package appuser

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
)

type fakeStore struct {
	owned          bool
	updateValues   map[string]any
	updateCalls    int
	auditLogCalls  int
	apiKeysByOwner []model.APIKey
	user           *model.User
	rewardGrants   []model.RewardGrant
	referrals      []model.UserReferral
	discord        *model.DiscordConnection
}

func (f *fakeStore) CountUserAPIKeys(_ context.Context, _ uint64) (int64, error) {
	return int64(len(f.apiKeysByOwner)), nil
}
func (f *fakeStore) FindAPIKeysByOwner(_ context.Context, _ uint64) ([]model.APIKey, error) {
	return f.apiKeysByOwner, nil
}
func (f *fakeStore) ListAppUsageEvents(_ context.Context, _ uint64) ([]model.UsageEvent, error) {
	return nil, nil
}
func (f *fakeStore) GetUserByID(_ context.Context, _ uint64) (*model.User, error) {
	return f.user, nil
}
func (f *fakeStore) ListRewardGrantsByUser(_ context.Context, _ uint64) ([]model.RewardGrant, error) {
	return f.rewardGrants, nil
}
func (f *fakeStore) ListReferralsByInviterUserID(_ context.Context, _ uint64) ([]model.UserReferral, error) {
	return f.referrals, nil
}
func (f *fakeStore) FindDiscordConnectionByUserID(_ context.Context, _ uint64) (*model.DiscordConnection, error) {
	return f.discord, nil
}
func (f *fakeStore) CreateAuditLog(_ context.Context, _, _, _, _ string) error {
	f.auditLogCalls++
	return nil
}
func (f *fakeStore) AppCreateAPIKey(_ context.Context, userID uint64, planName, _, prefix string) (*model.APIKey, error) {
	return &model.APIKey{ID: 1, OwnerUserID: &userID, PlanName: planName, KeyPrefix: prefix, Status: model.APIKeyStatusActive}, nil
}
func (f *fakeStore) UpdateAPIKey(_ context.Context, _ uint64, values map[string]any) error {
	f.updateCalls++
	f.updateValues = values
	return nil
}
func (f *fakeStore) IsAPIKeyOwnedByUser(_ context.Context, _, _ uint64) (bool, error) {
	return f.owned, nil
}

type fakeBilling struct{}

func (fakeBilling) ListOrdersByUser(_ context.Context, _ uint64) ([]model.Order, error) {
	return nil, nil
}
func (fakeBilling) Pricing() []model.PricingPack { return nil }

type fakeGrowthManager struct {
	connectCalls int
	grantCalls   int
	connection   *model.DiscordConnection
	grantResult  *growthsvc.RewardGrantResult
}

func (f *fakeGrowthManager) ConnectDiscord(_ context.Context, _ uint64, _, _ string, guildMember bool) (*model.DiscordConnection, error) {
	f.connectCalls++
	if f.connection == nil {
		f.connection = &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   guildMember,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		}
	}
	copied := *f.connection
	return &copied, nil
}

func (f *fakeGrowthManager) GrantDiscordJoinReward(_ context.Context, _ uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error) {
	f.grantCalls++
	if f.grantResult != nil {
		return f.grantResult, nil
	}
	return &growthsvc.RewardGrantResult{
		Created: false,
		Grant: &model.RewardGrant{
			SourceType:     model.RewardSourceDiscordJoin,
			IdempotencyKey: "discord-join:42:discord-42",
			AmountTotal:    rewardAmount,
		},
	}, nil
}

func TestUpdateAPIKeyOnlyPersistsStatusAndNote(t *testing.T) {
	t.Parallel()

	status := string(model.APIKeyStatusDisabled)
	note := "rotate after handoff"
	store := &fakeStore{owned: true}
	svc := NewService(store, fakeBilling{}, "salt")

	err := svc.UpdateAPIKey(context.Background(), 42, 7, UpdateAPIKeyRequest{
		Status: &status,
		Note:   &note,
	})

	require.NoError(t, err)
	require.Equal(t, 1, store.updateCalls)
	require.Equal(t, map[string]any{
		"status": status,
		"note":   note,
	}, store.updateValues)
	require.Equal(t, 1, store.auditLogCalls)
	_, hasQuota := store.updateValues["quota_total"]
	require.False(t, hasQuota)
	_, hasPlan := store.updateValues["plan_name"]
	require.False(t, hasPlan)
}

func TestUpdateAPIKeyRejectsForeignKey(t *testing.T) {
	t.Parallel()

	status := string(model.APIKeyStatusDisabled)
	store := &fakeStore{owned: false}
	svc := NewService(store, fakeBilling{}, "salt")

	err := svc.UpdateAPIKey(context.Background(), 42, 9, UpdateAPIKeyRequest{Status: &status})

	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.Zero(t, store.updateCalls)
	require.Zero(t, store.auditLogCalls)
}

func TestOverviewIncludesRewardInviteAndDiscordState(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		apiKeysByOwner: []model.APIKey{
			{QuotaTotal: intPtr(10), QuotaUsed: 3},
		},
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 10, AmountUsed: 4},
			{AmountTotal: 3, AmountUsed: 1},
		},
		referrals: []model.UserReferral{
			{InvitedUserID: 100},
			{InvitedUserID: 101, ActivatedAt: timePtr()},
		},
		discord: &model.DiscordConnection{UserID: 42, GuildMember: true},
	}
	svc := NewService(store, fakeBilling{}, "salt")

	overview, err := svc.Overview(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 7, overview.TotalRemaining)
	require.Equal(t, 8, overview.RewardRemaining)
	require.Equal(t, "invite-xyz", overview.InviteCode)
	require.Equal(t, growthsvc.MaxReferralsPerInviter, overview.InviteLimit)
	require.Equal(t, growthsvc.MaxReferralsPerInviter-2, overview.InviteRemaining)
	require.Equal(t, growthsvc.InviteActivationRewardAmount, overview.RewardPerInvite)
	require.Equal(t, 2, overview.ReferralCount)
	require.Equal(t, 1, overview.ActivatedReferralCount)
	require.True(t, overview.DiscordConnected)
	require.True(t, overview.DiscordGuildMember)
}

func TestGrowthReturnsUserFacingRewardReferralAndDiscordData(t *testing.T) {
	t.Parallel()

	registeredAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	connectedAt := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	rewardGrantedAt := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	store := &fakeStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{
				SourceType:   model.RewardSourceInviteActivation,
				AmountTotal:  10,
				AmountUsed:   4,
				Reason:       "invite activation reward",
				MetadataJSON: `{"invite_code":"invite-xyz"}`,
			},
		},
		referrals: []model.UserReferral{
			{
				InviteCode:      "invite-xyz",
				RegisteredAt:    registeredAt,
				RewardGrantedAt: &rewardGrantedAt,
			},
		},
		discord: &model.DiscordConnection{
			UserID:          42,
			Username:        "officecli-user",
			GuildMember:     true,
			ConnectedAt:     connectedAt,
			RewardGrantedAt: &rewardGrantedAt,
		},
	}
	svc := NewService(store, fakeBilling{}, "salt")

	growth, err := svc.Growth(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "invite-xyz", growth.InviteCode)
	require.Equal(t, growthsvc.MaxReferralsPerInviter, growth.InviteLimit)
	require.Equal(t, growthsvc.MaxReferralsPerInviter-1, growth.InviteRemaining)
	require.Equal(t, growthsvc.InviteActivationRewardAmount, growth.RewardPerInvite)
	require.Equal(t, 6, growth.RewardRemaining)
	require.Len(t, growth.RewardGrants, 1)
	require.Equal(t, 6, growth.RewardGrants[0].Remaining)
	require.Len(t, growth.Referrals, 1)
	require.Equal(t, registeredAt, growth.Referrals[0].RegisteredAt)
	require.NotNil(t, growth.DiscordConnection)
	require.Equal(t, "officecli-user", growth.DiscordConnection.Username)
	require.True(t, growth.DiscordConnection.GuildMember)
	require.Equal(t, connectedAt, growth.DiscordConnection.ConnectedAt)
	require.Equal(t, "verified", growth.DiscordConnection.VerificationStatus)
}

func TestConnectDiscordReturnsBlockedVerificationStateWithoutReward(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 4, AmountUsed: 1},
		},
		discord: &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   false,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		},
	}
	growth := &fakeGrowthManager{
		connection: &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   false,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		},
	}
	svc := NewService(store, fakeBilling{}, "salt", growth)

	response, err := svc.ConnectDiscord(context.Background(), 42, ConnectDiscordRequest{
		DiscordUserID: "discord-42",
		Username:      "officecli-user",
	})

	require.NoError(t, err)
	require.False(t, response.RewardGranted)
	require.Equal(t, 3, response.RewardRemaining)
	require.Equal(t, "verification_blocked", response.Connection.VerificationStatus)
	require.Equal(t, DiscordGuildVerificationBlockedReason, response.Connection.VerificationBlockedReason)
	require.Equal(t, 1, growth.connectCalls)
	require.Zero(t, growth.grantCalls)
}

func TestDiscordStatusReturnsConnectionAndRewardBalance(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 5, AmountUsed: 2},
		},
		discord: &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   false,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		},
	}
	svc := NewService(store, fakeBilling{}, "salt")

	status, err := svc.DiscordStatus(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 3, status.RewardRemaining)
	require.NotNil(t, status.Connection)
	require.Equal(t, "verification_blocked", status.Connection.VerificationStatus)
}

func intPtr(v int) *int { return &v }

func timePtr() *time.Time {
	now := time.Now().UTC()
	return &now
}
