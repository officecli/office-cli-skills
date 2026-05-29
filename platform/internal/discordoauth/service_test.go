package discordoauth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
)

type fakeClient struct {
	oauthConfigured bool
	guildConfigured bool
	user            *User
	guildMember     bool
}

func (f *fakeClient) OAuthConfigured() bool      { return f.oauthConfigured }
func (f *fakeClient) GuildCheckConfigured() bool { return f.guildConfigured }
func (f *fakeClient) AuthCodeURL(state string) string {
	return "https://discord.com/oauth2/authorize?state=" + state
}
func (f *fakeClient) ExchangeUser(_ context.Context, _ string) (*User, error) {
	return f.user, nil
}
func (f *fakeClient) VerifyGuildMembership(_ context.Context, _ string) (bool, error) {
	return f.guildMember, nil
}

type fakeSessions struct {
	values map[string]any
}

func newFakeSessions() *fakeSessions { return &fakeSessions{values: map[string]any{}} }

func (f *fakeSessions) SaveNamespacedSession(_ context.Context, namespace, sessionID string, payload any, _ time.Duration) error {
	f.values[namespace+":"+sessionID] = payload
	return nil
}
func (f *fakeSessions) LoadNamespacedSession(_ context.Context, namespace, sessionID string, dest any) (bool, error) {
	value, ok := f.values[namespace+":"+sessionID]
	if !ok {
		return false, nil
	}
	typed, ok := dest.(*StatePayload)
	if !ok {
		return false, nil
	}
	*typed = value.(StatePayload)
	return true, nil
}
func (f *fakeSessions) DeleteNamespacedSession(_ context.Context, namespace, sessionID string) error {
	delete(f.values, namespace+":"+sessionID)
	return nil
}

type fakeGrowth struct {
	connection *model.DiscordConnection
	grants     int
}

func (f *fakeGrowth) ConnectDiscord(_ context.Context, userID uint64, discordUserID, username string, guildMember bool) (*model.DiscordConnection, error) {
	f.connection = &model.DiscordConnection{
		UserID:        userID,
		DiscordUserID: discordUserID,
		Username:      username,
		GuildMember:   guildMember,
		ConnectedAt:   time.Date(2026, 4, 2, 18, 0, 0, 0, time.UTC),
	}
	return f.connection, nil
}

func (f *fakeGrowth) GrantDiscordJoinReward(_ context.Context, userID uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error) {
	f.grants++
	return &growthsvc.RewardGrantResult{
		Created: true,
		Grant: &model.RewardGrant{
			UserID:      userID,
			AmountTotal: rewardAmount,
		},
	}, nil
}

func TestLoginURLRequiresOAuthConfig(t *testing.T) {
	svc := NewService(&fakeClient{oauthConfigured: false}, newFakeSessions(), &fakeGrowth{})
	_, err := svc.LoginURL(context.Background(), 42, "/app")
	require.ErrorIs(t, err, ErrDiscordOAuthNotConfigured)
}

func TestHandleCallbackReturnsBlockedVerificationWhenGuildCheckMissing(t *testing.T) {
	sessions := newFakeSessions()
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), stateNamespace, "state-1", StatePayload{
		UserID:   42,
		ReturnTo: "/app",
	}, time.Minute))

	growth := &fakeGrowth{}
	svc := NewService(&fakeClient{
		oauthConfigured: true,
		guildConfigured: false,
		user:            &User{ID: "discord-42", Username: "member"},
	}, sessions, growth)

	result, err := svc.HandleCallback(context.Background(), "code", "state-1")
	require.NoError(t, err)
	require.Equal(t, "/app", result.ReturnTo)
	require.Equal(t, "verification_blocked", result.VerificationStatus)
	require.Equal(t, ErrDiscordGuildCheckNotConfigured.Error(), result.VerificationBlockedReason)
	require.NotNil(t, growth.connection)
	require.False(t, growth.connection.GuildMember)
	require.Zero(t, growth.grants)
}

func TestHandleCallbackGrantsRewardWhenGuildMemberVerified(t *testing.T) {
	sessions := newFakeSessions()
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), stateNamespace, "state-2", StatePayload{
		UserID:   42,
		ReturnTo: "/app",
	}, time.Minute))

	growth := &fakeGrowth{}
	svc := NewService(&fakeClient{
		oauthConfigured: true,
		guildConfigured: true,
		guildMember:     true,
		user:            &User{ID: "discord-42", Username: "member", GlobalName: "OfficeCLI Member"},
	}, sessions, growth)

	result, err := svc.HandleCallback(context.Background(), "code", "state-2")
	require.NoError(t, err)
	require.Equal(t, "verified", result.VerificationStatus)
	require.True(t, result.RewardGranted)
	require.NotNil(t, growth.connection)
	require.True(t, growth.connection.GuildMember)
	require.Equal(t, "OfficeCLI Member", growth.connection.Username)
	require.Equal(t, 1, growth.grants)
}
