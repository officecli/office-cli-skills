package app

import (
	"testing"
	"time"
)

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
	t.Setenv("ADMIN_PASSWORD", "")
	t.Setenv("SESSION_SECRET", "prod-session-secret-123456")
	t.Setenv("APP_SESSION_SECRET", "prod-app-session-secret-123456")
	t.Setenv("API_KEY_HASH_SALT", "prod-salt")
	t.Setenv("LICENSE_PROOF_SEED", "prod-license-proof-seed-ignored")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigProductionRejectsPlaceholderSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("ADMIN_PASSWORD", "real-admin-password")
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
	t.Setenv("ADMIN_PASSWORD", "real-admin-password")
	t.Setenv("SESSION_SECRET", "prod-session-secret-123456")
	t.Setenv("APP_SESSION_SECRET", "prod-app-session-secret-123456")
	t.Setenv("API_KEY_HASH_SALT", "prod-salt")
	t.Setenv("LICENSE_PROOF_SEED", "cHJvZC1saWNlbnNlLXByb29mLXNlZWQtMTIzNDU2Nzg")

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
	t.Setenv("ADMIN_PASSWORD", "real-admin-password")
	t.Setenv("SESSION_SECRET", "prod-session-secret-123456")
	t.Setenv("APP_SESSION_SECRET", "prod-app-session-secret-123456")
	t.Setenv("API_KEY_HASH_SALT", "prod-salt")
	t.Setenv("LICENSE_PROOF_SEED", "cHJvZC1saWNlbnNlLXByb29mLXNlZWQtMTIzNDU2Nzg")
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
	t.Setenv("ADMIN_PASSWORD", "real-admin-password")
	t.Setenv("SESSION_SECRET", "prod-session-secret-123456")
	t.Setenv("APP_SESSION_SECRET", "prod-app-session-secret-123456")
	t.Setenv("API_KEY_HASH_SALT", "prod-salt")
	t.Setenv("LICENSE_PROOF_SEED", "cHJvZC1saWNlbnNlLXByb29mLXNlZWQtMTIzNDU2Nzg")
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

func TestLoadConfigParsesAppGoogleAllowlist(t *testing.T) {
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
