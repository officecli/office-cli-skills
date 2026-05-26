package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/officecli/officecli/platform/internal/apikey"
	"github.com/officecli/officecli/platform/internal/auth"
	"github.com/officecli/officecli/platform/internal/model"
	"github.com/officecli/officecli/platform/internal/redemption"
	redisstore "github.com/officecli/officecli/platform/internal/store/redis"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type SessionPayload struct {
	SessionID      string    `json:"session_id"`
	CreatedAt      time.Time `json:"created_at"`
	PrincipalEmail string    `json:"principal_email,omitempty"`
	PrincipalName  string    `json:"principal_name,omitempty"`
	AuthMethod     string    `json:"auth_method,omitempty"`
}

type Service struct {
	store              *sqlstore.Store
	redis              *redisstore.Store
	adminPassword      string
	sessionTTL         time.Duration
	sessionCookieCodec CookieCodec
	cookieName         string
	apiKeySalt         string
	apiKeyCipher       *apikey.Cipher
	oauthProvider      auth.OAuthProvider
	adminAllowlist     map[string]struct{}
	hostedPricing      HostedPricingManager
	redemption         *redemption.Service
	mockData           bool
}

type GrowthSnapshot struct {
	RewardGrants       []model.RewardGrant       `json:"reward_grants"`
	HostedCreditGrants []model.HostedCreditGrant `json:"hosted_credit_grants"`
	Referrals          []model.UserReferral      `json:"referrals"`
	DiscordConnections []model.DiscordConnection `json:"discord_connections"`
}

func (s *Service) UseMockData(enabled bool) {
	s.mockData = enabled
}

type QuotaSources struct {
	RewardGrants     []model.RewardGrant `json:"reward_grants"`
	PaidExternalKeys []model.APIKey      `json:"paid_external_keys"`
	HostedKeys       []model.APIKey      `json:"hosted_keys"`
}

type HostedPricingManager interface {
	HostedPricingRules() []model.HostedPricingRule
}

type CookieCodec interface {
	Encode(sessionID string) (string, error)
	Decode(value string) (string, error)
}

func NewService(store *sqlstore.Store, redis *redisstore.Store, adminPassword string, sessionTTL time.Duration, cookieName string, codec CookieCodec, apiKeySalt string, apiKeyCipher *apikey.Cipher, oauthProvider auth.OAuthProvider, adminAllowlist []string, hostedPricing ...HostedPricingManager) *Service {
	allowlist := make(map[string]struct{}, len(adminAllowlist))
	for _, email := range adminAllowlist {
		normalized := strings.ToLower(strings.TrimSpace(email))
		if normalized == "" {
			continue
		}
		allowlist[normalized] = struct{}{}
	}
	var hostedPricingManager HostedPricingManager
	if len(hostedPricing) > 0 {
		hostedPricingManager = hostedPricing[0]
	}
	return &Service{
		store:              store,
		redis:              redis,
		adminPassword:      adminPassword,
		sessionTTL:         sessionTTL,
		cookieName:         cookieName,
		sessionCookieCodec: codec,
		apiKeySalt:         apiKeySalt,
		apiKeyCipher:       apiKeyCipher,
		oauthProvider:      oauthProvider,
		adminAllowlist:     allowlist,
		hostedPricing:      hostedPricingManager,
	}
}

func (s *Service) ResolveSession(raw string) (string, error) {
	sessionID, err := s.sessionCookieCodec.Decode(raw)
	if err != nil {
		return "", err
	}
	var payload SessionPayload
	found, err := s.redis.LoadSession(context.Background(), sessionID, &payload)
	if err != nil || !found {
		return "", err
	}
	return sessionID, nil
}

func (s *Service) Login(ctx context.Context, password string) (string, error) {
	if strings.TrimSpace(password) == "" || password != s.adminPassword {
		return "", fmt.Errorf("invalid admin password")
	}
	return s.createSession(ctx, SessionPayload{PrincipalEmail: "admin", PrincipalName: "Admin", AuthMethod: "password"}, "admin.login")
}

func (s *Service) MockGoogleLogin(ctx context.Context, email, name string) (*AdminIdentity, string, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	displayName := strings.TrimSpace(name)
	if normalizedEmail == "" {
		return nil, "", fmt.Errorf("mock admin email is required")
	}
	if displayName == "" {
		displayName = normalizedEmail
	}
	raw, err := s.createSession(ctx, SessionPayload{
		PrincipalEmail: normalizedEmail,
		PrincipalName:  displayName,
		AuthMethod:     "google_mock",
	}, "admin.google_mock_login")
	if err != nil {
		return nil, "", err
	}
	return &AdminIdentity{Email: normalizedEmail, Name: displayName, AuthMethod: "google_mock"}, raw, nil
}

func (s *Service) LoginURL(ctx context.Context, returnTo string) (string, error) {
	if s.oauthProvider == nil {
		return "", fmt.Errorf("admin google oauth is not configured")
	}
	state := uuid.NewString()
	payload := map[string]string{"return_to": returnTo}
	if err := s.redis.SaveNamespacedSession(ctx, "admin_oauth_state", state, payload, 10*time.Minute); err != nil {
		return "", err
	}
	return s.oauthProvider.AuthCodeURL(state), nil
}

func (s *Service) HandleGoogleCallback(ctx context.Context, code, state string) (*AdminIdentity, string, string, error) {
	if s.oauthProvider == nil {
		return nil, "", "", fmt.Errorf("admin google oauth is not configured")
	}

	var payload map[string]string
	ok, err := s.redis.LoadNamespacedSession(ctx, "admin_oauth_state", state, &payload)
	if err != nil || !ok {
		return nil, "", "", fmt.Errorf("invalid oauth state")
	}
	_ = s.redis.DeleteNamespacedSession(ctx, "admin_oauth_state", state)

	googleUser, err := s.oauthProvider.Exchange(ctx, code)
	if err != nil {
		return nil, "", "", err
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(googleUser.Email))
	if _, allowed := s.adminAllowlist[normalizedEmail]; !allowed {
		_ = s.store.CreateAuditLog(ctx, "admin.google_login_denied", "google_account", normalizedEmail, sqlstore.JSONString(map[string]any{
			"email": normalizedEmail,
			"name":  googleUser.Name,
		}))
		return nil, "", "", &AccessDeniedError{Email: normalizedEmail}
	}

	sessionID := uuid.NewString()
	session := SessionPayload{
		SessionID:      sessionID,
		CreatedAt:      time.Now().UTC(),
		PrincipalEmail: normalizedEmail,
		PrincipalName:  googleUser.Name,
		AuthMethod:     "google",
	}
	if err := s.redis.SaveSession(ctx, sessionID, session, s.sessionTTL); err != nil {
		return nil, "", "", err
	}
	rawCookie, err := s.sessionCookieCodec.Encode(sessionID)
	if err != nil {
		return nil, "", "", err
	}
	_ = s.store.CreateAuditLog(ctx, "admin.google_login", "session", sessionID, sqlstore.JSONString(map[string]any{
		"email":       normalizedEmail,
		"name":        googleUser.Name,
		"auth_method": "google",
	}))

	returnTo := "/admin"
	if redirectTarget := strings.TrimSpace(payload["return_to"]); redirectTarget != "" {
		returnTo = redirectTarget
	}
	return &AdminIdentity{Email: normalizedEmail, Name: googleUser.Name, AuthMethod: "google"}, rawCookie, returnTo, nil
}

func (s *Service) createSession(ctx context.Context, payload SessionPayload, auditAction string) (string, error) {
	sessionID := uuid.NewString()
	payload.SessionID = sessionID
	payload.CreatedAt = time.Now().UTC()
	if err := s.redis.SaveSession(ctx, sessionID, payload, s.sessionTTL); err != nil {
		return "", err
	}
	raw, err := s.sessionCookieCodec.Encode(sessionID)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(payload)
	_ = s.store.CreateAuditLog(ctx, auditAction, "session", sessionID, string(body))
	return raw, nil
}

func (s *Service) CurrentIdentity(ctx context.Context, rawCookie string) (*AdminIdentity, error) {
	sessionID, err := s.sessionCookieCodec.Decode(rawCookie)
	if err != nil {
		return nil, err
	}
	var payload SessionPayload
	found, err := s.redis.LoadSession(ctx, sessionID, &payload)
	if err != nil || !found {
		return nil, err
	}
	return &AdminIdentity{
		Email:      payload.PrincipalEmail,
		Name:       payload.PrincipalName,
		AuthMethod: payload.AuthMethod,
	}, nil
}

func (s *Service) Logout(ctx context.Context, rawCookie string) error {
	sessionID, err := s.sessionCookieCodec.Decode(rawCookie)
	if err != nil {
		return err
	}
	if err := s.redis.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	_ = s.store.CreateAuditLog(ctx, "admin.logout", "session", sessionID, "{}")
	return nil
}

func (s *Service) Overview(ctx context.Context) (*model.OverviewStats, error) {
	if s.mockData {
		return mockOverview(), nil
	}
	return s.store.Overview(ctx)
}

func (s *Service) FingerprintQuality(ctx context.Context) (*model.FingerprintQuality, error) {
	if s.mockData {
		return &model.FingerprintQuality{}, nil
	}
	return s.store.FingerprintQuality(ctx)
}

func (s *Service) OperationsFunnel(ctx context.Context, windowStart, now time.Time) (*model.OperationsFunnel, error) {
	if s.mockData {
		return &model.OperationsFunnel{WindowStart: windowStart, WindowEnd: now}, nil
	}
	return s.store.OperationsFunnel(ctx, windowStart, now)
}

func (s *Service) ListAPIKeys(ctx context.Context, ownerUserID *uint64) ([]model.APIKey, error) {
	if s.mockData {
		return mockAPIKeys(ownerUserID), nil
	}
	if ownerUserID != nil {
		return s.store.FindAPIKeysByOwner(ctx, *ownerUserID)
	}
	return s.store.ListAPIKeys(ctx)
}

func (s *Service) CreateAPIKey(ctx context.Context, req CreateAPIKeyRequest) (*CreateAPIKeyResponse, *model.APIKey, error) {
	if err := s.validateAPIKeyOwner(ctx, req); err != nil {
		return nil, nil, err
	}
	plain, prefix, hash, err := generateAPIKey(s.apiKeySalt)
	if err != nil {
		return nil, nil, err
	}
	ciphertext, err := s.apiKeyCipher.Encrypt(plain)
	if err != nil {
		return nil, nil, err
	}
	key, err := s.store.AdminCreateAPIKey(ctx, req.OwnerUserID, req.PlanName, req.ExpiresAt, req.Note, req.PlanCode, hash, prefix, ciphertext, req.QuotaTotal)
	if err != nil {
		return nil, nil, err
	}
	updates := map[string]any{}
	if req.AllowedModes != nil {
		updates["allowed_modes"] = *req.AllowedModes
	}
	if req.HostedEnabled != nil {
		updates["hosted_enabled"] = *req.HostedEnabled
	}
	if req.DefaultRuntimeMode != nil {
		updates["default_runtime_mode"] = *req.DefaultRuntimeMode
	}
	if req.CreditBalance != nil {
		updates["credit_balance"] = *req.CreditBalance
	}
	if len(updates) > 0 {
		if err := s.store.UpdateAPIKey(ctx, key.ID, updates); err != nil {
			return nil, nil, err
		}
		key, err = s.store.FindAPIKeyByID(ctx, key.ID)
		if err != nil {
			return nil, nil, err
		}
	}
	payload, _ := json.Marshal(key)
	_ = s.store.CreateAuditLog(ctx, "api_key.create", "api_key", fmt.Sprintf("%d", key.ID), string(payload))
	return &CreateAPIKeyResponse{PlaintextKey: plain, KeyPrefix: prefix}, key, nil
}

func (s *Service) validateAPIKeyOwner(ctx context.Context, req CreateAPIKeyRequest) error {
	if !createAPIKeyRequiresOwner(req) {
		return nil
	}
	if req.OwnerUserID == nil || *req.OwnerUserID == 0 {
		return fmt.Errorf("owner_user_id is required for hosted API keys")
	}
	user, err := s.store.GetUserByID(ctx, *req.OwnerUserID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("owner user not found: %d", *req.OwnerUserID)
	}
	return nil
}

func createAPIKeyRequiresOwner(req CreateAPIKeyRequest) bool {
	if req.HostedEnabled != nil && *req.HostedEnabled {
		return true
	}
	if req.AllowedModes == nil {
		return false
	}
	switch strings.TrimSpace(*req.AllowedModes) {
	case "hosted_only", "hybrid":
		return true
	default:
		return false
	}
}

func (s *Service) GetAPIKeyPlaintext(ctx context.Context, id uint64, actor string) (string, error) {
	key, err := s.store.FindAPIKeyByID(ctx, id)
	if err != nil {
		return "", err
	}
	if key == nil {
		return "", apikey.ErrAPIKeyNotFound
	}
	if !key.HasStoredPlaintext() {
		return "", apikey.ErrPlaintextUnavailable
	}
	plaintext, err := s.apiKeyCipher.Decrypt(*key.KeyCiphertext)
	if err != nil {
		return "", err
	}
	payload := sqlstore.JSONString(map[string]any{
		"actor":   strings.TrimSpace(actor),
		"surface": "admin",
	})
	_ = s.store.CreateAuditLog(ctx, "api_key.reveal", "api_key", fmt.Sprintf("%d", id), payload)
	return plaintext, nil
}

func (s *Service) UpdateAPIKey(ctx context.Context, id uint64, req UpdateAPIKeyRequest) error {
	updates := map[string]any{}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.PlanName != nil {
		updates["plan_name"] = *req.PlanName
	}
	if req.AllowedModes != nil {
		updates["allowed_modes"] = *req.AllowedModes
	}
	if req.HostedEnabled != nil {
		updates["hosted_enabled"] = *req.HostedEnabled
	}
	if req.DefaultRuntimeMode != nil {
		updates["default_runtime_mode"] = *req.DefaultRuntimeMode
	}
	if req.OwnerUserID != nil {
		updates["owner_user_id"] = *req.OwnerUserID
	}
	if req.Note != nil {
		updates["note"] = *req.Note
	}
	if req.PlanCode != nil {
		updates["plan_code"] = *req.PlanCode
	}
	if req.ExpiresAt != nil {
		updates["expires_at"] = *req.ExpiresAt
	}
	if req.QuotaTotal != nil {
		updates["quota_total"] = *req.QuotaTotal
	}
	if req.QuotaUsed != nil {
		updates["quota_used"] = *req.QuotaUsed
	}
	if req.CreditBalance != nil {
		updates["credit_balance"] = *req.CreditBalance
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.store.UpdateAPIKey(ctx, id, updates); err != nil {
		return err
	}
	payload, _ := json.Marshal(updates)
	return s.store.CreateAuditLog(ctx, "api_key.update", "api_key", fmt.Sprintf("%d", id), string(payload))
}

func (s *Service) ListUsageEvents(ctx context.Context, filter sqlstore.UsageEventFilter) ([]model.UsageEvent, error) {
	if s.mockData {
		return mockUsageEvents(filter), nil
	}
	return s.store.ListUsageEvents(ctx, filter)
}

func (s *Service) GetPreference(ctx context.Context, adminEmail, pageKey string) (*model.AdminUserPreference, error) {
	if strings.TrimSpace(adminEmail) == "" {
		return nil, fmt.Errorf("admin email is required")
	}
	return s.store.GetAdminUserPreference(ctx, adminEmail, pageKey)
}

func (s *Service) SavePreference(ctx context.Context, adminEmail, pageKey, preferencesJSON string) (*model.AdminUserPreference, error) {
	if strings.TrimSpace(adminEmail) == "" {
		return nil, fmt.Errorf("admin email is required")
	}
	var payload any
	if err := json.Unmarshal([]byte(preferencesJSON), &payload); err != nil {
		return nil, fmt.Errorf("preferences must be valid JSON")
	}
	preference, err := s.store.UpsertAdminUserPreference(ctx, adminEmail, pageKey, preferencesJSON)
	if err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "admin.preference.update", "admin_user_preference", strings.ToLower(strings.TrimSpace(adminEmail))+":"+strings.TrimSpace(pageKey), preferencesJSON)
	return preference, nil
}

func (s *Service) ListUsers(ctx context.Context, query string) ([]model.User, error) {
	if s.mockData {
		return mockUsers(query), nil
	}
	return s.store.ListUsers(ctx, query)
}

func (s *Service) UpdateUser(ctx context.Context, id uint64, req UpdateUserRequest) error {
	updates := map[string]any{}
	disableUser := false
	if req.Status != nil {
		updates["status"] = *req.Status
		disableUser = model.UserStatus(strings.TrimSpace(*req.Status)) == model.UserStatusDisabled
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.store.UpdateUser(ctx, id, updates); err != nil {
		return err
	}
	if disableUser {
		if err := s.store.DisableAPIKeysByOwnerUserID(ctx, id); err != nil {
			return err
		}
		if s.redis != nil {
			_ = s.redis.DeleteUserNamespacedSessions(ctx, "app", id)
		}
	}
	return s.store.CreateAuditLog(ctx, "user.update", "user", fmt.Sprintf("%d", id), sqlstore.JSONString(updates))
}

func (s *Service) ListOrders(ctx context.Context) ([]model.Order, error) {
	if s.mockData {
		return mockOrders(), nil
	}
	return s.store.ListOrders(ctx)
}

func (s *Service) UpdateOrder(ctx context.Context, id uint64, req UpdateOrderRequest) error {
	updates := map[string]any{}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Note != nil {
		updates["note"] = *req.Note
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.store.UpdateOrder(ctx, id, updates); err != nil {
		return err
	}
	return s.store.CreateAuditLog(ctx, "order.update", "order", fmt.Sprintf("%d", id), sqlstore.JSONString(updates))
}

func (s *Service) ListBillingEvents(ctx context.Context) ([]model.BillingEvent, error) {
	if s.mockData {
		return mockBillingEvents(), nil
	}
	return s.store.ListBillingEvents(ctx)
}

func (s *Service) ListCreditLedger(ctx context.Context, includeZeroDelta bool) ([]model.UserHostedCreditLedger, error) {
	return s.store.ListAllUserHostedCreditLedger(ctx, includeZeroDelta)
}

func (s *Service) Growth(ctx context.Context) (*GrowthSnapshot, error) {
	if s.mockData {
		return mockGrowth(), nil
	}
	rewardGrants, err := s.store.ListRewardGrants(ctx)
	if err != nil {
		return nil, err
	}
	hostedCreditGrants, err := s.store.ListHostedCreditGrants(ctx)
	if err != nil {
		return nil, err
	}
	referrals, err := s.store.ListReferrals(ctx)
	if err != nil {
		return nil, err
	}
	discordConnections, err := s.store.ListDiscordConnections(ctx)
	if err != nil {
		return nil, err
	}
	return &GrowthSnapshot{
		RewardGrants:       rewardGrants,
		HostedCreditGrants: hostedCreditGrants,
		Referrals:          referrals,
		DiscordConnections: discordConnections,
	}, nil
}

func (s *Service) QuotaSources(ctx context.Context, filter QuotaSourcesFilter) (*QuotaSources, error) {
	if s.mockData {
		return mockQuotaSources(filter), nil
	}
	rewardGrants, err := s.store.ListRewardGrants(ctx)
	if err != nil {
		return nil, err
	}
	apiKeys, err := s.store.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}

	result := &QuotaSources{
		RewardGrants:     make([]model.RewardGrant, 0, len(rewardGrants)),
		PaidExternalKeys: make([]model.APIKey, 0, len(apiKeys)),
		HostedKeys:       make([]model.APIKey, 0, len(apiKeys)),
	}

	for _, grant := range rewardGrants {
		if filter.UserID != 0 && grant.UserID != filter.UserID {
			continue
		}
		result.RewardGrants = append(result.RewardGrants, grant)
	}
	for _, key := range apiKeys {
		if filter.UserID != 0 {
			if key.OwnerUserID == nil || *key.OwnerUserID != filter.UserID {
				continue
			}
		}
		if filter.KeyPrefix != "" && !strings.Contains(strings.ToLower(key.KeyPrefix), strings.ToLower(filter.KeyPrefix)) {
			continue
		}
		if key.SupportsHosted() || key.CreditBalance > 0 {
			result.HostedKeys = append(result.HostedKeys, key)
		}
		if key.QuotaTotal != nil {
			result.PaidExternalKeys = append(result.PaidExternalKeys, key)
		}
	}

	return result, nil
}

func (s *Service) HostedPricingRules(ctx context.Context) ([]model.HostedPricingRule, error) {
	if s.mockData {
		return mockHostedPricingRules(), nil
	}
	return s.store.ListHostedPricingRules(ctx, false)
}

func (s *Service) HostedBillingConfig(ctx context.Context) (*HostedBillingConfig, error) {
	if s.mockData {
		return mockHostedBillingConfig(), nil
	}
	settings, err := s.store.HostedPricingSettings(ctx)
	if err != nil {
		return nil, err
	}
	settings.CreditsPerUSD = normalizeCreditsPerUSD(settings.CreditsPerUSD)
	modelConfigs, err := s.store.ListHostedModelPricingConfigs(ctx, false)
	if err != nil {
		return nil, err
	}
	modelConfigs = hostedModelPricingConfigsWithCredits(modelConfigs, settings.CreditsPerUSD)
	rules, err := s.store.ListHostedPricingRules(ctx, false)
	if err != nil {
		return nil, err
	}
	packs, err := s.store.ListHostedCreditPacks(ctx, false)
	if err != nil {
		return nil, err
	}
	return &HostedBillingConfig{Settings: *settings, ModelConfigs: modelConfigs, Rules: rules, Packs: packs}, nil
}

func (s *Service) UpdateHostedPricingSettings(ctx context.Context, req UpdateHostedPricingSettingsRequest) (*model.HostedPricingSetting, error) {
	currency := strings.ToLower(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "usd"
	}
	creditsPerUSD := req.CreditsPerUSD
	settings, err := s.store.UpdateHostedPricingSettings(ctx, model.HostedPricingSetting{MarkupBPS: req.MarkupBPS, Currency: currency, CreditsPerUSD: creditsPerUSD})
	if err != nil {
		return nil, err
	}
	settings.CreditsPerUSD = normalizeCreditsPerUSD(settings.CreditsPerUSD)
	_ = s.store.CreateAuditLog(ctx, "hosted_pricing.settings.update", "hosted_pricing_settings", fmt.Sprintf("%d", settings.ID), sqlstore.JSONString(settings))
	return settings, nil
}

func (s *Service) CreateHostedModelPricingConfig(ctx context.Context, req UpsertHostedModelPricingConfigRequest) (*model.HostedModelPricingConfig, error) {
	creditsPerUSD := s.currentCreditsPerUSD(ctx)
	config := hostedModelPricingConfigFromRequest(req, creditsPerUSD)
	if err := s.store.CreateHostedModelPricingConfig(ctx, &config); err != nil {
		return nil, err
	}
	config = hostedModelPricingConfigWithCredits(config, creditsPerUSD)
	_ = s.store.CreateAuditLog(ctx, "hosted_pricing.model_config.create", "hosted_model_pricing_config", fmt.Sprintf("%d", config.ID), sqlstore.JSONString(config))
	return &config, nil
}

func (s *Service) UpdateHostedModelPricingConfig(ctx context.Context, id uint64, req UpsertHostedModelPricingConfigRequest) (*model.HostedModelPricingConfig, error) {
	creditsPerUSD := s.currentCreditsPerUSD(ctx)
	config := hostedModelPricingConfigFromRequest(req, creditsPerUSD)
	values := map[string]any{
		"key":                            config.Key,
		"kind":                           config.Kind,
		"provider":                       config.Provider,
		"model":                          config.Model,
		"prompt_per_1m_cost_microusd":    config.PromptPer1MCostMicrousd,
		"output_per_1m_cost_microusd":    config.OutputPer1MCostMicrousd,
		"reasoning_per_1m_cost_microusd": config.ReasoningPer1MCostMicrousd,
		"enabled":                        config.Enabled,
	}
	updated, err := s.store.UpdateHostedModelPricingConfig(ctx, id, values)
	if err != nil {
		return nil, err
	}
	*updated = hostedModelPricingConfigWithCredits(*updated, creditsPerUSD)
	_ = s.store.CreateAuditLog(ctx, "hosted_pricing.model_config.update", "hosted_model_pricing_config", fmt.Sprintf("%d", id), sqlstore.JSONString(updated))
	return updated, nil
}

func validateHostedPricingRuleRequest(req UpsertHostedPricingRuleRequest) error {
	profile := strings.ToLower(strings.TrimSpace(req.DocumentProfile))
	switch profile {
	case "text", "image":
		return nil
	case "":
		return fmt.Errorf("document_profile is required")
	default:
		return fmt.Errorf("unsupported document_profile %q; must be text or image", req.DocumentProfile)
	}
}

func (s *Service) CreateHostedPricingRule(ctx context.Context, req UpsertHostedPricingRuleRequest) (*model.HostedPricingRule, error) {
	if err := validateHostedPricingRuleRequest(req); err != nil {
		return nil, err
	}
	rule := hostedPricingRuleFromRequest(req)
	if err := s.store.CreateHostedPricingRule(ctx, &rule); err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "hosted_pricing.rule.create", "hosted_pricing_rule", fmt.Sprintf("%d", rule.ID), sqlstore.JSONString(rule))
	return &rule, nil
}

func (s *Service) UpdateHostedPricingRule(ctx context.Context, id uint64, req UpsertHostedPricingRuleRequest) (*model.HostedPricingRule, error) {
	if err := validateHostedPricingRuleRequest(req); err != nil {
		return nil, err
	}
	rule := hostedPricingRuleFromRequest(req)
	values := map[string]any{
		"document_profile":               rule.DocumentProfile,
		"provider":                       rule.Provider,
		"model":                          rule.Model,
		"text_model_key":                 rule.TextModelKey,
		"image_model_key":                rule.ImageModelKey,
		"prompt_per_1k_cost_microusd":    rule.PromptPer1KCostMicrousd,
		"output_per_1k_cost_microusd":    rule.OutputPer1KCostMicrousd,
		"reasoning_per_1k_cost_microusd": rule.ReasoningPer1KCostMicrousd,
		"image_per_asset_cost_microusd":  rule.ImagePerAssetCostMicrousd,
		"minimum_charge_credits":         rule.MinimumChargeCredits,
		"markup_bps":                     rule.MarkupBPS,
		"enabled":                        rule.Enabled,
	}
	updated, err := s.store.UpdateHostedPricingRule(ctx, id, values)
	if err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "hosted_pricing.rule.update", "hosted_pricing_rule", fmt.Sprintf("%d", id), sqlstore.JSONString(updated))
	return updated, nil
}

func (s *Service) CreateHostedCreditPack(ctx context.Context, req UpsertHostedCreditPackRequest) (*model.HostedCreditPack, error) {
	pack := hostedCreditPackFromRequest(req, s.currentCreditsPerUSD(ctx))
	if err := s.store.CreateHostedCreditPack(ctx, &pack); err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "hosted_pricing.pack.create", "hosted_credit_pack", fmt.Sprintf("%d", pack.ID), sqlstore.JSONString(pack))
	return &pack, nil
}

func (s *Service) UpdateHostedCreditPack(ctx context.Context, id uint64, req UpsertHostedCreditPackRequest) (*model.HostedCreditPack, error) {
	pack := hostedCreditPackFromRequest(req, s.currentCreditsPerUSD(ctx))
	values := map[string]any{
		"code":          pack.Code,
		"name":          pack.Name,
		"description":   pack.Description,
		"currency":      pack.Currency,
		"amount_total":  pack.AmountTotal,
		"credit_amount": pack.CreditAmount,
		"enabled":       pack.Enabled,
	}
	updated, err := s.store.UpdateHostedCreditPack(ctx, id, values)
	if err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "hosted_pricing.pack.update", "hosted_credit_pack", fmt.Sprintf("%d", id), sqlstore.JSONString(updated))
	return updated, nil
}

func hostedModelPricingConfigFromRequest(req UpsertHostedModelPricingConfigRequest, creditsPerUSD int) model.HostedModelPricingConfig {
	promptCost := req.PromptPer1MCostMicrousd
	if req.PromptPer1MCostCredits != nil {
		promptCost = microusdFromCredits(*req.PromptPer1MCostCredits, creditsPerUSD)
	}
	outputCost := req.OutputPer1MCostMicrousd
	if req.OutputPer1MCostCredits != nil {
		outputCost = microusdFromCredits(*req.OutputPer1MCostCredits, creditsPerUSD)
	}
	reasoningCost := req.ReasoningPer1MCostMicrousd
	if req.ReasoningPer1MCostCredits != nil {
		reasoningCost = microusdFromCredits(*req.ReasoningPer1MCostCredits, creditsPerUSD)
	}
	return model.HostedModelPricingConfig{
		Key:                        strings.TrimSpace(req.Key),
		Kind:                       model.HostedModelPricingKind(strings.TrimSpace(req.Kind)),
		Provider:                   strings.TrimSpace(req.Provider),
		Model:                      strings.TrimSpace(req.Model),
		PromptPer1MCostMicrousd:    promptCost,
		OutputPer1MCostMicrousd:    outputCost,
		ReasoningPer1MCostMicrousd: reasoningCost,
		Enabled:                    req.Enabled,
	}
}

func hostedPricingRuleFromRequest(req UpsertHostedPricingRuleRequest) model.HostedPricingRule {
	return model.HostedPricingRule{
		DocumentProfile:            strings.TrimSpace(req.DocumentProfile),
		Provider:                   strings.TrimSpace(req.Provider),
		Model:                      strings.TrimSpace(req.Model),
		TextModelKey:               strings.TrimSpace(req.TextModelKey),
		ImageModelKey:              strings.TrimSpace(req.ImageModelKey),
		PromptPer1KCostMicrousd:    req.PromptPer1KCostMicrousd,
		OutputPer1KCostMicrousd:    req.OutputPer1KCostMicrousd,
		ReasoningPer1KCostMicrousd: req.ReasoningPer1KCostMicrousd,
		ImagePerAssetCostMicrousd:  req.ImagePerAssetCostMicrousd,
		PromptPer1KCredits:         req.PromptPer1KCredits,
		OutputPer1KCredits:         req.OutputPer1KCredits,
		ReasoningPer1KCredits:      req.ReasoningPer1KCredits,
		ImagePerAssetCredits:       req.ImagePerAssetCredits,
		MinimumChargeCredits:       req.MinimumChargeCredits,
		MarkupBPS:                  req.MarkupBPS,
		Enabled:                    req.Enabled,
	}
}

func hostedCreditPackFromRequest(req UpsertHostedCreditPackRequest, creditsPerUSD int) model.HostedCreditPack {
	currency := strings.ToLower(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "usd"
	}
	amountTotal := req.AmountTotal
	if req.CreditAmount > 0 {
		amountTotal = centsFromCredits(req.CreditAmount, creditsPerUSD)
	}
	return model.HostedCreditPack{
		Code:         strings.TrimSpace(req.Code),
		Name:         strings.TrimSpace(req.Name),
		Description:  strings.TrimSpace(req.Description),
		Currency:     currency,
		AmountTotal:  amountTotal,
		CreditAmount: req.CreditAmount,
		Enabled:      req.Enabled,
	}
}

func (s *Service) currentCreditsPerUSD(ctx context.Context) int {
	settings, err := s.store.HostedPricingSettings(ctx)
	if err != nil || settings == nil {
		return 100
	}
	return normalizeCreditsPerUSD(settings.CreditsPerUSD)
}

func hostedModelPricingConfigsWithCredits(configs []model.HostedModelPricingConfig, creditsPerUSD int) []model.HostedModelPricingConfig {
	out := make([]model.HostedModelPricingConfig, len(configs))
	for i, config := range configs {
		out[i] = hostedModelPricingConfigWithCredits(config, creditsPerUSD)
	}
	return out
}

func hostedModelPricingConfigWithCredits(config model.HostedModelPricingConfig, creditsPerUSD int) model.HostedModelPricingConfig {
	config.PromptPer1MCostCredits = creditsFromMicrousd(config.PromptPer1MCostMicrousd, creditsPerUSD)
	config.OutputPer1MCostCredits = creditsFromMicrousd(config.OutputPer1MCostMicrousd, creditsPerUSD)
	config.ReasoningPer1MCostCredits = creditsFromMicrousd(config.ReasoningPer1MCostMicrousd, creditsPerUSD)
	return config
}

func normalizeCreditsPerUSD(value int) int {
	if value <= 0 {
		return 100
	}
	return value
}

func microusdFromCredits(credits int64, creditsPerUSD int) int64 {
	if credits <= 0 {
		return 0
	}
	creditsPerUSD = normalizeCreditsPerUSD(creditsPerUSD)
	numerator := credits * 1_000_000
	result := numerator / int64(creditsPerUSD)
	if numerator%int64(creditsPerUSD) != 0 {
		result++
	}
	return result
}

func creditsFromMicrousd(microusd int64, creditsPerUSD int) int64 {
	if microusd <= 0 {
		return 0
	}
	creditsPerUSD = normalizeCreditsPerUSD(creditsPerUSD)
	numerator := microusd * int64(creditsPerUSD)
	result := numerator / 1_000_000
	if numerator%1_000_000 != 0 {
		result++
	}
	return result
}

func centsFromCredits(credits int, creditsPerUSD int) int64 {
	if credits <= 0 {
		return 0
	}
	creditsPerUSD = normalizeCreditsPerUSD(creditsPerUSD)
	numerator := int64(credits) * 100
	result := numerator / int64(creditsPerUSD)
	if numerator%int64(creditsPerUSD) != 0 {
		result++
	}
	return result
}

func generateAPIKey(salt string) (plain string, prefix string, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	plain = "cop_live_" + token
	prefixLen := 12
	if len(plain) < prefixLen {
		prefixLen = len(plain)
	}
	prefix = plain[:prefixLen]
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + plain))
	hash = hex.EncodeToString(sum[:])
	return plain, prefix, hash, nil
}
