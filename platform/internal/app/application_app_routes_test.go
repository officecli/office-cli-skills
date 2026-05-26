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

	"github.com/officecli/officecli/platform/internal/apikey"
	"github.com/officecli/officecli/platform/internal/appuser"
	"github.com/officecli/officecli/platform/internal/auth"
	"github.com/officecli/officecli/platform/internal/discordoauth"
	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
)

type overviewRouteStore struct {
	apiKeys       []model.APIKey
	usage         []model.UsageEvent
	user          *model.User
	rewardGrants  []model.RewardGrant
	referrals     []model.UserReferral
	discord       *model.DiscordConnection
	creditGrants  map[string]*model.HostedCreditGrant
	hostedLedgers map[string]*model.UserHostedCreditLedger
	hostedAccount *model.UserHostedCreditAccount
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

func (s *overviewRouteStore) AppCreateAPIKey(_ context.Context, userID uint64, planName, _, prefix, ciphertext string) (*model.APIKey, error) {
	key := model.APIKey{ID: uint64(len(s.apiKeys) + 1), OwnerUserID: &userID, PlanName: planName, KeyPrefix: prefix, KeyCiphertext: &ciphertext, Status: model.APIKeyStatusActive, AllowedModes: "external_only"}
	s.apiKeys = append(s.apiKeys, key)
	return &key, nil
}

func (s *overviewRouteStore) FindHostedCreditGrantByIdempotencyKey(_ context.Context, key string) (*model.HostedCreditGrant, error) {
	if s.creditGrants == nil || s.creditGrants[key] == nil {
		return nil, nil
	}
	copied := *s.creditGrants[key]
	return &copied, nil
}

func (s *overviewRouteStore) GrantHostedCreditsToAPIKey(_ context.Context, apiKeyID, userID uint64, source model.HostedCreditGrantSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.HostedCreditGrant, bool, error) {
	if s.creditGrants == nil {
		s.creditGrants = map[string]*model.HostedCreditGrant{}
	}
	if grant := s.creditGrants[idempotencyKey]; grant != nil {
		copied := *grant
		return &copied, false, nil
	}
	for i := range s.apiKeys {
		if s.apiKeys[i].ID == apiKeyID {
			s.apiKeys[i].CreditBalance += creditAmount
			s.apiKeys[i].HostedEnabled = true
			s.apiKeys[i].AllowedModes = "hybrid"
			break
		}
	}
	grant := &model.HostedCreditGrant{ID: uint64(len(s.creditGrants) + 1), UserID: userID, APIKeyID: apiKeyID, SourceType: source, IdempotencyKey: idempotencyKey, CreditAmount: creditAmount, Reason: reason, MetadataJSON: metadataJSON}
	s.creditGrants[idempotencyKey] = grant
	copied := *grant
	return &copied, true, nil
}

func (s *overviewRouteStore) GetHostedCreditAccountByUser(_ context.Context, userID uint64) (*model.UserHostedCreditAccount, error) {
	if s.hostedAccount == nil {
		return &model.UserHostedCreditAccount{UserID: userID}, nil
	}
	copied := *s.hostedAccount
	return &copied, nil
}

func (s *overviewRouteStore) GrantHostedCreditsToUser(_ context.Context, userID uint64, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.UserHostedCreditLedger, *model.UserHostedCreditAccount, bool, error) {
	if s.hostedLedgers == nil {
		s.hostedLedgers = map[string]*model.UserHostedCreditLedger{}
	}
	if ledger := s.hostedLedgers[idempotencyKey]; ledger != nil {
		copied := *ledger
		account, _ := s.GetHostedCreditAccountByUser(context.Background(), userID)
		return &copied, account, false, nil
	}
	if s.hostedAccount == nil {
		s.hostedAccount = &model.UserHostedCreditAccount{UserID: userID}
	}
	s.hostedAccount.CreditBalance += creditAmount
	ledger := &model.UserHostedCreditLedger{ID: uint64(len(s.hostedLedgers) + 1), UserID: userID, SourceType: source, IdempotencyKey: idempotencyKey, CreditDelta: creditAmount, Reason: reason, MetadataJSON: metadataJSON}
	s.hostedLedgers[idempotencyKey] = ledger
	copied := *ledger
	account := *s.hostedAccount
	return &copied, &account, true, nil
}

func (s *overviewRouteStore) GrantHostedCreditsToFingerprint(_ context.Context, fingerprint string, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.FingerprintCreditLedger, *model.FingerprintCreditAccount, bool, error) {
	ledger := &model.FingerprintCreditLedger{FingerprintHash: fingerprint, SourceType: source, IdempotencyKey: idempotencyKey, CreditDelta: creditAmount, Reason: reason, MetadataJSON: metadataJSON}
	account := &model.FingerprintCreditAccount{FingerprintHash: fingerprint, CreditBalance: creditAmount, BonusGranted: true}
	return ledger, account, true, nil
}

func (s *overviewRouteStore) TransferAnonymousCreditsToUser(_ context.Context, _ string, _ uint64) (int, error) {
	return 0, nil
}

func (s *overviewRouteStore) UpdateAPIKey(_ context.Context, _ uint64, _ map[string]any) error {
	return nil
}

func (s *overviewRouteStore) IsAPIKeyOwnedByUser(_ context.Context, _, _ uint64) (bool, error) {
	return true, nil
}

func (s *overviewRouteStore) ListUserHostedCreditLedger(_ context.Context, _ uint64, _ bool) ([]model.UserHostedCreditLedger, error) {
	return nil, nil
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

type routeAuthUserStore struct {
	user *model.User
}

func (s routeAuthUserStore) SaveGoogleUser(_ context.Context, googleSub, email, name string, avatarURL *string) (*model.User, error) {
	if s.user == nil {
		return nil, nil
	}
	copied := *s.user
	copied.GoogleSub = model.StringPtr(googleSub)
	copied.Email = email
	copied.Name = name
	copied.AvatarURL = avatarURL
	return &copied, nil
}

func (s routeAuthUserStore) SaveGitHubUser(_ context.Context, githubSub, email, name string, avatarURL *string) (*model.User, error) {
	if s.user == nil {
		return nil, nil
	}
	copied := *s.user
	copied.GitHubSub = model.StringPtr(githubSub)
	copied.Email = email
	copied.Name = name
	copied.AvatarURL = avatarURL
	return &copied, nil
}

func (s routeAuthUserStore) GetUserByID(_ context.Context, id uint64) (*model.User, error) {
	if s.user == nil || s.user.ID != id {
		return nil, nil
	}
	copied := *s.user
	return &copied, nil
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

func (s overviewSessionStore) AddUserNamespacedSession(_ context.Context, _ string, _ uint64, _ string, _ time.Duration) error {
	return nil
}

func (s overviewSessionStore) RemoveUserNamespacedSession(_ context.Context, _ string, _ uint64, _ string) error {
	return nil
}

func (s overviewSessionStore) DeleteUserNamespacedSessions(_ context.Context, _ string, _ uint64) error {
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
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{
		apiKeys:      []model.APIKey{{ID: 1, Status: model.APIKeyStatusActive, QuotaTotal: routeIntPtr(10), QuotaUsed: 4}},
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
	}, "salt", mustTestAPIKeyCipher(t))
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{}, nil)

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

func TestRegisterAppRoutesQuotaSummaryReturnsAccountQuotaAndTrialPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{
		apiKeys: []model.APIKey{
			{ID: 1, KeyPrefix: "cop_live_a", PlanName: "Starter", Status: model.APIKeyStatusActive, QuotaTotal: routeIntPtr(10), QuotaUsed: 4},
			{ID: 2, KeyPrefix: "cop_live_b", PlanName: "Team", Status: model.APIKeyStatusDisabled, QuotaTotal: routeIntPtr(20), QuotaUsed: 5},
		},
		rewardGrants: []model.RewardGrant{{AmountTotal: 9, AmountUsed: 3, Reason: "invite activation reward", SourceType: model.RewardSourceInviteActivation}},
	}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt", mustTestAPIKeyCipher(t))
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/app/quota-summary", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data appuser.QuotaSummary `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.Data.RewardQuota.Remaining != 6 {
		t.Fatalf("reward remaining = %+v", body.Data)
	}
	if body.Data.PaidExternalQuota.TotalRemaining != 21 {
		t.Fatalf("paid quota = %+v", body.Data)
	}
	if !body.Data.TrialPolicy.CLIBinaryOnly {
		t.Fatalf("trial policy = %+v", body.Data.TrialPolicy)
	}
	if len(body.Data.PaidExternalQuota.Keys) != 2 || len(body.Data.RewardQuota.Grants) != 1 {
		t.Fatalf("summary detail = %+v", body.Data)
	}
}

func TestRegisterAppRoutesRejectsDisabledSessionUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt", mustTestAPIKeyCipher(t))
	authSvc := auth.NewService(
		nil,
		routeAuthUserStore{user: &model.User{ID: 42, Status: model.UserStatusDisabled}},
		overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}},
		"cop_app_session",
		time.Hour,
		overviewCookieCodec{},
		nil,
		nil,
	)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/app/overview", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterAppRoutesAPIKeyPlaintextReturnsStoredKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cipher := mustTestAPIKeyCipher(t)
	ciphertext, err := cipher.Encrypt("cop_live_secret_123")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	store := &overviewRouteStore{
		apiKeys: []model.APIKey{{
			ID:            7,
			KeyPrefix:     "cop_live_demo",
			KeyCiphertext: &ciphertext,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Growth",
		}},
	}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt", cipher)
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/app/api-keys/7/plaintext", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data appuser.APIKeyPlaintextResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.Data.PlaintextKey != "cop_live_secret_123" {
		t.Fatalf("plaintext = %q", body.Data.PlaintextKey)
	}
}

func TestRegisterAppRoutesAPIKeyPlaintextRejectsLegacyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{
		apiKeys: []model.APIKey{{
			ID:        7,
			KeyPrefix: "cop_live_legacy",
			Status:    model.APIKeyStatusActive,
			PlanName:  "Legacy",
		}},
	}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt", mustTestAPIKeyCipher(t))
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/app/api-keys/7/plaintext", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
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
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 4, AmountUsed: 1},
		},
	}
	growth := &routeGrowthManager{}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt", mustTestAPIKeyCipher(t), growth)
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{}, nil)

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
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt", mustTestAPIKeyCipher(t))
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{}, nil)

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
	gin.SetMode(gin.TestMode)

	store := &overviewRouteStore{user: &model.User{ID: 42, InviteCode: "invite-xyz"}}
	appSvc := appuser.NewService(store, overviewRouteBilling{}, "salt", mustTestAPIKeyCipher(t))
	authSvc := auth.NewService(nil, nil, overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}}, "cop_app_session", time.Hour, overviewCookieCodec{}, nil, nil)

	router := gin.New()
	api := router.Group("/api")
	registerAppRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc, nil, fakeDiscordOAuthRouteService{err: discordoauth.ErrDiscordOAuthNotConfigured}, nil)

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

func mustTestAPIKeyCipher(t *testing.T) *apikey.Cipher {
	t.Helper()
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}
	return cipher
}
