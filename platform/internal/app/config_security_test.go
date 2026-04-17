package app

import (
	"testing"
	"time"
)

func setRequiredProductionEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ADMIN_PASSWORD", "real-admin-password")
	t.Setenv("SESSION_SECRET", "prod-session-secret-123456")
	t.Setenv("APP_SESSION_SECRET", "prod-app-session-secret-123456")
	t.Setenv("API_KEY_HASH_SALT", "prod-salt")
	t.Setenv("LICENSE_PROOF_SEED", "cHJvZC1saWNlbnNlLXByb29mLXNlZWQtMTIzNDU2Nzg")
	t.Setenv("STRIPE_SECRET_KEY", "sk_live_123")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_123")
}

func TestLoadConfigDefaultsToDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("ADMIN_PASSWORD", "")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("APP_SESSION_SECRET", "")
	t.Setenv("API_KEY_HASH_SALT", "")
	t.Setenv("LICENSE_PROOF_SEED", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q", cfg.AppEnv)
	}
}

func TestLoadConfigProductionRejectsDefaultAdminPassword(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	setRequiredProductionEnv(t)
	t.Setenv("ADMIN_PASSWORD", "")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigProductionRejectsPlaceholderSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	setRequiredProductionEnv(t)
	t.Setenv("SESSION_SECRET", "change-me-change-me-change-me-123456")
	t.Setenv("APP_SESSION_SECRET", "change-me-app-session-secret-123456")
	t.Setenv("API_KEY_HASH_SALT", "change-me-salt")
	t.Setenv("LICENSE_PROOF_SEED", "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigProductionAcceptsExplicitValues(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	setRequiredProductionEnv(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.AppEnv != "production" {
		t.Fatalf("AppEnv = %q", cfg.AppEnv)
	}
}

func TestLoadConfigDevelopmentUsesRelaxedRateLimitDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("ADMIN_LOGIN_RATE_LIMIT_PER_MINUTE", "")
	t.Setenv("LICENSE_RATE_LIMIT_PER_MINUTE", "")
	t.Setenv("RATE_LIMIT_VISITOR_TTL", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.AdminLoginRateLimitPerMinute != 60 {
		t.Fatalf("AdminLoginRateLimitPerMinute = %d", cfg.AdminLoginRateLimitPerMinute)
	}
	if cfg.LicenseRateLimitPerMinute != 300 {
		t.Fatalf("LicenseRateLimitPerMinute = %d", cfg.LicenseRateLimitPerMinute)
	}
	if cfg.RateLimitVisitorTTL != time.Minute {
		t.Fatalf("RateLimitVisitorTTL = %v", cfg.RateLimitVisitorTTL)
	}
}

func TestLoadConfigProductionUsesStrictRateLimitDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	setRequiredProductionEnv(t)
	t.Setenv("ADMIN_LOGIN_RATE_LIMIT_PER_MINUTE", "")
	t.Setenv("LICENSE_RATE_LIMIT_PER_MINUTE", "")
	t.Setenv("RATE_LIMIT_VISITOR_TTL", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.AdminLoginRateLimitPerMinute != 5 {
		t.Fatalf("AdminLoginRateLimitPerMinute = %d", cfg.AdminLoginRateLimitPerMinute)
	}
	if cfg.LicenseRateLimitPerMinute != 30 {
		t.Fatalf("LicenseRateLimitPerMinute = %d", cfg.LicenseRateLimitPerMinute)
	}
	if cfg.RateLimitVisitorTTL != 5*time.Minute {
		t.Fatalf("RateLimitVisitorTTL = %v", cfg.RateLimitVisitorTTL)
	}
}

func TestLoadConfigAllowsRateLimitOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	setRequiredProductionEnv(t)
	t.Setenv("ADMIN_LOGIN_RATE_LIMIT_PER_MINUTE", "9")
	t.Setenv("LICENSE_RATE_LIMIT_PER_MINUTE", "44")
	t.Setenv("RATE_LIMIT_VISITOR_TTL", "2m")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.AdminLoginRateLimitPerMinute != 9 {
		t.Fatalf("AdminLoginRateLimitPerMinute = %d", cfg.AdminLoginRateLimitPerMinute)
	}
	if cfg.LicenseRateLimitPerMinute != 44 {
		t.Fatalf("LicenseRateLimitPerMinute = %d", cfg.LicenseRateLimitPerMinute)
	}
	if cfg.RateLimitVisitorTTL != 2*time.Minute {
		t.Fatalf("RateLimitVisitorTTL = %v", cfg.RateLimitVisitorTTL)
	}
}

func TestLoadConfigProductionRejectsTestStripeSecretKey(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	setRequiredProductionEnv(t)
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_123")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "STRIPE_SECRET_KEY must use a Stripe live key in production" {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestLoadConfigProductionRejectsMissingStripeWebhookSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	setRequiredProductionEnv(t)
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "STRIPE_WEBHOOK_SECRET must be explicitly configured in production" {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestLoadConfigParsesAppGoogleAllowlist(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_GOOGLE_ALLOWLIST", " Demo@example.com, ops@example.com , ")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.AppGoogleAllowlist) != 2 {
		t.Fatalf("AppGoogleAllowlist len = %d", len(cfg.AppGoogleAllowlist))
	}
	if cfg.AppGoogleAllowlist[0] != "Demo@example.com" {
		t.Fatalf("AppGoogleAllowlist[0] = %q", cfg.AppGoogleAllowlist[0])
	}
	if cfg.AppGoogleAllowlist[1] != "ops@example.com" {
		t.Fatalf("AppGoogleAllowlist[1] = %q", cfg.AppGoogleAllowlist[1])
	}
}

func TestLoadConfigBuildsExternalPricingFromUnitPriceAndRatio(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("EXTERNAL_UNIT_PRICE_CENTS", "5")
	t.Setenv("EXTERNAL_500_PRICE_RATIO", "449/495")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.PricingPacks) < 2 {
		t.Fatalf("PricingPacks len = %d", len(cfg.PricingPacks))
	}
	if cfg.PricingPacks[0].Code != "external-100" || cfg.PricingPacks[0].AmountTotal != 500 {
		t.Fatalf("starter pack = %+v", cfg.PricingPacks[0])
	}
	if cfg.PricingPacks[1].Code != "external-500" || cfg.PricingPacks[1].AmountTotal != 2268 {
		t.Fatalf("bulk pack = %+v", cfg.PricingPacks[1])
	}
}

func TestLoadConfigRejectsInvalidExternal500PriceRatio(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("EXTERNAL_500_PRICE_RATIO", "not-a-ratio")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error")
	}
}
