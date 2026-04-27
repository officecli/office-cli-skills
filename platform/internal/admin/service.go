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
}

type GrowthSnapshot struct {
	RewardGrants       []model.RewardGrant       `json:"reward_grants"`
	Referrals          []model.UserReferral      `json:"referrals"`
	DiscordConnections []model.DiscordConnection `json:"discord_connections"`
}

type DailyFreeQuotaView struct {
	ID              uint64    `json:"id"`
	FingerprintHash string    `json:"fingerprint_hash"`
	UsageDate       string    `json:"usage_date"`
	DailyLimit      int       `json:"daily_limit"`
	DailyUsed       int       `json:"daily_used"`
	Remaining       int       `json:"remaining"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type QuotaSources struct {
	FreeTrialDevices []DailyFreeQuotaView `json:"free_trial_devices"`
	RewardGrants     []model.RewardGrant  `json:"reward_grants"`
	PaidExternalKeys []model.APIKey       `json:"paid_external_keys"`
	HostedKeys       []model.APIKey       `json:"hosted_keys"`
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
	sessionID := uuid.NewString()
	payload := SessionPayload{SessionID: sessionID, CreatedAt: time.Now().UTC(), AuthMethod: "password"}
	if err := s.redis.SaveSession(ctx, sessionID, payload, s.sessionTTL); err != nil {
		return "", err
	}
	raw, err := s.sessionCookieCodec.Encode(sessionID)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(payload)
	_ = s.store.CreateAuditLog(ctx, "admin.login", "session", sessionID, string(body))
	return raw, nil
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
	return s.store.Overview(ctx)
}
func (s *Service) ListAPIKeys(ctx context.Context) ([]model.APIKey, error) {
	return s.store.ListAPIKeys(ctx)
}

func (s *Service) CreateAPIKey(ctx context.Context, req CreateAPIKeyRequest) (*CreateAPIKeyResponse, *model.APIKey, error) {
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
	if req.CreditReserved != nil {
		updates["credit_reserved"] = *req.CreditReserved
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

func (s *Service) ListFreeQuotas(ctx context.Context, fingerprint string, usageDate string) ([]DailyFreeQuotaView, error) {
	quotas, err := s.store.ListDailyFreeQuotas(ctx, fingerprint, usageDate)
	if err != nil {
		return nil, err
	}
	views := make([]DailyFreeQuotaView, 0, len(quotas))
	for _, quota := range quotas {
		views = append(views, newDailyFreeQuotaView(quota))
	}
	return views, nil
}

func (s *Service) UpdateFreeQuota(ctx context.Context, id uint64, freeLimit int) error {
	if err := s.store.UpdateDailyFreeQuota(ctx, id, freeLimit); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"free_limit": freeLimit})
	return s.store.CreateAuditLog(ctx, "free_quota.update", "daily_free_quota", fmt.Sprintf("%d", id), string(payload))
}

func (s *Service) ListUsageEvents(ctx context.Context, filter sqlstore.UsageEventFilter) ([]model.UsageEvent, error) {
	return s.store.ListUsageEvents(ctx, filter)
}

func (s *Service) ListUsers(ctx context.Context) ([]model.User, error) {
	return s.store.ListUsers(ctx)
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
	return s.store.ListBillingEvents(ctx)
}

func (s *Service) Growth(ctx context.Context) (*GrowthSnapshot, error) {
	rewardGrants, err := s.store.ListRewardGrants(ctx)
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
		Referrals:          referrals,
		DiscordConnections: discordConnections,
	}, nil
}

func (s *Service) QuotaSources(ctx context.Context, filter QuotaSourcesFilter) (*QuotaSources, error) {
	freeQuotas, err := s.store.ListDailyFreeQuotas(ctx, filter.Fingerprint, filter.UsageDate)
	if err != nil {
		return nil, err
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
		FreeTrialDevices: make([]DailyFreeQuotaView, 0, len(freeQuotas)),
		RewardGrants:     make([]model.RewardGrant, 0, len(rewardGrants)),
		PaidExternalKeys: make([]model.APIKey, 0, len(apiKeys)),
		HostedKeys:       make([]model.APIKey, 0, len(apiKeys)),
	}

	for _, quota := range freeQuotas {
		result.FreeTrialDevices = append(result.FreeTrialDevices, newDailyFreeQuotaView(quota))
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
		if key.SupportsHosted() || key.CreditBalance > 0 || key.CreditReserved > 0 {
			result.HostedKeys = append(result.HostedKeys, key)
		}
		if key.QuotaTotal != nil {
			result.PaidExternalKeys = append(result.PaidExternalKeys, key)
		}
	}

	return result, nil
}

func newDailyFreeQuotaView(quota model.DailyFreeQuota) DailyFreeQuotaView {
	return DailyFreeQuotaView{
		ID:              quota.ID,
		FingerprintHash: quota.FingerprintHash,
		UsageDate:       quota.UsageDate,
		DailyLimit:      quota.DailyLimit,
		DailyUsed:       quota.DailyUsed,
		Remaining:       quota.Remaining(),
		CreatedAt:       quota.CreatedAt,
		UpdatedAt:       quota.UpdatedAt,
	}
}

func (s *Service) HostedPricingRules(_ context.Context) ([]model.HostedPricingRule, error) {
	if s.hostedPricing == nil {
		return nil, nil
	}
	return s.hostedPricing.HostedPricingRules(), nil
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
