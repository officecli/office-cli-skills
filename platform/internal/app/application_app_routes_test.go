package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/officecli/officecli/platform/internal/appuser"
	"github.com/officecli/officecli/platform/internal/auth"
	"github.com/officecli/officecli/platform/internal/discordoauth"
	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
)

type overviewRouteStore struct {
	apiKeys      []model.APIKey
	usage        []model.UsageEvent
	user         *model.User
	rewardGrants []model.RewardGrant
	referrals    []model.UserReferral
	discord      *model.DiscordConnection
}

func (s *overviewRouteStore) CountUserAPIKeys(_ context.Context, _ uint64) (int64, error) {
	return int64(len(s.apiKeys)), nil
}

func (s *overviewRouteStore) FindAPIKeysByOwner(_ context.Context, _ uint64) ([]model.APIKey, error) {
	return s.apiKeys, nil
}

func (s *overviewRouteStore) ListAppUsageEvents(_ context.Context, _ uint64) ([]model.UsageEvent, error) {
	return s.usage, nil
}

func (s *overviewRouteStore) GetUserByID(_ context.Context, _ uint64) (*model.User, error) {
	return s.user, nil
}

func (s *overviewRouteStore) ListRewardGrantsByUser(_ context.Context, _ uint64) ([]model.RewardGrant, error) {
	return s.rewardGrants, nil
}

func (s *overviewRouteStore) ListReferralsByInviterUserID(_ context.Context, _ uint64) ([]model.UserReferral, error) {
	return s.referrals, nil
}

func (s *overviewRouteStore) FindDiscordConnectionByUserID(_ context.Context, _ uint64) (*model.DiscordConnection, error) {
	return s.discord, nil
}

func (s *overviewRouteStore) CreateAuditLog(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (s *overviewRouteStore) AppCreateAPIKey(_ context.Context, userID uint64, planName, _, prefix string) (*model.APIKey, error) {
	return &model.APIKey{ID: 1, OwnerUserID: &userID, PlanName: planName, KeyPrefix: prefix}, nil
}

func (s *overviewRouteStore) UpdateAPIKey(_ context.Context, _ uint64, _ map[string]any) error {
	return nil
}

func (s *overviewRouteStore) IsAPIKeyOwnedByUser(_ context.Context, _, _ uint64) (bool, error) {
	return true, nil
}

type overviewRouteBilling struct {
	orders  []model.Order
	pricing []model.PricingPack
}

func (b overviewRouteBilling) ListOrdersByUser(_ context.Context, _ uint64) ([]model.Order, error) {
	return b.orders, nil
}

func (b overviewRouteBilling) Pricing() []model.PricingPack {
	return b.pricing
}

type overviewSessionStore struct {
	payload auth.SessionPayload
}

func (s overviewSessionStore) SaveNamespacedSession(_ context.Context, _, _ string, _ any, _ time.Duration) error {
	return nil
}

func (s overviewSessionStore) LoadNamespacedSession(_ context.Context, namespace, sessionID string, dest any) (bool, error) {
	if namespace != "app" || sessionID != s.payload.SessionID {
		return false, nil
	}
	payload, ok := dest.(*auth.SessionPayload)
	if !ok {
		return false, nil
	}
	*payload = s.payload
	return true, nil
}

func (s overviewSessionStore) DeleteNamespacedSession(_ context.Context, _, _ string) error {
	return nil
}

type overviewCookieCodec struct{}

func (overviewCookieCodec) Encode(sessionID string) (string, error) {
	return "cookie:" + sessionID, nil
}
func (overviewCookieCodec) Decode(value string) (string, error) { return value[len("cookie:"):], nil }

type fakeDiscordOAuthRouteService struct {
	loginURL string
	result   *discordoauth.CallbackResult
	err      error
}

func (f fakeDiscordOAuthRouteService) LoginURL(_ context.Context, _ uint64, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.loginURL, nil
}

func (f fakeDiscordOAuthRouteService) HandleCallback(_ context.Context, _, _ string) (*discordoauth.CallbackResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestRegisterAppRoutesOverviewReturnsRewardReferralAndDiscordState(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{
		apiKeys:      []model.APIKey{{QuotaTotal: routeIntPtr(10), QuotaUsed: 4}},
		usage:        []model.UsageEvent{{ID: 1}, {ID: 2}},
		user:         &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{{AmountTotal: 9, AmountUsed: 3}},
		referrals: []model.UserReferral{
			{InvitedUserID: 100},
			{InvitedUserID: 101, ActivatedAt: routeTimePtr()},
		},
		discord: &model.DiscordConnection{UserID: 42, GuildMember: true},
	}
	appSvc := appuser.NewService(store, overviewRouteBilling{
		orders:  []model.Order{{ID: 11, PackKind: model.PackKindExternalGeneration}},
		pricing: []model.PricingPack{{Code: "external-500", AmountTotal: 2268, QuotaAmount: 500, PackKind: string(model.PackKindExternalGeneration)}},
	}, "salt")
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{})

	req := httptest.NewRequest(http.MethodGet, "/api/app/overview", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data appuser.Overview `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	overview := body.Data
	if overview.APIKeyCount != 1 || overview.TotalRemaining != 6 || overview.RewardRemaining != 6 {
		t.Fatalf("overview quotas = %+v", overview)
	}
	if overview.InviteCode != "invite-xyz" || overview.ReferralCount != 2 || overview.ActivatedReferralCount != 1 {
		t.Fatalf("overview referral state = %+v", overview)
	}
	if !overview.DiscordConnected || !overview.DiscordGuildMember {
		t.Fatalf("overview discord state = %+v", overview)
	}
	if overview.RecentUsageCount != 2 || overview.RecentOrdersCount != 1 || len(overview.Pricing) != 1 {
		t.Fatalf("overview activity = %+v", overview)
	}
}

type routeGrowthManager struct {
	connection   *model.DiscordConnection
	connectCalls int
	grantCalls   int
}

func (g *routeGrowthManager) ConnectDiscord(_ context.Context, userID uint64, discordUserID, username string, guildMember bool) (*model.DiscordConnection, error) {
	g.connectCalls++
	if g.connection == nil {
		g.connection = &model.DiscordConnection{
			UserID:        userID,
			DiscordUserID: discordUserID,
			Username:      username,
			GuildMember:   guildMember,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		}
	}
	g.connection.UserID = userID
	g.connection.DiscordUserID = discordUserID
	g.connection.Username = username
	g.connection.GuildMember = guildMember
	copied := *g.connection
	return &copied, nil
}

func (g *routeGrowthManager) GrantDiscordJoinReward(_ context.Context, _ uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error) {
	g.grantCalls++
	return &growthsvc.RewardGrantResult{
		Created: false,
		Grant: &model.RewardGrant{
			SourceType:     model.RewardSourceDiscordJoin,
			IdempotencyKey: "discord-join:test",
			AmountTotal:    rewardAmount,
		},
	}, nil
}

func TestRegisterAppRoutesDiscordConnectReturnsBlockedVerificationStatus(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 4, AmountUsed: 1},
		},
	}
	growth := &routeGrowthManager{}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt", growth)
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{})

	req := httptest.NewRequest(http.MethodPost, "/api/app/discord/connect", strings.NewReader(`{"discord_user_id":"discord-42","username":"officecli-user","guild_member":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data appuser.ConnectDiscordResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.Data.Connection.VerificationStatus != "verification_blocked" {
		t.Fatalf("unexpected verification status: %+v", body.Data.Connection)
	}
	if body.Data.Connection.VerificationBlockedReason == "" {
		t.Fatalf("missing blocker reason: %+v", body.Data.Connection)
	}
	if body.Data.RewardGranted {
		t.Fatalf("expected reward not granted: %+v", body.Data)
	}
	if growth.connectCalls != 1 || growth.grantCalls != 0 {
		t.Fatalf("unexpected growth calls: connect=%d grant=%d", growth.connectCalls, growth.grantCalls)
	}
}

func TestRegisterAppRoutesDiscordStatusReturnsCurrentSnapshot(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 7, AmountUsed: 2},
		},
		discord: &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   false,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		},
	}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt")
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{})

	req := httptest.NewRequest(http.MethodGet, "/api/app/discord/status", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data appuser.DiscordStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.Data.RewardRemaining != 5 {
		t.Fatalf("reward remaining = %d", body.Data.RewardRemaining)
	}
	if body.Data.Connection == nil || body.Data.Connection.VerificationStatus != "verification_blocked" {
		t.Fatalf("unexpected discord status: %+v", body.Data.Connection)
	}
}

func TestRegisterAppRoutesDiscordLoginRedirectsBackWhenOAuthUnavailable(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{user: &model.User{ID: 42, InviteCode: "invite-xyz"}}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt")
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{err: discordoauth.ErrDiscordOAuthNotConfigured})

	req := httptest.NewRequest(http.MethodGet, "/api/app/discord/login?return_to=%2Fapp", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "discord=oauth_not_configured") {
		t.Fatalf("unexpected redirect location = %s", location)
	}
}

func routeIntPtr(v int) *int { return &v }

func routeTimePtr() *time.Time {
	now := time.Now().UTC()
	return &now
}
