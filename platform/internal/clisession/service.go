package clisession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

var (
	ErrInvalidChallenge     = errors.New("invalid cli login challenge")
	ErrChallengeExpired     = errors.New("cli login challenge expired")
	ErrChallengeNotComplete = errors.New("cli login challenge is not complete")
	ErrInvalidCodeVerifier  = errors.New("invalid cli login code verifier")
	ErrInvalidExchangeCode  = errors.New("invalid cli login exchange code")
	ErrInvalidSession       = errors.New("invalid cli session")
)

const (
	defaultChallengeTTL = 10 * time.Minute
	defaultSessionTTL   = 180 * 24 * time.Hour
)

type Store interface {
	CreateCLILoginChallenge(ctx context.Context, challenge *model.CLILoginChallenge) error
	GetCLILoginChallengeByChallengeID(ctx context.Context, challengeID string) (*model.CLILoginChallenge, error)
	CompleteCLILoginChallenge(ctx context.Context, challengeID string, userID uint64, exchangeCodeHash string, completedAt time.Time) (*model.CLILoginChallenge, error)
	ConsumeCLILoginChallenge(ctx context.Context, challengeID string, consumedAt time.Time) error
	CreateCLISession(ctx context.Context, session *model.CLISession) error
	FindCLISessionByTokenHash(ctx context.Context, tokenHash string) (*model.CLISession, error)
	TouchCLISession(ctx context.Context, id uint64, usedAt time.Time) error
	RevokeCLISession(ctx context.Context, id uint64, revokedAt time.Time) error
	RevokeCLISessionsByUser(ctx context.Context, userID uint64, revokedAt time.Time) error
	ListCLISessionsByUser(ctx context.Context, userID uint64) ([]model.CLISession, error)
}

type Service struct {
	store       Store
	platformURL string
	clock       func() time.Time
}

type StartRequest struct {
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	RedirectURI         string `json:"redirect_uri"`
	State               string `json:"state"`
}

type StartResponse struct {
	ChallengeID string    `json:"challenge_id"`
	LoginURL    string    `json:"login_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type ExchangeRequest struct {
	ChallengeID  string `json:"challenge_id"`
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
}

type ExchangeResponse struct {
	Token       string    `json:"token"`
	TokenPrefix string    `json:"token_prefix"`
	UserID      uint64    `json:"user_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type SessionResponse struct {
	Authenticated bool       `json:"authenticated"`
	UserID        uint64     `json:"user_id,omitempty"`
	TokenPrefix   string     `json:"token_prefix,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

func NewService(store Store, platformURL string) *Service {
	return &Service{store: store, platformURL: strings.TrimRight(strings.TrimSpace(platformURL), "/"), clock: time.Now}
}

func (s *Service) Start(ctx context.Context, req StartRequest) (*StartResponse, error) {
	if strings.TrimSpace(req.CodeChallengeMethod) != "S256" {
		return nil, fmt.Errorf("code_challenge_method must be S256")
	}
	if strings.TrimSpace(req.CodeChallenge) == "" || strings.TrimSpace(req.RedirectURI) == "" || strings.TrimSpace(req.State) == "" {
		return nil, fmt.Errorf("code_challenge, redirect_uri, and state are required")
	}
	challengeID := "cli_" + randomToken(24)
	expiresAt := s.clock().UTC().Add(defaultChallengeTTL)
	challenge := &model.CLILoginChallenge{
		ChallengeID:         challengeID,
		CodeChallenge:       strings.TrimSpace(req.CodeChallenge),
		CodeChallengeMethod: "S256",
		RedirectURI:         strings.TrimSpace(req.RedirectURI),
		State:               strings.TrimSpace(req.State),
		Status:              model.CLILoginChallengeStatusPending,
		ExpiresAt:           expiresAt,
	}
	if err := s.store.CreateCLILoginChallenge(ctx, challenge); err != nil {
		return nil, err
	}
	loginURL := s.platformURL + "/api/auth/google/login?return_to=" + url.QueryEscape("/api/cli/login/complete?challenge_id="+url.QueryEscape(challengeID))
	return &StartResponse{ChallengeID: challengeID, LoginURL: loginURL, ExpiresAt: expiresAt}, nil
}

func (s *Service) Complete(ctx context.Context, challengeID string, userID uint64) (string, string, error) {
	challenge, err := s.store.GetCLILoginChallengeByChallengeID(ctx, strings.TrimSpace(challengeID))
	if err != nil {
		return "", "", err
	}
	if challenge == nil {
		return "", "", ErrInvalidChallenge
	}
	if s.clock().UTC().After(challenge.ExpiresAt) {
		return "", "", ErrChallengeExpired
	}
	code := "code_" + randomToken(32)
	hash := sha256Hex(code)
	completed, err := s.store.CompleteCLILoginChallenge(ctx, challenge.ChallengeID, userID, hash, s.clock().UTC())
	if err != nil {
		return "", "", err
	}
	return redirectWithCode(completed.RedirectURI, code, completed.State), code, nil
}

func (s *Service) Exchange(ctx context.Context, req ExchangeRequest) (*ExchangeResponse, error) {
	challenge, err := s.store.GetCLILoginChallengeByChallengeID(ctx, strings.TrimSpace(req.ChallengeID))
	if err != nil {
		return nil, err
	}
	if challenge == nil || challenge.UserID == nil || challenge.ExchangeCodeHash == nil {
		return nil, ErrInvalidChallenge
	}
	if s.clock().UTC().After(challenge.ExpiresAt) {
		return nil, ErrChallengeExpired
	}
	if challenge.Status != model.CLILoginChallengeStatusCompleted {
		return nil, ErrChallengeNotComplete
	}
	if expectedS256(req.CodeVerifier) != challenge.CodeChallenge {
		return nil, ErrInvalidCodeVerifier
	}
	if sha256Hex(strings.TrimSpace(req.Code)) != *challenge.ExchangeCodeHash {
		return nil, ErrInvalidExchangeCode
	}
	token := "ocli_sess_" + randomToken(32)
	expiresAt := s.clock().UTC().Add(defaultSessionTTL)
	session := &model.CLISession{
		UserID:      *challenge.UserID,
		TokenHash:   HashToken(token),
		TokenPrefix: tokenPrefix(token),
		Name:        "OfficeCLI",
		ExpiresAt:   expiresAt,
	}
	if err := s.store.CreateCLISession(ctx, session); err != nil {
		return nil, err
	}
	if err := s.store.ConsumeCLILoginChallenge(ctx, challenge.ChallengeID, s.clock().UTC()); err != nil {
		return nil, err
	}
	return &ExchangeResponse{Token: token, TokenPrefix: session.TokenPrefix, UserID: session.UserID, ExpiresAt: expiresAt}, nil
}

func (s *Service) Resolve(ctx context.Context, token string) (*model.CLISession, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrInvalidSession
	}
	session, err := s.store.FindCLISessionByTokenHash(ctx, HashToken(token))
	if err != nil {
		return nil, err
	}
	if session == nil || session.RevokedAt != nil || s.clock().UTC().After(session.ExpiresAt) {
		return nil, ErrInvalidSession
	}
	_ = s.store.TouchCLISession(ctx, session.ID, s.clock().UTC())
	return session, nil
}

func (s *Service) Session(ctx context.Context, token string) (*SessionResponse, error) {
	session, err := s.Resolve(ctx, token)
	if err != nil {
		return &SessionResponse{Authenticated: false}, nil
	}
	return &SessionResponse{Authenticated: true, UserID: session.UserID, TokenPrefix: session.TokenPrefix, ExpiresAt: &session.ExpiresAt}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	session, err := s.Resolve(ctx, token)
	if err != nil {
		return nil
	}
	return s.store.RevokeCLISession(ctx, session.ID, s.clock().UTC())
}

func (s *Service) StoreSessions(ctx context.Context, userID uint64) ([]model.CLISession, error) {
	return s.store.ListCLISessionsByUser(ctx, userID)
}

func (s *Service) RevokeUserSession(ctx context.Context, userID, sessionID uint64) error {
	sessions, err := s.store.ListCLISessionsByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.ID == sessionID {
			return s.store.RevokeCLISession(ctx, sessionID, s.clock().UTC())
		}
	}
	return ErrInvalidSession
}

func HashToken(token string) string {
	return sha256Hex(strings.TrimSpace(token))
}

func expectedS256(verifier string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(verifier)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomToken(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func tokenPrefix(token string) string {
	if len(token) <= 16 {
		return token
	}
	return token[:16]
}

func redirectWithCode(rawURI, code, state string) string {
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return rawURI
	}
	values := parsed.Query()
	values.Set("code", code)
	values.Set("state", state)
	parsed.RawQuery = values.Encode()
	return parsed.String()
}
