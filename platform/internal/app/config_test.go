package app

import (
	"net/url"
	"strings"
	"testing"
)

func TestLoadConfigDefaultsFreeTrialLimitToFive(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DEFAULT_FREE_LIMIT", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.DefaultFreeLimit != 5 {
		t.Fatalf("DefaultFreeLimit = %d, want 5", cfg.DefaultFreeLimit)
	}
}

func TestDefaultHostedPricingRulesUseTextAndImageProfiles(t *testing.T) {
	t.Setenv("HOSTED_PRICING_RULES_JSON", "")

	profiles := map[string]bool{}
	for _, rule := range defaultHostedPricingRules() {
		profiles[rule.DocumentProfile] = true
		switch rule.DocumentProfile {
		case "text":
			if rule.TextModelKey != "text_default" || rule.ImageModelKey != "" || rule.ReservationCredits != 16 || rule.MinimumChargeCredits != 2 {
				t.Fatalf("unexpected text pricing rule: %#v", rule)
			}
		case "image":
			if rule.ImageModelKey != "image_default" || rule.ImagePerAssetCredits != 1 || rule.ReservationCredits != 1 || rule.MinimumChargeCredits != 1 {
				t.Fatalf("unexpected image pricing rule: %#v", rule)
			}
		default:
			t.Fatalf("unexpected hosted pricing profile: %q", rule.DocumentProfile)
		}
	}
	if !profiles["text"] || !profiles["image"] || len(profiles) != 2 {
		t.Fatalf("default hosted pricing profiles = %#v", profiles)
	}
}

func TestLoadConfigAdminOAuth2ClientFallsBackToAppClient(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("OAUTH2_CLIENT_ID", "app-client")
	t.Setenv("OAUTH2_CLIENT_SECRET", "app-secret")
	t.Setenv("ADMIN_OAUTH2_CLIENT_ID", "")
	t.Setenv("ADMIN_OAUTH2_CLIENT_SECRET", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.AdminOAuth2ClientID != "app-client" {
		t.Fatalf("AdminOAuth2ClientID = %q, want app-client", cfg.AdminOAuth2ClientID)
	}
	if cfg.AdminOAuth2ClientSecret != "app-secret" {
		t.Fatalf("AdminOAuth2ClientSecret = %q, want app-secret", cfg.AdminOAuth2ClientSecret)
	}
}

func TestLoadConfigRejectsPartialAdminOAuth2ClientOverride(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("OAUTH2_CLIENT_ID", "app-client")
	t.Setenv("OAUTH2_CLIENT_SECRET", "app-secret")
	t.Setenv("ADMIN_OAUTH2_CLIENT_ID", "admin-client")
	t.Setenv("ADMIN_OAUTH2_CLIENT_SECRET", "")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want partial admin OAuth2 override error")
	}
	if !strings.Contains(err.Error(), "ADMIN_OAUTH2_CLIENT_ID and ADMIN_OAUTH2_CLIENT_SECRET must be configured together") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewAdminOAuthProviderUsesAdminOAuth2ClientOverride(t *testing.T) {
	provider := newAdminOAuthProvider(Config{
		OAuth2ClientID:          "app-client",
		OAuth2ClientSecret:      "app-secret",
		OAuth2AuthURL:           "https://idp.example.com/authorize",
		OAuth2TokenURL:          "https://idp.example.com/token",
		OAuth2UserinfoURL:       "https://idp.example.com/userinfo",
		OAuth2RedirectURL:       "https://officecli.shimodev.com/api/auth/oauth2/callback",
		AdminOAuth2ClientID:     "admin-client",
		AdminOAuth2ClientSecret: "admin-secret",
		AdminOAuth2RedirectURL:  "https://officecli.shimodev.com/api/admin/auth/oauth2/callback",
		GoogleRedirectURL:       "https://officecli.shimodev.com/api/auth/google/callback",
		AdminGoogleRedirectURL:  "https://officecli.shimodev.com/api/admin/auth/google/callback",
	})

	loginURL := provider.AuthCodeURL("state-123")
	parsed, err := url.Parse(loginURL)
	if err != nil {
		t.Fatalf("Parse loginURL: %v", err)
	}

	if parsed.Query().Get("client_id") != "admin-client" {
		t.Fatalf("client_id = %q, want admin-client", parsed.Query().Get("client_id"))
	}
	if parsed.Query().Get("redirect_uri") != "https://officecli.shimodev.com/api/admin/auth/oauth2/callback" {
		t.Fatalf("redirect_uri = %q", parsed.Query().Get("redirect_uri"))
	}
}
