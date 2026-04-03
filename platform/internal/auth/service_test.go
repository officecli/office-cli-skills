package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	copied.GoogleSub = googleSub
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

type fakeSessionStore struct {
	payloads map[string]any
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{payloads: map[string]any{}}
}

func sessionKey(namespace, sessionID string) string { return namespace + ":" + sessionID }

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
	svc := NewService(fakeOAuthProvider{}, &fakeAuthUserStore{}, sessions, "cop_app_session", time.Hour, fakeCookieCodec{}, nil)

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
	)

	user, rawCookie, returnTo, err := svc.HandleCallback(context.Background(), "code", state)
	require.NoError(t, err)
	require.Equal(t, uint64(42), user.ID)
	require.Equal(t, "/app", returnTo)
	require.Contains(t, rawCookie, "cookie:")
	require.Equal(t, "invite-xyz", referrals.inviteCode)
	require.Equal(t, uint64(42), referrals.userID)
}
