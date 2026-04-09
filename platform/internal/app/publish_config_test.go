package app

import (
	"testing"
)

func TestPublishConfigFromAppConfigIncludesDynamicAuth(t *testing.T) {
	cfg := Config{
		ClaudeOfficeBaseURL:          "https://claudeoffice.com",
		ClaudeOfficeAuthKey:          "legacy-auth-key",
		ClaudeOfficeAuthKeyID:        "platform-prod",
		ClaudeOfficeAuthSharedSecret: "shared-secret",
		PublishDefaultExpireSeconds:  3600,
	}

	got := publishConfigFromAppConfig(cfg, "hash-salt")

	if got.BaseURL != cfg.ClaudeOfficeBaseURL {
		t.Fatalf("BaseURL = %q", got.BaseURL)
	}
	if got.AuthKey != cfg.ClaudeOfficeAuthKey {
		t.Fatalf("AuthKey = %q", got.AuthKey)
	}
	if got.AuthKeyID != cfg.ClaudeOfficeAuthKeyID {
		t.Fatalf("AuthKeyID = %q", got.AuthKeyID)
	}
	if got.AuthSharedSecret != cfg.ClaudeOfficeAuthSharedSecret {
		t.Fatalf("AuthSharedSecret = %q", got.AuthSharedSecret)
	}
	if got.HashSalt != "hash-salt" {
		t.Fatalf("HashSalt = %q", got.HashSalt)
	}
	if got.DefaultExpireSeconds != cfg.PublishDefaultExpireSeconds {
		t.Fatalf("DefaultExpireSeconds = %d", got.DefaultExpireSeconds)
	}
}
