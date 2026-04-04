package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/officecli/officecli/platform/internal/model"
)

type Config struct {
	AppEnv                       string
	HTTPAddr                     string
	MYSQLDSN                     string
	RedisAddr                    string
	AdminLoginRateLimitPerMinute int
	LicenseRateLimitPerMinute    int
	RateLimitVisitorTTL          time.Duration
	DefaultFreeLimit             int
	AdminPassword                string
	SessionSecret                string
	APIKeyHashSalt               string
	UsageIdempotencyTTL          time.Duration
	AdminSessionTTL              time.Duration
	AdminStaticDir               string
	AppStaticDir                 string
	SiteStaticDir                string
	SiteBaseURL                  string
	PlatformBaseURL              string
	AppSessionSecret             string
	AppSessionTTL                time.Duration
	LicenseProofSeed             string
	LicenseProofTTL              time.Duration
	GoogleClientID               string
	GoogleClientSecret           string
	GoogleRedirectURL            string
	DiscordClientID              string
	DiscordClientSecret          string
	DiscordRedirectURL           string
	DiscordGuildID               string
	DiscordBotToken              string
	HostedLLMBaseURL             string
	HostedLLMAPIKey              string
	HostedLLMTextModel           string
	HostedLLMImageModel          string
	HostedLLMProvider            string
	ClaudeOfficeBaseURL          string
	ClaudeOfficeAuthKey          string
	PublishRateLimitPerMinute    int
	PublishMaxFileBytes          int64
	PublishDefaultExpireSeconds  int
	HostedPricingRules           []model.HostedPricingRule
	AdminGoogleRedirectURL       string
	AdminGoogleAllowlist         []string
	StripeSecretKey              string
	StripeWebhookSecret          string
	StripeSuccessURL             string
	StripeCancelURL              string
	PricingPacks                 []model.PricingPack
}

func LoadConfig() (Config, error) {
	cfg := Config{
		AppEnv:                       normalizeAppEnv(mustEnvDefault("APP_ENV", "development")),
		HTTPAddr:                     mustEnvDefault("HTTP_ADDR", ":8080"),
		MYSQLDSN:                     mustEnvDefault("MYSQL_DSN", "root:root@tcp(127.0.0.1:3306)/cli_office_platform?parseTime=true&multiStatements=true&charset=utf8mb4"),
		RedisAddr:                    mustEnvDefault("REDIS_ADDR", "127.0.0.1:6379"),
		AdminLoginRateLimitPerMinute: defaultAdminLoginRateLimit(normalizeAppEnv(mustEnvDefault("APP_ENV", "development"))),
		LicenseRateLimitPerMinute:    defaultLicenseRateLimit(normalizeAppEnv(mustEnvDefault("APP_ENV", "development"))),
		RateLimitVisitorTTL:          defaultRateLimitVisitorTTL(normalizeAppEnv(mustEnvDefault("APP_ENV", "development"))),
		DefaultFreeLimit:             mustEnvInt("DEFAULT_FREE_LIMIT", 10),
		AdminPassword:                mustEnvDefault("ADMIN_PASSWORD", "admin123"),
		SessionSecret:                mustEnvDefault("SESSION_SECRET", "change-me-change-me-change-me-123456"),
		APIKeyHashSalt:               mustEnvDefault("API_KEY_HASH_SALT", "change-me-salt"),
		UsageIdempotencyTTL:          mustEnvDuration("USAGE_IDEMPOTENCY_TTL", 24*time.Hour),
		AdminSessionTTL:              mustEnvDuration("ADMIN_SESSION_TTL", 24*time.Hour),
		AdminStaticDir:               mustEnvDefault("ADMIN_STATIC_DIR", "web/admin/dist"),
		AppStaticDir:                 mustEnvDefault("APP_STATIC_DIR", "web/app/dist"),
		SiteStaticDir:                mustEnvDefault("SITE_STATIC_DIR", "web/site/dist"),
		SiteBaseURL:                  mustEnvDefault("SITE_BASE_URL", "https://officecli.io"),
		PlatformBaseURL:              mustEnvDefault("PLATFORM_BASE_URL", "https://platform.officecli.io"),
		AppSessionSecret:             mustEnvDefault("APP_SESSION_SECRET", "change-me-app-session-secret-123456"),
		AppSessionTTL:                mustEnvDuration("APP_SESSION_TTL", 24*time.Hour),
		LicenseProofSeed:             mustEnvDefault("LICENSE_PROOF_SEED", "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"),
		LicenseProofTTL:              mustEnvDuration("LICENSE_PROOF_TTL", 2*time.Minute),
		GoogleClientID:               os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:           os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:            mustEnvDefault("GOOGLE_REDIRECT_URL", "https://platform.officecli.io/api/auth/google/callback"),
		DiscordClientID:              os.Getenv("DISCORD_CLIENT_ID"),
		DiscordClientSecret:          os.Getenv("DISCORD_CLIENT_SECRET"),
		DiscordRedirectURL:           mustEnvDefault("DISCORD_REDIRECT_URL", "https://platform.officecli.io/api/app/discord/callback"),
		DiscordGuildID:               os.Getenv("DISCORD_GUILD_ID"),
		DiscordBotToken:              os.Getenv("DISCORD_BOT_TOKEN"),
		HostedLLMBaseURL:             mustEnvDefault("HOSTED_LLM_BASE_URL", "https://api.openai.com/v1"),
		HostedLLMAPIKey:              os.Getenv("HOSTED_LLM_API_KEY"),
		HostedLLMTextModel:           mustEnvDefault("HOSTED_LLM_TEXT_MODEL", "gpt-4.1"),
		HostedLLMImageModel:          mustEnvDefault("HOSTED_LLM_IMAGE_MODEL", "gpt-image-1"),
		HostedLLMProvider:            mustEnvDefault("HOSTED_LLM_PROVIDER", "openai"),
		ClaudeOfficeBaseURL:          mustEnvDefault("CLAUDEOFFICE_BASE_URL", ""),
		ClaudeOfficeAuthKey:          os.Getenv("CLAUDEOFFICE_AUTH_KEY"),
		PublishRateLimitPerMinute:    mustEnvInt("PUBLISH_RATE_LIMIT_PER_MINUTE", 30),
		PublishMaxFileBytes:          mustEnvInt64("PUBLISH_MAX_FILE_BYTES", 50<<20),
		PublishDefaultExpireSeconds:  mustEnvInt("PUBLISH_DEFAULT_EXPIRE_SECONDS", 24*60*60),
		HostedPricingRules:           defaultHostedPricingRules(),
		AdminGoogleRedirectURL:       mustEnvDefault("ADMIN_GOOGLE_REDIRECT_URL", "https://platform.officecli.io/api/admin/auth/google/callback"),
		AdminGoogleAllowlist:         parseCSVList(os.Getenv("ADMIN_GOOGLE_ALLOWLIST")),
		StripeSecretKey:              os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:          os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripeSuccessURL:             mustEnvDefault("STRIPE_SUCCESS_URL", "https://platform.officecli.io/app/billing?status=success"),
		StripeCancelURL:              mustEnvDefault("STRIPE_CANCEL_URL", "https://platform.officecli.io/app/billing?status=cancel"),
		PricingPacks:                 defaultPricingPacks(),
	}
	cfg.AdminLoginRateLimitPerMinute = mustEnvInt("ADMIN_LOGIN_RATE_LIMIT_PER_MINUTE", cfg.AdminLoginRateLimitPerMinute)
	cfg.LicenseRateLimitPerMinute = mustEnvInt("LICENSE_RATE_LIMIT_PER_MINUTE", cfg.LicenseRateLimitPerMinute)
	cfg.RateLimitVisitorTTL = mustEnvDuration("RATE_LIMIT_VISITOR_TTL", cfg.RateLimitVisitorTTL)
	if cfg.AppEnv != "development" && cfg.AppEnv != "staging" && cfg.AppEnv != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be one of development, staging, production")
	}
	if len(cfg.SessionSecret) < 16 {
		return Config{}, fmt.Errorf("SESSION_SECRET must be at least 16 chars")
	}
	if len(cfg.AppSessionSecret) < 16 {
		return Config{}, fmt.Errorf("APP_SESSION_SECRET must be at least 16 chars")
	}
	if cfg.AppEnv == "production" {
		if err := validateProductionSecrets(cfg); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func parseCSVList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := strings.TrimSpace(part)
		if normalized == "" {
			continue
		}
		items = append(items, normalized)
	}
	return items
}

func defaultAdminLoginRateLimit(appEnv string) int {
	if appEnv == "production" {
		return 5
	}
	return 60
}

func defaultLicenseRateLimit(appEnv string) int {
	if appEnv == "production" {
		return 30
	}
	return 300
}

func defaultRateLimitVisitorTTL(appEnv string) time.Duration {
	if appEnv == "production" {
		return 5 * time.Minute
	}
	return time.Minute
}

func defaultPricingPacks() []model.PricingPack {
	raw := strings.TrimSpace(os.Getenv("PRICING_PACKS_JSON"))
	if raw == "" {
		return fallbackPricingPacks()
	}
	var packs []model.PricingPack
	if err := jsonUnmarshal([]byte(raw), &packs); err != nil || len(packs) == 0 {
		return fallbackPricingPacks()
	}
	return packs
}

func fallbackPricingPacks() []model.PricingPack {
	return []model.PricingPack{
		{Code: "external-100", Name: "External 100", Description: "100 external generations for workflows that already bring their own LLM.", Currency: "usd", AmountTotal: 1900, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)},
		{Code: "external-500", Name: "External 500", Description: "500 external generations for teams running document work in bulk.", Currency: "usd", AmountTotal: 7900, QuotaAmount: 500, PackKind: string(model.PackKindExternalGeneration)},
		{Code: "hosted-300", Name: "Hosted 300", Description: "300 hosted credits for low-volume runs on the platform-managed LLM runtime.", Currency: "usd", AmountTotal: 2900, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)},
		{Code: "hosted-1200", Name: "Hosted 1200", Description: "1200 hosted credits for teams that want the platform-managed LLM runtime.", Currency: "usd", AmountTotal: 9900, CreditAmount: 1200, PackKind: string(model.PackKindHostedCredits)},
	}
}

func defaultHostedPricingRules() []model.HostedPricingRule {
	raw := strings.TrimSpace(os.Getenv("HOSTED_PRICING_RULES_JSON"))
	if raw != "" {
		var rules []model.HostedPricingRule
		if err := jsonUnmarshal([]byte(raw), &rules); err == nil && len(rules) > 0 {
			return rules
		}
	}
	return []model.HostedPricingRule{
		{
			DocumentProfile:       "docx-xlsx",
			Provider:              "openai",
			Model:                 "gpt-4.1",
			PromptPer1KCredits:    1,
			OutputPer1KCredits:    2,
			ReasoningPer1KCredits: 2,
			ImagePerAssetCredits:  0,
			ReservationCredits:    16,
			MinimumChargeCredits:  2,
		},
		{
			DocumentProfile:       "pptx-no-image",
			Provider:              "openai",
			Model:                 "gpt-4.1",
			PromptPer1KCredits:    2,
			OutputPer1KCredits:    3,
			ReasoningPer1KCredits: 3,
			ImagePerAssetCredits:  0,
			ReservationCredits:    28,
			MinimumChargeCredits:  6,
		},
		{
			DocumentProfile:       "pptx-with-image",
			Provider:              "openai",
			Model:                 "gpt-4.1",
			PromptPer1KCredits:    2,
			OutputPer1KCredits:    4,
			ReasoningPer1KCredits: 4,
			ImagePerAssetCredits:  24,
			ReservationCredits:    48,
			MinimumChargeCredits:  12,
		},
	}
}

func mustEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func normalizeAppEnv(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "development"
	}
	return value
}

func validateProductionSecrets(cfg Config) error {
	if strings.TrimSpace(cfg.AdminPassword) == "" || cfg.AdminPassword == "admin123" {
		return fmt.Errorf("ADMIN_PASSWORD must be explicitly configured in production")
	}
	if strings.TrimSpace(cfg.SessionSecret) == "" || cfg.SessionSecret == "change-me-change-me-change-me-123456" {
		return fmt.Errorf("SESSION_SECRET must be explicitly configured in production")
	}
	if strings.TrimSpace(cfg.AppSessionSecret) == "" || cfg.AppSessionSecret == "change-me-app-session-secret-123456" {
		return fmt.Errorf("APP_SESSION_SECRET must be explicitly configured in production")
	}
	if strings.TrimSpace(cfg.APIKeyHashSalt) == "" || cfg.APIKeyHashSalt == "change-me-salt" {
		return fmt.Errorf("API_KEY_HASH_SALT must be explicitly configured in production")
	}
	if strings.TrimSpace(cfg.LicenseProofSeed) == "" || cfg.LicenseProofSeed == "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA" {
		return fmt.Errorf("LICENSE_PROOF_SEED must be explicitly configured in production")
	}
	if err := validateLicenseProofSeed(cfg.LicenseProofSeed); err != nil {
		return err
	}
	return nil
}

func validateLicenseProofSeed(raw string) error {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("LICENSE_PROOF_SEED must be base64url encoded: %w", err)
	}
	if len(decoded) != 32 {
		return fmt.Errorf("LICENSE_PROOF_SEED must decode to 32 bytes")
	}
	return nil
}

func mustEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func mustEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func mustEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		parsed, err := time.ParseDuration(v)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func jsonUnmarshal(data []byte, dest any) error {
	return json.Unmarshal(data, dest)
}
