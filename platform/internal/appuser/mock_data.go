package appuser

import (
	"errors"
	"time"

	"github.com/officecli/officecli/platform/internal/apikey"
	"github.com/officecli/officecli/platform/internal/model"
)

var mockBaseTime = time.Date(2026, 5, 17, 9, 30, 0, 0, time.UTC)

func mockOverview() *Overview {
	return &Overview{
		APIKeyCount:            3,
		TotalRemaining:         620,
		HostedCreditBalance:    1380,
		SignupCreditBonus:      SignupHostedCreditBonus,
		RewardRemaining:        226,
		InviteCode:             "MOCK2026",
		InviteLimit:            5,
		InviteRemaining:        2,
		RewardPerInvite:        100,
		ReferralCount:          3,
		ActivatedReferralCount: 2,
		DiscordConnected:       true,
		DiscordGuildMember:     false,
		RecentUsageCount:       4,
		RecentOrdersCount:      3,
		Pricing:                nil,
	}
}

func mockAPIKeys() []APIKeyView {
	lastUsed := mockBaseTime.Add(-16 * time.Minute)
	expires := mockBaseTime.Add(30 * 24 * time.Hour)
	noteHosted := "Primary CI credential with retained plaintext for copy flow validation."
	noteLegacy := "Legacy external quota credential retained for backwards-compatible quota display."
	return []APIKeyView{
		{ID: 9001, KeyPrefix: "ocli_app_mock_hosted_7KQ2", PlaintextAvailable: true, Status: model.APIKeyStatusActive, PlanName: "Hosted Team", Note: &noteHosted, LastUsedAt: &lastUsed, CreditBalance: 1200, CreatedAt: mockBaseTime.Add(-72 * time.Hour)},
		{ID: 9002, KeyPrefix: "ocli_app_mock_external_Q9X1", PlaintextAvailable: false, Status: model.APIKeyStatusActive, PlanName: "External Legacy", Note: &noteLegacy, ExpiresAt: &expires, QuotaTotal: mockIntPtr(800), QuotaUsed: 180, QuotaRemaining: 620, CreatedAt: mockBaseTime.Add(-48 * time.Hour)},
		{ID: 9003, KeyPrefix: "ocli_app_mock_disabled_0Z9", PlaintextAvailable: true, Status: model.APIKeyStatusDisabled, PlanName: "Suspended Hosted", CreditBalance: 180, CreatedAt: mockBaseTime.Add(-18 * time.Hour)},
	}
}

func mockAPIKeyPlaintext(id uint64) (string, error) {
	switch id {
	case 9001:
		return "ocli_live_mock_hosted_secret_7KQ2", nil
	case 9003:
		return "ocli_live_mock_disabled_secret_0Z9", nil
	default:
		return "", apikey.ErrPlaintextUnavailable
	}
}

func mockUsageEvents() []UsageEventView {
	reasonReserved := "hosted_credits_reserved"
	reasonAllowed := "external_unlimited"
	reasonBlocked := "daily_free_limit_exhausted"
	cli := "officecli/0.2.34"
	docPPTX := "pptx"
	docIMG := "img"
	runtimeHosted := "hosted"
	runtimeExternal := "external"
	providerOpenAI := "openai"
	modelText := "gpt-4.1"
	modelImage := "gpt-image-2"
	return []UsageEventView{
		{ID: 9101, FingerprintHash: "fp_app_mock_hosted_ci_device", Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, ReasonCode: &reasonReserved, CLIVersion: &cli, DocumentType: &docPPTX, RuntimeMode: &runtimeHosted, Charged: true, Provider: &providerOpenAI, ModelName: &modelText, PromptTokens: 18420, CompletionTokens: 2470, ReasoningTokens: 512, SettledCredits: 11, CreatedAt: mockBaseTime.Add(-9 * time.Minute)},
		{ID: 9102, FingerprintHash: "fp_app_mock_external_builder", Mode: model.UsageModePaid, Action: model.UsageActionStatus, Result: model.UsageResultAllowed, ReasonCode: &reasonAllowed, CLIVersion: &cli, DocumentType: &docPPTX, RuntimeMode: &runtimeExternal, BilledUnits: 1, UnitType: "document", CreatedAt: mockBaseTime.Add(-25 * time.Minute)},
		{ID: 9103, FingerprintHash: "fp_app_mock_trial_exhausted", Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultBlocked, ReasonCode: &reasonBlocked, CLIVersion: &cli, DocumentType: &docIMG, RuntimeMode: &runtimeHosted, Provider: &providerOpenAI, ModelName: &modelImage, ImageCount: 2, CreatedAt: mockBaseTime.Add(-43 * time.Minute)},
	}
}

func mockQuotaSummary() *QuotaSummary {
	growth := mockGrowth()
	keys := mockAPIKeys()
	return &QuotaSummary{
		RewardQuota: RewardQuotaSummary{Remaining: 226, Grants: growth.RewardGrants},
		PaidExternalQuota: PaidExternalQuotaSummary{
			TotalRemaining: 620,
			Keys:           []APIKeyView{keys[1]},
		},
		TrialPolicy: TrialPolicy{
			CLIBinaryOnly: true,
			Message:       "External Mode uses your configured model endpoint and does not consume OfficeCLI quota.",
		},
	}
}

func mockGrowth() *GrowthSnapshot {
	activated := mockBaseTime.Add(-20 * time.Hour)
	rewarded := mockBaseTime.Add(-19 * time.Hour)
	discordRewarded := mockBaseTime.Add(-3 * time.Hour)
	return &GrowthSnapshot{
		InviteCode:      "MOCK2026",
		InviteLimit:     5,
		InviteRemaining: 2,
		RewardPerInvite: 100,
		RewardRemaining: 226,
		RewardGrants: []RewardGrantView{
			{SourceType: model.RewardSourceInviteActivation, AmountTotal: 100, AmountUsed: 24, Remaining: 76, Reason: "Invite activation reward", MetadataJSON: `{"invite_code":"MOCK2026"}`, CreatedAt: mockBaseTime.Add(-20 * time.Hour), UpdatedAt: mockBaseTime.Add(-2 * time.Hour)},
			{SourceType: model.RewardSourceDiscordJoin, AmountTotal: 150, AmountUsed: 0, Remaining: 150, Reason: "Discord community reward", MetadataJSON: `{}`, CreatedAt: mockBaseTime.Add(-3 * time.Hour), UpdatedAt: mockBaseTime.Add(-3 * time.Hour)},
		},
		Referrals: []ReferralView{
			{InviteCode: "MOCK2026", RegisteredAt: mockBaseTime.Add(-24 * time.Hour), ActivatedAt: &activated, RewardGrantedAt: &rewarded},
			{InviteCode: "MOCK2026", RegisteredAt: mockBaseTime.Add(-8 * time.Hour)},
			{InviteCode: "MOCK2026", RegisteredAt: mockBaseTime.Add(-4 * time.Hour)},
		},
		DiscordConnection: &DiscordConnectionView{
			Username:                  "mock-officecli-user",
			GuildMember:               false,
			ConnectedAt:               mockBaseTime.Add(-3 * time.Hour),
			RewardGrantedAt:           &discordRewarded,
			VerificationStatus:        "verification_blocked",
			VerificationBlockedReason: DiscordGuildVerificationBlockedReason,
		},
	}
}

func MockOrders() []model.Order {
	intent := "pi_mock_app_paid"
	session := "cs_mock_app_pending"
	return []model.Order{
		{ID: 9201, UserID: 8801, Status: model.OrderStatusPaid, Currency: "usd", AmountTotal: 300, PackCode: "hosted-300", PackName: "Hosted 300", PackKind: model.PackKindHostedCredits, CreditAmount: 300, StripePaymentIntentID: &intent, CreatedAt: mockBaseTime.Add(-3 * time.Hour), UpdatedAt: mockBaseTime.Add(-2 * time.Hour)},
		{ID: 9202, UserID: 8801, Status: model.OrderStatusPending, Currency: "usd", AmountTotal: 1200, PackCode: "hosted-1200", PackName: "Hosted 1200", PackKind: model.PackKindHostedCredits, CreditAmount: 1200, StripeCheckoutSessionID: &session, CreatedAt: mockBaseTime.Add(-80 * time.Minute), UpdatedAt: mockBaseTime.Add(-80 * time.Minute)},
	}
}

func mockIntPtr(value int) *int {
	return &value
}

var errMockUnsupportedMutation = errors.New("mock app data is read-only")
