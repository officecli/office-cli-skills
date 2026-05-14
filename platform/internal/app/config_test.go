package app

import "testing"

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
