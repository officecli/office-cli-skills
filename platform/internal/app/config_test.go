package app

import "testing"

func TestDefaultHostedPricingRulesIncludeIMG(t *testing.T) {
	t.Setenv("HOSTED_PRICING_RULES_JSON", "")

	for _, rule := range defaultHostedPricingRules() {
		if rule.DocumentProfile != "img" {
			continue
		}
		if rule.ImagePerAssetCredits != 1 || rule.ReservationCredits != 1 || rule.MinimumChargeCredits != 1 {
			t.Fatalf("unexpected img pricing rule: %#v", rule)
		}
		return
	}
	t.Fatal("default hosted pricing rules did not include img")
}
