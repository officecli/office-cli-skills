package growth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
)

type fakeUsers struct {
	byCode map[string]*model.User
}

func (f *fakeUsers) FindUserByInviteCode(_ context.Context, inviteCode string) (*model.User, error) {
	if user, ok := f.byCode[inviteCode]; ok {
		copied := *user
		return &copied, nil
	}
	return nil, nil
}

type fakeReferrals struct {
	byInvited map[uint64]*model.UserReferral
}

func newFakeReferrals() *fakeReferrals {
	return &fakeReferrals{byInvited: map[uint64]*model.UserReferral{}}
}

func (f *fakeReferrals) FindReferralByInvitedUserID(_ context.Context, invitedUserID uint64) (*model.UserReferral, error) {
	if referral, ok := f.byInvited[invitedUserID]; ok {
		copied := *referral
		return &copied, nil
	}
	return nil, nil
}

func (f *fakeReferrals) CountReferralsByInviterUserID(_ context.Context, inviterUserID uint64) (int64, error) {
	var count int64
	for _, referral := range f.byInvited {
		if referral.InviterUserID == inviterUserID {
			count++
		}
	}
	return count, nil
}

func (f *fakeReferrals) SaveReferral(_ context.Context, referral *model.UserReferral) error {
	copied := *referral
	f.byInvited[referral.InvitedUserID] = &copied
	return nil
}

type fakeDiscord struct {
	byUser    map[uint64]*model.DiscordConnection
	byDiscord map[string]*model.DiscordConnection
}

func newFakeDiscord() *fakeDiscord {
	return &fakeDiscord{
		byUser:    map[uint64]*model.DiscordConnection{},
		byDiscord: map[string]*model.DiscordConnection{},
	}
}

func (f *fakeDiscord) FindDiscordConnectionByUserID(_ context.Context, userID uint64) (*model.DiscordConnection, error) {
	if connection, ok := f.byUser[userID]; ok {
		copied := *connection
		return &copied, nil
	}
	return nil, nil
}

func (f *fakeDiscord) FindDiscordConnectionByDiscordUserID(_ context.Context, discordUserID string) (*model.DiscordConnection, error) {
	if connection, ok := f.byDiscord[discordUserID]; ok {
		copied := *connection
		return &copied, nil
	}
	return nil, nil
}

func (f *fakeDiscord) SaveDiscordConnection(_ context.Context, connection *model.DiscordConnection) error {
	copied := *connection
	f.byUser[connection.UserID] = &copied
	f.byDiscord[connection.DiscordUserID] = &copied
	return nil
}

type fakeGrants struct {
	byKey map[string]*model.RewardGrant
}

func newFakeGrants() *fakeGrants {
	return &fakeGrants{byKey: map[string]*model.RewardGrant{}}
}

func (f *fakeGrants) FindRewardGrantByIdempotencyKey(_ context.Context, key string) (*model.RewardGrant, error) {
	if grant, ok := f.byKey[key]; ok {
		copied := *grant
		return &copied, nil
	}
	return nil, nil
}

func (f *fakeGrants) SaveRewardGrant(_ context.Context, grant *model.RewardGrant) error {
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = time.Now().UTC()
	}
	copied := *grant
	f.byKey[grant.IdempotencyKey] = &copied
	return nil
}

func TestRegisterReferralIsIdempotent(t *testing.T) {
	referrals := newFakeReferrals()
	svc := NewService(
		&fakeUsers{byCode: map[string]*model.User{"invite-abc": {ID: 11, InviteCode: "invite-abc"}}},
		referrals,
		newFakeDiscord(),
		newFakeGrants(),
	)
	svc.clock = func() time.Time { return time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC) }

	first, err := svc.RegisterReferral(context.Background(), "invite-abc", 22)
	require.NoError(t, err)
	require.Equal(t, uint64(11), first.InviterUserID)
	require.Equal(t, uint64(22), first.InvitedUserID)

	second, err := svc.RegisterReferral(context.Background(), "invite-abc", 22)
	require.NoError(t, err)
	require.Equal(t, first.InviterUserID, second.InviterUserID)
	require.Len(t, referrals.byInvited, 1)
}

func TestRegisterReferralRejectsSelfReferral(t *testing.T) {
	svc := NewService(
		&fakeUsers{byCode: map[string]*model.User{"invite-self": {ID: 8, InviteCode: "invite-self"}}},
		newFakeReferrals(),
		newFakeDiscord(),
		newFakeGrants(),
	)

	_, err := svc.RegisterReferral(context.Background(), "invite-self", 8)
	require.ErrorIs(t, err, ErrSelfReferral)
}

func TestRegisterReferralRejectsInviteLimit(t *testing.T) {
	referrals := newFakeReferrals()
	for i := uint64(0); i < MaxReferralsPerInviter; i++ {
		referrals.byInvited[100+i] = &model.UserReferral{
			InviterUserID: 11,
			InvitedUserID: 100 + i,
			InviteCode:    "invite-abc",
			RegisteredAt:  time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		}
	}
	svc := NewService(
		&fakeUsers{byCode: map[string]*model.User{"invite-abc": {ID: 11, InviteCode: "invite-abc"}}},
		referrals,
		newFakeDiscord(),
		newFakeGrants(),
	)

	_, err := svc.RegisterReferral(context.Background(), "invite-abc", 999)
	require.ErrorIs(t, err, ErrInviteLimitReached)
	require.Len(t, referrals.byInvited, MaxReferralsPerInviter)
}

func TestActivateReferralCreatesSingleGrant(t *testing.T) {
	referrals := newFakeReferrals()
	referrals.byInvited[22] = &model.UserReferral{
		InviterUserID: 11,
		InvitedUserID: 22,
		InviteCode:    "invite-abc",
		RegisteredAt:  time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}
	grants := newFakeGrants()
	svc := NewService(&fakeUsers{}, referrals, newFakeDiscord(), grants)
	svc.clock = func() time.Time { return time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC) }

	first, err := svc.ActivateReferral(context.Background(), 22, 5)
	require.NoError(t, err)
	require.True(t, first.Created)
	require.Equal(t, model.RewardSourceInviteActivation, first.Grant.SourceType)
	require.Equal(t, "invite-activation:22", first.Grant.IdempotencyKey)

	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(first.Grant.MetadataJSON), &metadata))
	require.EqualValues(t, 22, metadata["invited_user_id"])

	second, err := svc.ActivateReferral(context.Background(), 22, 5)
	require.NoError(t, err)
	require.False(t, second.Created)
	require.Len(t, grants.byKey, 1)

	updatedReferral, err := referrals.FindReferralByInvitedUserID(context.Background(), 22)
	require.NoError(t, err)
	require.NotNil(t, updatedReferral.ActivatedAt)
	require.NotNil(t, updatedReferral.RewardGrantedAt)
}

func TestConnectDiscordRejectsConflictingBinding(t *testing.T) {
	discord := newFakeDiscord()
	discord.byDiscord["discord-1"] = &model.DiscordConnection{
		UserID:        7,
		DiscordUserID: "discord-1",
		Username:      "demo",
	}

	svc := NewService(&fakeUsers{}, newFakeReferrals(), discord, newFakeGrants())
	_, err := svc.ConnectDiscord(context.Background(), 8, "discord-1", "other", true)
	require.ErrorIs(t, err, ErrDiscordAlreadyLinked)
}

func TestConnectDiscordCreatesConnection(t *testing.T) {
	discord := newFakeDiscord()
	svc := NewService(&fakeUsers{}, newFakeReferrals(), discord, newFakeGrants())
	svc.clock = func() time.Time { return time.Date(2026, 4, 2, 12, 30, 0, 0, time.UTC) }

	connection, err := svc.ConnectDiscord(context.Background(), 11, "discord-11", "member", false)
	require.NoError(t, err)
	require.Equal(t, uint64(11), connection.UserID)
	require.Equal(t, "discord-11", connection.DiscordUserID)
	require.Equal(t, "member", connection.Username)
	require.False(t, connection.GuildMember)

	saved, err := discord.FindDiscordConnectionByUserID(context.Background(), 11)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "discord-11", saved.DiscordUserID)
}

func TestConnectDiscordIsIdempotent(t *testing.T) {
	discord := newFakeDiscord()
	svc := NewService(&fakeUsers{}, newFakeReferrals(), discord, newFakeGrants())
	svc.clock = func() time.Time { return time.Date(2026, 4, 2, 13, 0, 0, 0, time.UTC) }

	first, err := svc.ConnectDiscord(context.Background(), 11, "discord-11", "member", false)
	require.NoError(t, err)

	second, err := svc.ConnectDiscord(context.Background(), 11, "discord-11", "member-renamed", false)
	require.NoError(t, err)
	require.Equal(t, first.DiscordUserID, second.DiscordUserID)
	require.Equal(t, "member-renamed", second.Username)
	require.Len(t, discord.byUser, 1)
	require.Len(t, discord.byDiscord, 1)
}

func TestGrantDiscordJoinRewardRequiresGuildMembership(t *testing.T) {
	discord := newFakeDiscord()
	discord.byUser[11] = &model.DiscordConnection{
		UserID:        11,
		DiscordUserID: "discord-11",
		Username:      "member",
		GuildMember:   false,
	}

	svc := NewService(&fakeUsers{}, newFakeReferrals(), discord, newFakeGrants())
	_, err := svc.GrantDiscordJoinReward(context.Background(), 11, 3)
	require.ErrorIs(t, err, ErrDiscordGuildRequired)
}

func TestGrantDiscordJoinRewardIsIdempotent(t *testing.T) {
	discord := newFakeDiscord()
	discord.byUser[11] = &model.DiscordConnection{
		UserID:        11,
		DiscordUserID: "discord-11",
		Username:      "member",
		GuildMember:   true,
		ConnectedAt:   time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}
	discord.byDiscord["discord-11"] = discord.byUser[11]

	grants := newFakeGrants()
	svc := NewService(&fakeUsers{}, newFakeReferrals(), discord, grants)
	svc.clock = func() time.Time { return time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC) }

	first, err := svc.GrantDiscordJoinReward(context.Background(), 11, 2)
	require.NoError(t, err)
	require.True(t, first.Created)
	require.Equal(t, model.RewardSourceDiscordJoin, first.Grant.SourceType)

	second, err := svc.GrantDiscordJoinReward(context.Background(), 11, 2)
	require.NoError(t, err)
	require.False(t, second.Created)
	require.Len(t, grants.byKey, 1)

	connection, err := discord.FindDiscordConnectionByUserID(context.Background(), 11)
	require.NoError(t, err)
	require.NotNil(t, connection.RewardGrantedAt)
}
