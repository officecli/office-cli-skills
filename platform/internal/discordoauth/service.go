package discordoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	growthsvc "github.com/officecli/officecli-internal/platform/internal/growth"
	"github.com/officecli/officecli-internal/platform/internal/model"
)

var (
	ErrDiscordOAuthNotConfigured      = errors.New("discord oauth is not configured")
	ErrDiscordGuildCheckNotConfigured = errors.New("discord guild verification is not configured")
	ErrDiscordOAuthStateInvalid       = errors.New("invalid discord oauth state")
)

const (
	stateNamespace                 = "discord_oauth_state"
	defaultDiscordJoinRewardAmount = 2
)

type SessionStore interface {
	SaveNamespacedSession(ctx context.Context, namespace, sessionID string, payload any, ttl time.Duration) error
	LoadNamespacedSession(ctx context.Context, namespace, sessionID string, dest any) (bool, error)
	DeleteNamespacedSession(ctx context.Context, namespace, sessionID string) error
}

type GrowthManager interface {
	ConnectDiscord(ctx context.Context, userID uint64, discordUserID, username string, guildMember bool) (*model.DiscordConnection, error)
	GrantDiscordJoinReward(ctx context.Context, userID uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error)
}

type Client interface {
	OAuthConfigured() bool
	GuildCheckConfigured() bool
	AuthCodeURL(state string) string
	ExchangeUser(ctx context.Context, code string) (*User, error)
	VerifyGuildMembership(ctx context.Context, userID string) (bool, error)
}

type User struct {
	ID         string
	Username   string
	GlobalName string
}

type StatePayload struct {
	UserID   uint64 `json:"user_id"`
	ReturnTo string `json:"return_to"`
}

type CallbackResult struct {
	ReturnTo                  string
	Connection                *model.DiscordConnection
	RewardGranted             bool
	VerificationStatus        string
	VerificationBlockedReason string
}

type Service struct {
	client   Client
	sessions SessionStore
	growth   GrowthManager
}

func NewService(client Client, sessions SessionStore, growth GrowthManager) *Service {
	return &Service{client: client, sessions: sessions, growth: growth}
}

func (s *Service) LoginURL(ctx context.Context, userID uint64, returnTo string) (string, error) {
	if s.client == nil || !s.client.OAuthConfigured() {
		return "", ErrDiscordOAuthNotConfigured
	}
	state := fmt.Sprintf("discord-%d-%d", userID, time.Now().UTC().UnixNano())
	payload := StatePayload{UserID: userID, ReturnTo: normalizeReturnTo(returnTo)}
	if err := s.sessions.SaveNamespacedSession(ctx, stateNamespace, state, payload, 10*time.Minute); err != nil {
		return "", err
	}
	return s.client.AuthCodeURL(state), nil
}

func (s *Service) HandleCallback(ctx context.Context, code, state string) (*CallbackResult, error) {
	if s.client == nil || !s.client.OAuthConfigured() {
		return nil, ErrDiscordOAuthNotConfigured
	}

	var payload StatePayload
	ok, err := s.sessions.LoadNamespacedSession(ctx, stateNamespace, state, &payload)
	if err != nil || !ok {
		return nil, ErrDiscordOAuthStateInvalid
	}
	_ = s.sessions.DeleteNamespacedSession(ctx, stateNamespace, state)

	user, err := s.client.ExchangeUser(ctx, code)
	if err != nil {
		return nil, err
	}

	guildMember := false
	verificationStatus := "verification_blocked"
	blockedReason := ErrDiscordGuildCheckNotConfigured.Error()
	if s.client.GuildCheckConfigured() {
		guildMember, err = s.client.VerifyGuildMembership(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		if guildMember {
			verificationStatus = "verified"
			blockedReason = ""
		}
	}

	connection, err := s.growth.ConnectDiscord(ctx, payload.UserID, user.ID, user.DisplayName(), guildMember)
	if err != nil {
		return nil, err
	}

	result := &CallbackResult{
		ReturnTo:                  normalizeReturnTo(payload.ReturnTo),
		Connection:                connection,
		VerificationStatus:        verificationStatus,
		VerificationBlockedReason: blockedReason,
	}

	if guildMember {
		grant, err := s.growth.GrantDiscordJoinReward(ctx, payload.UserID, defaultDiscordJoinRewardAmount)
		if err != nil && !errors.Is(err, growthsvc.ErrDiscordGuildRequired) {
			return nil, err
		}
		if grant != nil {
			result.RewardGranted = grant.Created
		}
	}

	return result, nil
}

type OAuthClient struct {
	config   *oauth2.Config
	guildID  string
	botToken string
	client   *http.Client
}

func NewOAuthClient(clientID, clientSecret, redirectURL, guildID, botToken string) *OAuthClient {
	return &OAuthClient{
		config: &oauth2.Config{
			ClientID:     strings.TrimSpace(clientID),
			ClientSecret: strings.TrimSpace(clientSecret),
			RedirectURL:  strings.TrimSpace(redirectURL),
			Scopes:       []string{"identify"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://discord.com/oauth2/authorize",
				TokenURL: "https://discord.com/api/oauth2/token",
			},
		},
		guildID:  strings.TrimSpace(guildID),
		botToken: strings.TrimSpace(botToken),
		client:   http.DefaultClient,
	}
}

func (c *OAuthClient) OAuthConfigured() bool {
	return c != nil &&
		c.config != nil &&
		strings.TrimSpace(c.config.ClientID) != "" &&
		strings.TrimSpace(c.config.ClientSecret) != "" &&
		strings.TrimSpace(c.config.RedirectURL) != ""
}

func (c *OAuthClient) GuildCheckConfigured() bool {
	return c != nil && strings.TrimSpace(c.guildID) != "" && strings.TrimSpace(c.botToken) != ""
}

func (c *OAuthClient) AuthCodeURL(state string) string {
	return c.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (c *OAuthClient) ExchangeUser(ctx context.Context, code string) (*User, error) {
	token, err := c.config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	client := c.config.Client(ctx, token)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/users/@me", nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord user lookup failed with status %d", response.StatusCode)
	}
	var payload struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.ID == "" || payload.Username == "" {
		return nil, fmt.Errorf("discord user profile is incomplete")
	}
	return &User{ID: payload.ID, Username: payload.Username, GlobalName: payload.GlobalName}, nil
}

func (c *OAuthClient) VerifyGuildMembership(ctx context.Context, userID string) (bool, error) {
	if !c.GuildCheckConfigured() {
		return false, ErrDiscordGuildCheckNotConfigured
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://discord.com/api/guilds/%s/members/%s", c.guildID, userID), nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bot "+c.botToken)
	response, err := c.client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("discord guild member lookup failed with status %d", response.StatusCode)
	}
}

func (u *User) DisplayName() string {
	if strings.TrimSpace(u.GlobalName) != "" {
		return strings.TrimSpace(u.GlobalName)
	}
	return strings.TrimSpace(u.Username)
}

func normalizeReturnTo(returnTo string) string {
	if strings.TrimSpace(returnTo) == "" {
		return "/app"
	}
	return returnTo
}
