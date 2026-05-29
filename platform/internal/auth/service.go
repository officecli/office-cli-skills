package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
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
	SaveGitHubUser(ctx context.Context, githubSub, email, name string, avatarURL *string) (*model.User, error)
	GetUserByID(ctx context.Context, id uint64) (*model.User, error)
}

type SessionStore interface {
	SaveNamespacedSession(ctx context.Context, namespace, sessionID string, payload any, ttl time.Duration) error
	LoadNamespacedSession(ctx context.Context, namespace, sessionID string, dest any) (bool, error)
	DeleteNamespacedSession(ctx context.Context, namespace, sessionID string) error
	AddUserNamespacedSession(ctx context.Context, namespace string, userID uint64, sessionID string, ttl time.Duration) error
	RemoveUserNamespacedSession(ctx context.Context, namespace string, userID uint64, sessionID string) error
	DeleteUserNamespacedSessions(ctx context.Context, namespace string, userID uint64) error
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
	provider         OAuthProvider
	users            UserStore
	sessions         SessionStore
	referrals        ReferralRegistrar
	cookieName       string
	sessionTTL       time.Duration
	codec            CookieCodec
	allowlist        map[string]struct{}
	allowAll         bool
	githubProvider   OAuthProvider
	githubAllowlist  map[string]struct{}
	githubAllowAll   bool
}

func NewService(provider OAuthProvider, users UserStore, sessions SessionStore, cookieName string, sessionTTL time.Duration, codec CookieCodec, referrals ReferralRegistrar, allowlist []string) *Service {
	normalizedAllowlist, allowAll := normalizeAllowlist(allowlist)
	return &Service{
		provider:   provider,
		users:      users,
		sessions:   sessions,
		referrals:  referrals,
		cookieName: cookieName,
		sessionTTL: sessionTTL,
		codec:      codec,
		allowlist:  normalizedAllowlist,
		allowAll:   allowAll,
	}
}

func (s *Service) WithGitHubProvider(provider OAuthProvider, allowlist []string) *Service {
	s.githubProvider = provider
	s.githubAllowlist, s.githubAllowAll = normalizeAllowlist(allowlist)
	return s
}

func (s *Service) GitHubEnabled() bool {
	return s != nil && s.githubProvider != nil
}

func normalizeAllowlist(allowlist []string) (map[string]struct{}, bool) {
	normalized := make(map[string]struct{}, len(allowlist))
	for _, email := range allowlist {
		entry := strings.ToLower(strings.TrimSpace(email))
		if entry == "" {
			continue
		}
		if entry == "*" {
			return map[string]struct{}{}, true
		}
		normalized[entry] = struct{}{}
	}
	return normalized, false
}

func normalizeReturnTo(returnTo string) string {
	requested := strings.TrimSpace(returnTo)
	if requested == "" {
		return "/app"
	}
	lower := strings.ToLower(requested)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return requested
	}
	if !strings.HasPrefix(requested, "/") {
		return "/app"
	}
	if requested == "/api/cli/login/complete" || strings.HasPrefix(requested, "/api/cli/login/complete?") {
		return requested
	}
	if requested == "/api/cli/login/verify" || strings.HasPrefix(requested, "/api/cli/login/verify?") {
		return requested
	}
	if requested == "/app" || strings.HasPrefix(requested, "/app/") || strings.HasPrefix(requested, "/app?") || strings.HasPrefix(requested, "/app#") {
		return requested
	}
	if requested == "/" {
		return "/app"
	}
	if strings.HasPrefix(requested, "/?") || strings.HasPrefix(requested, "/#") {
		return "/app" + strings.TrimPrefix(requested, "/")
	}
	return "/app" + requested
}

func (s *Service) LoginURL(ctx context.Context, returnTo, inviteCode string) (string, error) {
	state := uuid.NewString()
	payload := map[string]string{"return_to": normalizeReturnTo(returnTo)}
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
	normalizedEmail := strings.ToLower(strings.TrimSpace(googleUser.Email))
	if !s.allowAll {
		if _, allowed := s.allowlist[normalizedEmail]; !allowed {
			s.writeAuditLog(ctx, "app.google_login_denied", normalizedEmail, map[string]any{
				"email":  normalizedEmail,
				"name":   googleUser.Name,
				"reason": "email_not_allowlisted",
			})
			return nil, "", "", &AccessDeniedError{Email: normalizedEmail, Reason: "email_not_allowlisted"}
		}
	}
	user, err := s.users.SaveGoogleUser(ctx, googleUser.Subject, normalizedEmail, googleUser.Name, googleUser.AvatarURL)
	if err != nil {
		return nil, "", "", err
	}
	if user.Status == model.UserStatusDisabled {
		s.writeAuditLog(ctx, "app.google_login_denied", normalizedEmail, map[string]any{
			"email":   normalizedEmail,
			"name":    googleUser.Name,
			"user_id": user.ID,
			"reason":  "user_disabled",
		})
		return nil, "", "", &AccessDeniedError{Email: normalizedEmail, Reason: "user_disabled"}
	}
	if inviteCode := strings.TrimSpace(payload["invite_code"]); inviteCode != "" && s.referrals != nil {
		if _, err := s.referrals.RegisterReferral(ctx, inviteCode, user.ID); err != nil && !errors.Is(err, growthsvc.ErrInviteLimitReached) {
			return nil, "", "", err
		}
	}

	sessionID := uuid.NewString()
	session := SessionPayload{SessionID: sessionID, UserID: user.ID, CreatedAt: time.Now().UTC()}
	if err := s.sessions.SaveNamespacedSession(ctx, "app", sessionID, session, s.sessionTTL); err != nil {
		return nil, "", "", err
	}
	if err := s.sessions.AddUserNamespacedSession(ctx, "app", user.ID, sessionID, s.sessionTTL); err != nil {
		_ = s.sessions.DeleteNamespacedSession(ctx, "app", sessionID)
		return nil, "", "", err
	}
	rawCookie, err := s.codec.Encode(sessionID)
	if err != nil {
		_ = s.removeAppSession(ctx, session)
		return nil, "", "", err
	}
	returnTo := normalizeReturnTo(payload["return_to"])
	return user, rawCookie, returnTo, nil
}

func (s *Service) GitHubLoginURL(ctx context.Context, returnTo, inviteCode string) (string, error) {
	if !s.GitHubEnabled() {
		return "", fmt.Errorf("github login is not configured")
	}
	state := uuid.NewString()
	payload := map[string]string{"return_to": normalizeReturnTo(returnTo)}
	if normalizedInviteCode := strings.TrimSpace(inviteCode); normalizedInviteCode != "" {
		payload["invite_code"] = normalizedInviteCode
	}
	if err := s.sessions.SaveNamespacedSession(ctx, "oauth_state", state, payload, 10*time.Minute); err != nil {
		return "", err
	}
	return s.githubProvider.AuthCodeURL(state), nil
}

func (s *Service) HandleGitHubCallback(ctx context.Context, code, state string) (*model.User, string, string, error) {
	if !s.GitHubEnabled() {
		return nil, "", "", fmt.Errorf("github login is not configured")
	}
	var payload map[string]string
	ok, err := s.sessions.LoadNamespacedSession(ctx, "oauth_state", state, &payload)
	if err != nil || !ok {
		return nil, "", "", fmt.Errorf("invalid oauth state")
	}
	_ = s.sessions.DeleteNamespacedSession(ctx, "oauth_state", state)

	ghUser, err := s.githubProvider.Exchange(ctx, code)
	if err != nil {
		return nil, "", "", err
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(ghUser.Email))
	if !s.githubAllowAll {
		if _, allowed := s.githubAllowlist[normalizedEmail]; !allowed {
			s.writeAuditLogWithTarget(ctx, "app.github_login_denied", "github_account", normalizedEmail, map[string]any{
				"email":  normalizedEmail,
				"name":   ghUser.Name,
				"reason": "email_not_allowlisted",
			})
			return nil, "", "", &AccessDeniedError{Email: normalizedEmail, Reason: "email_not_allowlisted"}
		}
	}
	user, err := s.users.SaveGitHubUser(ctx, ghUser.Subject, normalizedEmail, ghUser.Name, ghUser.AvatarURL)
	if err != nil {
		return nil, "", "", err
	}
	if user.Status == model.UserStatusDisabled {
		s.writeAuditLogWithTarget(ctx, "app.github_login_denied", "github_account", normalizedEmail, map[string]any{
			"email":   normalizedEmail,
			"name":    ghUser.Name,
			"user_id": user.ID,
			"reason":  "user_disabled",
		})
		return nil, "", "", &AccessDeniedError{Email: normalizedEmail, Reason: "user_disabled"}
	}
	if inviteCode := strings.TrimSpace(payload["invite_code"]); inviteCode != "" && s.referrals != nil {
		if _, err := s.referrals.RegisterReferral(ctx, inviteCode, user.ID); err != nil && !errors.Is(err, growthsvc.ErrInviteLimitReached) {
			return nil, "", "", err
		}
	}

	sessionID := uuid.NewString()
	session := SessionPayload{SessionID: sessionID, UserID: user.ID, CreatedAt: time.Now().UTC()}
	if err := s.sessions.SaveNamespacedSession(ctx, "app", sessionID, session, s.sessionTTL); err != nil {
		return nil, "", "", err
	}
	if err := s.sessions.AddUserNamespacedSession(ctx, "app", user.ID, sessionID, s.sessionTTL); err != nil {
		_ = s.sessions.DeleteNamespacedSession(ctx, "app", sessionID)
		return nil, "", "", err
	}
	rawCookie, err := s.codec.Encode(sessionID)
	if err != nil {
		_ = s.removeAppSession(ctx, session)
		return nil, "", "", err
	}
	returnTo := normalizeReturnTo(payload["return_to"])
	return user, rawCookie, returnTo, nil
}

func (s *Service) MockGoogleLogin(ctx context.Context, email, name, returnTo string) (*model.User, string, string, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	displayName := strings.TrimSpace(name)
	if normalizedEmail == "" {
		return nil, "", "", fmt.Errorf("mock app email is required")
	}
	if displayName == "" {
		displayName = normalizedEmail
	}
	user, err := s.users.SaveGoogleUser(ctx, "mock-google-app-user", normalizedEmail, displayName, nil)
	if err != nil {
		return nil, "", "", err
	}
	if user.Status == model.UserStatusDisabled {
		return nil, "", "", &AccessDeniedError{Email: normalizedEmail, Reason: "user_disabled"}
	}
	sessionID := uuid.NewString()
	session := SessionPayload{SessionID: sessionID, UserID: user.ID, CreatedAt: time.Now().UTC()}
	if err := s.sessions.SaveNamespacedSession(ctx, "app", sessionID, session, s.sessionTTL); err != nil {
		return nil, "", "", err
	}
	if err := s.sessions.AddUserNamespacedSession(ctx, "app", user.ID, sessionID, s.sessionTTL); err != nil {
		_ = s.sessions.DeleteNamespacedSession(ctx, "app", sessionID)
		return nil, "", "", err
	}
	rawCookie, err := s.codec.Encode(sessionID)
	if err != nil {
		_ = s.removeAppSession(ctx, session)
		return nil, "", "", err
	}
	s.writeAuditLog(ctx, "app.google_mock_login", normalizedEmail, map[string]any{
		"email":   normalizedEmail,
		"name":    displayName,
		"user_id": user.ID,
	})
	return user, rawCookie, normalizeReturnTo(returnTo), nil
}

func (s *Service) ResolveSession(raw string) (*SessionPayload, error) {
	sessionID, err := s.codec.Decode(raw)
	if err != nil {
		return nil, err
	}
	var payload SessionPayload
	ok, err := s.sessions.LoadNamespacedSession(context.Background(), "app", sessionID, &payload)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("app session not found")
	}
	if s.users == nil {
		return &payload, nil
	}
	user, err := s.users.GetUserByID(context.Background(), payload.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Status == model.UserStatusDisabled {
		_ = s.removeAppSession(context.Background(), payload)
		return nil, fmt.Errorf("app session user is disabled")
	}
	return &payload, nil
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	sessionID, err := s.codec.Decode(raw)
	if err != nil {
		return err
	}
	var payload SessionPayload
	ok, loadErr := s.sessions.LoadNamespacedSession(ctx, "app", sessionID, &payload)
	if loadErr != nil {
		return loadErr
	}
	if ok {
		return s.removeAppSession(ctx, payload)
	}
	return s.sessions.DeleteNamespacedSession(ctx, "app", sessionID)
}

func (s *Service) Me(ctx context.Context, raw string) (*model.User, error) {
	payload, err := s.ResolveSession(raw)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("app session not found")
	}
	if s.users == nil {
		return nil, fmt.Errorf("user store is unavailable")
	}
	user, err := s.users.GetUserByID(ctx, payload.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Status == model.UserStatusDisabled {
		_ = s.removeAppSession(ctx, *payload)
		return nil, fmt.Errorf("app session user is disabled")
	}
	return user, nil
}

func (s *Service) RevokeUserSessions(ctx context.Context, userID uint64) error {
	if userID == 0 {
		return nil
	}
	return s.sessions.DeleteUserNamespacedSessions(ctx, "app", userID)
}

func MarshalAudit(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

type AccessDeniedError struct {
	Email  string
	Reason string
}

func (e *AccessDeniedError) Error() string {
	return "app account is not authorized"
}

type auditLogger interface {
	CreateAuditLog(ctx context.Context, action, targetType, targetID string, payload string) error
}

func (s *Service) writeAuditLog(ctx context.Context, action, targetID string, payload map[string]any) {
	s.writeAuditLogWithTarget(ctx, action, "google_account", targetID, payload)
}

func (s *Service) writeAuditLogWithTarget(ctx context.Context, action, targetType, targetID string, payload map[string]any) {
	logger, ok := s.users.(auditLogger)
	if !ok {
		return
	}
	_ = logger.CreateAuditLog(ctx, action, targetType, targetID, MarshalAudit(payload))
}

func (s *Service) removeAppSession(ctx context.Context, payload SessionPayload) error {
	if err := s.sessions.DeleteNamespacedSession(ctx, "app", payload.SessionID); err != nil {
		return err
	}
	return s.sessions.RemoveUserNamespacedSession(ctx, "app", payload.UserID, payload.SessionID)
}
