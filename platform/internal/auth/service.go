package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/officecli/officecli/platform/internal/model"
)

type GoogleUser struct {
	Subject   string
	Email     string
	Name      string
	AvatarURL *string
}

type OAuthProvider interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (*GoogleUser, error)
}

type UserStore interface {
	SaveGoogleUser(ctx context.Context, googleSub, email, name string, avatarURL *string) (*model.User, error)
	GetUserByID(ctx context.Context, id uint64) (*model.User, error)
}

type SessionStore interface {
	SaveNamespacedSession(ctx context.Context, namespace, sessionID string, payload any, ttl time.Duration) error
	LoadNamespacedSession(ctx context.Context, namespace, sessionID string, dest any) (bool, error)
	DeleteNamespacedSession(ctx context.Context, namespace, sessionID string) error
}

type ReferralRegistrar interface {
	RegisterReferral(ctx context.Context, inviteCode string, invitedUserID uint64) (*model.UserReferral, error)
}

type CookieCodec interface {
	Encode(sessionID string) (string, error)
	Decode(value string) (string, error)
}

type SessionPayload struct {
	SessionID string    `json:"session_id"`
	UserID    uint64    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	provider   OAuthProvider
	users      UserStore
	sessions   SessionStore
	referrals  ReferralRegistrar
	cookieName string
	sessionTTL time.Duration
	codec      CookieCodec
}

func NewService(provider OAuthProvider, users UserStore, sessions SessionStore, cookieName string, sessionTTL time.Duration, codec CookieCodec, referrals ReferralRegistrar) *Service {
	return &Service{
		provider:   provider,
		users:      users,
		sessions:   sessions,
		referrals:  referrals,
		cookieName: cookieName,
		sessionTTL: sessionTTL,
		codec:      codec,
	}
}

func (s *Service) LoginURL(ctx context.Context, returnTo, inviteCode string) (string, error) {
	state := uuid.NewString()
	payload := map[string]string{"return_to": returnTo}
	if normalizedInviteCode := strings.TrimSpace(inviteCode); normalizedInviteCode != "" {
		payload["invite_code"] = normalizedInviteCode
	}
	if err := s.sessions.SaveNamespacedSession(ctx, "oauth_state", state, payload, 10*time.Minute); err != nil {
		return "", err
	}
	return s.provider.AuthCodeURL(state), nil
}

func (s *Service) HandleCallback(ctx context.Context, code, state string) (*model.User, string, string, error) {
	var payload map[string]string
	ok, err := s.sessions.LoadNamespacedSession(ctx, "oauth_state", state, &payload)
	if err != nil || !ok {
		return nil, "", "", fmt.Errorf("invalid oauth state")
	}
	_ = s.sessions.DeleteNamespacedSession(ctx, "oauth_state", state)

	googleUser, err := s.provider.Exchange(ctx, code)
	if err != nil {
		return nil, "", "", err
	}
	user, err := s.users.SaveGoogleUser(ctx, googleUser.Subject, googleUser.Email, googleUser.Name, googleUser.AvatarURL)
	if err != nil {
		return nil, "", "", err
	}
	if inviteCode := strings.TrimSpace(payload["invite_code"]); inviteCode != "" && s.referrals != nil {
		if _, err := s.referrals.RegisterReferral(ctx, inviteCode, user.ID); err != nil {
			return nil, "", "", err
		}
	}

	sessionID := uuid.NewString()
	session := SessionPayload{SessionID: sessionID, UserID: user.ID, CreatedAt: time.Now().UTC()}
	if err := s.sessions.SaveNamespacedSession(ctx, "app", sessionID, session, s.sessionTTL); err != nil {
		return nil, "", "", err
	}
	rawCookie, err := s.codec.Encode(sessionID)
	if err != nil {
		return nil, "", "", err
	}
	returnTo := "/app"
	if payload["return_to"] != "" {
		returnTo = payload["return_to"]
	}
	return user, rawCookie, returnTo, nil
}

func (s *Service) ResolveSession(raw string) (*SessionPayload, error) {
	sessionID, err := s.codec.Decode(raw)
	if err != nil {
		return nil, err
	}
	var payload SessionPayload
	ok, err := s.sessions.LoadNamespacedSession(context.Background(), "app", sessionID, &payload)
	if err != nil || !ok {
		return nil, err
	}
	return &payload, nil
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	sessionID, err := s.codec.Decode(raw)
	if err != nil {
		return err
	}
	return s.sessions.DeleteNamespacedSession(ctx, "app", sessionID)
}

func (s *Service) Me(ctx context.Context, raw string) (*model.User, error) {
	payload, err := s.ResolveSession(raw)
	if err != nil {
		return nil, err
	}
	return s.users.GetUserByID(ctx, payload.UserID)
}

func MarshalAudit(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
