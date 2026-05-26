package auth

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
)

type fakeOAuthProvider struct {
	user *GoogleUser
	err  error
}

func (f fakeOAuthProvider) AuthCodeURL(state string) string {
	return "https://example.com/oauth?state=" + state
}

func (f fakeOAuthProvider) Exchange(_ context.Context, _ string) (*GoogleUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.user, nil
}

type fakeAuthUserStore struct {
	user *model.User
	err  error
}

func (f *fakeAuthUserStore) SaveGoogleUser(_ context.Context, googleSub, email, name string, avatarURL *string) (*model.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	copied := *f.user
	copied.GoogleSub = model.StringPtr(googleSub)
	copied.Email = email
	copied.Name = name
	copied.AvatarURL = avatarURL
	return &copied, nil
}

func (f *fakeAuthUserStore) SaveGitHubUser(_ context.Context, githubSub, email, name string, avatarURL *string) (*model.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	copied := *f.user
	copied.GitHubSub = model.StringPtr(githubSub)
	copied.Email = email
	copied.Name = name
	copied.AvatarURL = avatarURL
	return &copied, nil
}

func (f *fakeAuthUserStore) GetUserByID(_ context.Context, id uint64) (*model.User, error) {
	if f.user != nil && f.user.ID == id {
		copied := *f.user
		return &copied, nil
	}
	return nil, nil
}

func (f *fakeAuthUserStore) CreateAuditLog(_ context.Context, action, targetType, targetID string, payload string) error {
	return nil
}

type fakeSessionStore struct {
	payloads     map[string]any
	userSessions map[string]map[string]struct{}
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{
		payloads:     map[string]any{},
		userSessions: map[string]map[string]struct{}{},
	}
}

func sessionKey(namespace, sessionID string) string { return namespace + ":" + sessionID }
func userSessionKey(namespace string, userID uint64) string {
	return namespace + ":" + strconv.FormatUint(userID, 10)
}

func (f *fakeSessionStore) SaveNamespacedSession(_ context.Context, namespace, sessionID string, payload any, _ time.Duration) error {
	f.payloads[sessionKey(namespace, sessionID)] = payload
	return nil
}

func (f *fakeSessionStore) LoadNamespacedSession(_ context.Context, namespace, sessionID string, dest any) (bool, error) {
	payload, ok := f.payloads[sessionKey(namespace, sessionID)]
	if !ok {
		return false, nil
	}
	switch out := dest.(type) {
	case *map[string]string:
		*out = payload.(map[string]string)
	case *SessionPayload:
		*out = payload.(SessionPayload)
	default:
		return false, errors.New("unexpected destination type")
	}
	return true, nil
}

func (f *fakeSessionStore) DeleteNamespacedSession(_ context.Context, namespace, sessionID string) error {
	delete(f.payloads, sessionKey(namespace, sessionID))
	return nil
}

func (f *fakeSessionStore) AddUserNamespacedSession(_ context.Context, namespace string, userID uint64, sessionID string, _ time.Duration) error {
	key := userSessionKey(namespace, userID)
	if f.userSessions[key] == nil {
		f.userSessions[key] = map[string]struct{}{}
	}
	f.userSessions[key][sessionID] = struct{}{}
	return nil
}

func (f *fakeSessionStore) RemoveUserNamespacedSession(_ context.Context, namespace string, userID uint64, sessionID string) error {
	key := userSessionKey(namespace, userID)
	delete(f.userSessions[key], sessionID)
	if len(f.userSessions[key]) == 0 {
		delete(f.userSessions, key)
	}
	return nil
}

func (f *fakeSessionStore) DeleteUserNamespacedSessions(_ context.Context, namespace string, userID uint64) error {
	key := userSessionKey(namespace, userID)
	for sessionID := range f.userSessions[key] {
		delete(f.payloads, sessionKey(namespace, sessionID))
	}
	delete(f.userSessions, key)
	return nil
}

type fakeCookieCodec struct{}

func (fakeCookieCodec) Encode(sessionID string) (string, error) { return "cookie:" + sessionID, nil }
func (fakeCookieCodec) Decode(value string) (string, error)     { return value[len("cookie:"):], nil }

type fakeReferralRegistrar struct {
	inviteCode string
	userID     uint64
	err        error
}

func (f *fakeReferralRegistrar) RegisterReferral(_ context.Context, inviteCode string, invitedUserID uint64) (*model.UserReferral, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.inviteCode = inviteCode
	f.userID = invitedUserID
	return &model.UserReferral{InvitedUserID: invitedUserID, InviteCode: inviteCode}, nil
}

func TestLoginURLStoresInviteCodeInOAuthState(t *testing.T) {
	sessions := newFakeSessionStore()
	svc := NewService(fakeOAuthProvider{}, &fakeAuthUserStore{}, sessions, "cop_app_session", time.Hour, fakeCookieCodec{}, nil, nil)

	url, err := svc.LoginURL(context.Background(), "/app", "invite-abc")
	require.NoError(t, err)
	require.Contains(t, url, "state=")
	require.Len(t, sessions.payloads, 1)

	for _, raw := range sessions.payloads {
		payload := raw.(map[string]string)
		require.Equal(t, "/app", payload["return_to"])
		require.Equal(t, "invite-abc", payload["invite_code"])
	}
}

func TestLoginURLNormalizesAppRelativeReturnToInOAuthState(t *testing.T) {
	sessions := newFakeSessionStore()
	svc := NewService(fakeOAuthProvider{}, &fakeAuthUserStore{}, sessions, "cop_app_session", time.Hour, fakeCookieCodec{}, nil, nil)

	_, err := svc.LoginURL(context.Background(), "/billing?status=success", "")
	require.NoError(t, err)
	require.Len(t, sessions.payloads, 1)

	for _, raw := range sessions.payloads {
		payload := raw.(map[string]string)
		require.Equal(t, "/app/billing?status=success", payload["return_to"])
	}
}

func TestLoginURLPreservesCLILoginCompleteReturnToInOAuthState(t *testing.T) {
	sessions := newFakeSessionStore()
	svc := NewService(fakeOAuthProvider{}, &fakeAuthUserStore{}, sessions, "cop_app_session", time.Hour, fakeCookieCodec{}, nil, nil)

	_, err := svc.LoginURL(context.Background(), "/api/cli/login/complete?challenge_id=cli_123", "")
	require.NoError(t, err)
	require.Len(t, sessions.payloads, 1)

	for _, raw := range sessions.payloads {
		payload := raw.(map[string]string)
		require.Equal(t, "/api/cli/login/complete?challenge_id=cli_123", payload["return_to"])
	}
}

func TestHandleCallbackRegistersReferralFromOAuthState(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to":   "/app",
		"invite_code": "invite-xyz",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042"}}
	referrals := &fakeReferralRegistrar{}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "demo@example.com", Name: "Demo"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		referrals,
		[]string{"demo@example.com"},
	)

	user, rawCookie, returnTo, err := svc.HandleCallback(context.Background(), "code", state)
	require.NoError(t, err)
	require.Equal(t, uint64(42), user.ID)
	require.Equal(t, "/app", returnTo)
	require.Contains(t, rawCookie, "cookie:")
	require.Equal(t, "invite-xyz", referrals.inviteCode)
	require.Equal(t, uint64(42), referrals.userID)
}

func TestHandleCallbackIgnoresInviteLimitError(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to":   "/app",
		"invite_code": "invite-xyz",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042"}}
	referrals := &fakeReferralRegistrar{err: growthsvc.ErrInviteLimitReached}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "demo@example.com", Name: "Demo"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		referrals,
		[]string{"demo@example.com"},
	)

	user, rawCookie, returnTo, err := svc.HandleCallback(context.Background(), "code", state)
	require.NoError(t, err)
	require.Equal(t, uint64(42), user.ID)
	require.Equal(t, "/app", returnTo)
	require.Contains(t, rawCookie, "cookie:")
}

func TestHandleCallbackNormalizesLegacyAppRelativeReturnTo(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to": "/billing?status=success&session_id=cs_test_123",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042"}}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "demo@example.com", Name: "Demo"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		nil,
		[]string{"demo@example.com"},
	)

	_, _, returnTo, err := svc.HandleCallback(context.Background(), "code", state)
	require.NoError(t, err)
	require.Equal(t, "/app/billing?status=success&session_id=cs_test_123", returnTo)
}

func TestHandleCallbackPreservesCLILoginVerifyReturnTo(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to": "/api/cli/login/verify?user_code=568J-DJZ5",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042"}}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "demo@example.com", Name: "Demo"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		nil,
		[]string{"demo@example.com"},
	)

	_, _, returnTo, err := svc.HandleCallback(context.Background(), "code", state)
	require.NoError(t, err)
	require.Equal(t, "/api/cli/login/verify?user_code=568J-DJZ5", returnTo)
}

func TestHandleCallbackRejectsNonAllowlistedEmail(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to": "/app",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042", Status: model.UserStatusActive}}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "blocked@example.com", Name: "Blocked"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		nil,
		[]string{"demo@example.com"},
	)

	_, _, _, err := svc.HandleCallback(context.Background(), "code", state)
	require.Error(t, err)
	var denied *AccessDeniedError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, "blocked@example.com", denied.Email)
	require.Equal(t, "email_not_allowlisted", denied.Reason)
}

func TestHandleCallbackAllowsAnyEmailWhenAllowlistWildcardIsConfigured(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to": "/app",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042", Status: model.UserStatusActive}}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "anyone@example.com", Name: "Anyone"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		nil,
		[]string{"*"},
	)

	user, rawCookie, returnTo, err := svc.HandleCallback(context.Background(), "code", state)
	require.NoError(t, err)
	require.Equal(t, uint64(42), user.ID)
	require.Equal(t, "/app", returnTo)
	require.Contains(t, rawCookie, "cookie:")
}

func TestHandleCallbackKeepsAbsolutePreviewReturnTo(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to": "https://officecli.io/p/share-token",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042", Status: model.UserStatusActive}}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "anyone@example.com", Name: "Anyone"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		nil,
		[]string{"*"},
	)

	_, _, returnTo, err := svc.HandleCallback(context.Background(), "code", state)
	require.NoError(t, err)
	require.Equal(t, "https://officecli.io/p/share-token", returnTo)
}

func TestHandleCallbackRejectsDisabledUser(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to": "/app",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042", Status: model.UserStatusDisabled}}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "demo@example.com", Name: "Blocked"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		nil,
		[]string{"demo@example.com"},
	)

	_, _, _, err := svc.HandleCallback(context.Background(), "code", state)
	require.Error(t, err)
	var denied *AccessDeniedError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, "demo@example.com", denied.Email)
	require.Equal(t, "user_disabled", denied.Reason)
}

func TestHandleCallbackIndexesAppSessionByUser(t *testing.T) {
	sessions := newFakeSessionStore()
	state := "oauth-state"
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "oauth_state", state, map[string]string{
		"return_to": "/app",
	}, time.Minute))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, InviteCode: "invite-042", Status: model.UserStatusActive}}
	svc := NewService(
		fakeOAuthProvider{user: &GoogleUser{Subject: "google-sub", Email: "demo@example.com", Name: "Demo"}},
		users,
		sessions,
		"cop_app_session",
		time.Hour,
		fakeCookieCodec{},
		nil,
		[]string{"demo@example.com"},
	)

	_, rawCookie, _, err := svc.HandleCallback(context.Background(), "code", state)
	require.NoError(t, err)

	sessionID, err := fakeCookieCodec{}.Decode(rawCookie)
	require.NoError(t, err)
	_, ok := sessions.userSessions[userSessionKey("app", 42)][sessionID]
	require.True(t, ok)
}

func TestResolveSessionRejectsDisabledUserAndDeletesSession(t *testing.T) {
	sessions := newFakeSessionStore()
	require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "app", "session-1", SessionPayload{
		SessionID: "session-1",
		UserID:    42,
		CreatedAt: time.Now().UTC(),
	}, time.Hour))
	require.NoError(t, sessions.AddUserNamespacedSession(context.Background(), "app", 42, "session-1", time.Hour))

	users := &fakeAuthUserStore{user: &model.User{ID: 42, Status: model.UserStatusDisabled}}
	svc := NewService(fakeOAuthProvider{}, users, sessions, "cop_app_session", time.Hour, fakeCookieCodec{}, nil, nil)

	payload, err := svc.ResolveSession("cookie:session-1")
	require.Error(t, err)
	require.Nil(t, payload)
	_, exists := sessions.payloads[sessionKey("app", "session-1")]
	require.False(t, exists)
	_, indexed := sessions.userSessions[userSessionKey("app", 42)]
	require.False(t, indexed)
}

func TestRevokeUserSessionsDeletesAllIndexedSessions(t *testing.T) {
	sessions := newFakeSessionStore()
	for _, sessionID := range []string{"session-1", "session-2"} {
		require.NoError(t, sessions.SaveNamespacedSession(context.Background(), "app", sessionID, SessionPayload{
			SessionID: sessionID,
			UserID:    42,
			CreatedAt: time.Now().UTC(),
		}, time.Hour))
		require.NoError(t, sessions.AddUserNamespacedSession(context.Background(), "app", 42, sessionID, time.Hour))
	}

	svc := NewService(fakeOAuthProvider{}, &fakeAuthUserStore{user: &model.User{ID: 42, Status: model.UserStatusActive}}, sessions, "cop_app_session", time.Hour, fakeCookieCodec{}, nil, nil)

	require.NoError(t, svc.RevokeUserSessions(context.Background(), 42))
	require.Empty(t, sessions.payloads)
	_, indexed := sessions.userSessions[userSessionKey("app", 42)]
	require.False(t, indexed)
}
