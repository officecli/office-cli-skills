package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/officecli/officecli/platform/internal/model"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

var mockBaseTime = time.Date(2026, 5, 17, 9, 30, 0, 0, time.UTC)

func mockOverview() *model.OverviewStats {
	usageTrend := make([]model.OverviewUsageTrendPoint, 0, 7)
	start := mockBaseTime.AddDate(0, 0, -6)
	for i := 0; i < 7; i++ {
		date := start.AddDate(0, 0, i)
		checks := int64(48 + i*9)
		consumes := int64(30 + i*7)
		blocked := int64(i % 3)
		usageTrend = append(usageTrend, model.OverviewUsageTrendPoint{
			Date:     date.Format("2006-01-02"),
			Checks:   checks,
			Consumes: consumes,
			Blocked:  blocked,
			Allowed:  checks + consumes - blocked,
			Total:    checks + consumes,
		})
	}
	return &model.OverviewStats{
		TotalAPIKeys:          4,
		ActiveAPIKeys:         3,
		DisabledAPIKeys:       1,
		ExpiredAPIKeys:        1,
		FreeMachines:          3,
		ChecksLast24h:         128,
		ConsumesLast24h:       86,
		BlockedLast24h:        7,
		TotalUsers:            4,
		PaidOrdersLast24h:     2,
		PaidQuotaAddedLast24h: 1200,
		RemainingPaidQuota:    3950,
		UsageTrend:            usageTrend,
		ModeBreakdown: []model.OverviewBreakdownItem{
			{Key: "free", Label: "Free", Value: 34},
			{Key: "reward", Label: "Reward", Value: 18},
			{Key: "paid", Label: "Paid", Value: 27},
			{Key: "hosted", Label: "Hosted", Value: 49},
		},
		ResultBreakdown: []model.OverviewBreakdownItem{
			{Key: "allowed", Label: "Allowed", Value: 121},
			{Key: "blocked", Label: "Blocked", Value: 7},
		},
		APIKeyStatusBreakdown: []model.OverviewBreakdownItem{
			{Key: "active", Label: "Active", Value: 2},
			{Key: "disabled", Label: "Disabled", Value: 1},
			{Key: "expired", Label: "Expired", Value: 1},
			{Key: "other", Label: "Other", Value: 0},
		},
	}
}

func mockUsers(query string) []model.User {
	users := []model.User{
		{ID: 101, GoogleSub: model.StringPtr("mock-google-101"), Email: "ava.chen@example.com", Name: "Ava Chen", InviteCode: "AVA2026", Status: model.UserStatusActive, CreditBalance: 1380, CreatedAt: mockBaseTime.Add(-72 * time.Hour), UpdatedAt: mockBaseTime.Add(-2 * time.Hour)},
		{ID: 102, GoogleSub: model.StringPtr("mock-google-102"), Email: "ops.disabled@example.com", Name: "Disabled Operator", InviteCode: "OPSLOCK", Status: model.UserStatusDisabled, CreditBalance: 0, CreatedAt: mockBaseTime.Add(-48 * time.Hour), UpdatedAt: mockBaseTime.Add(-4 * time.Hour)},
		{ID: 103, GoogleSub: model.StringPtr("mock-google-103"), Email: "nora.patel@example.com", Name: "Nora Patel", InviteCode: "NORA100", Status: model.UserStatusActive, CreditBalance: 640, CreatedAt: mockBaseTime.Add(-24 * time.Hour), UpdatedAt: mockBaseTime.Add(-30 * time.Minute)},
		{ID: 104, GoogleSub: model.StringPtr("mock-google-104"), Email: "long.email.operator+regional-review@example.co", Name: "Regional Review Owner", InviteCode: "REGION-LONG", Status: model.UserStatusActive, CreditBalance: 90, CreatedAt: mockBaseTime.Add(-12 * time.Hour), UpdatedAt: mockBaseTime.Add(-15 * time.Minute)},
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return users
	}
	filtered := make([]model.User, 0, len(users))
	for _, user := range users {
		haystack := strings.ToLower(user.Email + " " + user.Name + " " + user.InviteCode + " " + string(user.Status))
		if strings.Contains(haystack, query) || strings.Contains(toID(user.ID), query) {
			filtered = append(filtered, user)
		}
	}
	return filtered
}

func mockAPIKeys(ownerUserID *uint64) []model.APIKey {
	planCodeHosted := "hosted-team"
	planCodeHybrid := "hybrid-pro"
	planCodeExternal := "external-500"
	defaultHosted := "hosted"
	defaultExternal := "external"
	noteHosted := "Mock hosted key with account-level credits."
	noteHybrid := "Mock hybrid key with long operator note used for table wrapping validation."
	noteExternal := "Mock external quota key."
	lastUsed := mockBaseTime.Add(-18 * time.Minute)
	expires := mockBaseTime.Add(30 * 24 * time.Hour)
	owner101 := uint64(101)
	owner102 := uint64(102)
	owner103 := uint64(103)
	totalExternal := 500
	totalHybrid := 1500
	keys := []model.APIKey{
		{ID: 201, OwnerUserID: &owner101, KeyPrefix: "ocli_mock_hosted_7KQ2", Status: model.APIKeyStatusActive, PlanName: "Hosted Team", PlanCode: &planCodeHosted, AllowedModes: "hosted_only", HostedEnabled: true, DefaultRuntimeMode: &defaultHosted, CreditBalance: 1200, LastUsedAt: &lastUsed, Note: &noteHosted, PlaintextAvailable: true, CreatedAt: mockBaseTime.Add(-70 * time.Hour), UpdatedAt: mockBaseTime.Add(-20 * time.Minute)},
		{ID: 202, OwnerUserID: &owner103, KeyPrefix: "ocli_mock_hybrid_LONGPREFIX", Status: model.APIKeyStatusActive, PlanName: "Hybrid Pro", PlanCode: &planCodeHybrid, AllowedModes: "hybrid", HostedEnabled: true, DefaultRuntimeMode: &defaultHosted, CreditBalance: 450, QuotaTotal: &totalHybrid, QuotaUsed: 320, QuotaRemaining: intPtr(1180), ExpiresAt: &expires, LastUsedAt: &lastUsed, Note: &noteHybrid, PlaintextAvailable: true, CreatedAt: mockBaseTime.Add(-36 * time.Hour), UpdatedAt: mockBaseTime.Add(-10 * time.Minute)},
		{ID: 203, OwnerUserID: &owner101, KeyPrefix: "ocli_mock_external_Q9X1", Status: model.APIKeyStatusActive, PlanName: "External 500", PlanCode: &planCodeExternal, AllowedModes: "external_only", HostedEnabled: false, DefaultRuntimeMode: &defaultExternal, QuotaTotal: &totalExternal, QuotaUsed: 480, QuotaRemaining: intPtr(20), ExpiresAt: &expires, Note: &noteExternal, CreatedAt: mockBaseTime.Add(-22 * time.Hour), UpdatedAt: mockBaseTime.Add(-1 * time.Hour)},
		{ID: 204, OwnerUserID: &owner102, KeyPrefix: "ocli_mock_disabled_0Z9", Status: model.APIKeyStatusDisabled, PlanName: "Suspended Hosted", AllowedModes: "hosted_only", HostedEnabled: true, DefaultRuntimeMode: &defaultHosted, CreditBalance: 20, CreatedAt: mockBaseTime.Add(-14 * time.Hour), UpdatedAt: mockBaseTime.Add(-3 * time.Hour)},
	}
	if ownerUserID == nil {
		return keys
	}
	filtered := make([]model.APIKey, 0, len(keys))
	for _, key := range keys {
		if key.OwnerUserID != nil && *key.OwnerUserID == *ownerUserID {
			filtered = append(filtered, key)
		}
	}
	return filtered
}

func mockUsageEvents(filter sqlstore.UsageEventFilter) []model.UsageEvent {
	requestID1 := "req_mock_20260517_allowed_hosted"
	requestID2 := "req_mock_20260517_blocked_free"
	requestID3 := "req_mock_20260517_paid_external"
	reasonBlocked := "daily_free_limit_exhausted"
	reasonAllowed := "hosted_credits_reserved"
	cli := "officecli/0.2.34"
	docPPTX := "pptx"
	docIMG := "img"
	runtimeHosted := "hosted"
	runtimeExternal := "external"
	providerOpenAI := "openai"
	modelText := "gpt-4.1"
	modelImage := "gpt-image-2"
	clientIP := "127.0.0.1"
	forwarded := "10.0.0.12, 172.18.0.2"
	userAgent := "OfficeCLI mock e2e user-agent with enough length to validate wrapping in the admin event card"
	host := "officecli.localhost:18080"
	path := "/api/license/consume"
	method := "POST"
	owner101 := uint64(101)
	owner103 := uint64(103)
	key201 := uint64(201)
	key202 := uint64(202)
	rows := []model.UsageEvent{
		{ID: 401, RequestID: &requestID1, FingerprintHash: "fp_mock_7d2e3a9b82a8f1e5c6b41b450a-long-hash", Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, APIKeyID: &key201, Result: model.UsageResultAllowed, ReasonCode: &reasonAllowed, CLIVersion: &cli, DocumentType: &docPPTX, RuntimeMode: &runtimeHosted, UserID: &owner101, Provider: &providerOpenAI, ModelName: &modelText, PromptTokens: 18420, CompletionTokens: 2470, ReasoningTokens: 512, ReservedCredits: 16, SettledCredits: 11, RefundCredits: 5, MarkupBPS: 3000, UpstreamCostMicrousd: 18300, UncappedChargeCredits: 11, ProfitMicrousd: 5490, ClientIP: &clientIP, ForwardedFor: &forwarded, UserAgent: &userAgent, RequestHost: &host, RequestPath: &path, RequestMethod: &method, CreatedAt: mockBaseTime.Add(-9 * time.Minute)},
		{ID: 402, RequestID: &requestID2, FingerprintHash: "fp_mock_blocked_review_needed_001", Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultBlocked, ReasonCode: &reasonBlocked, CLIVersion: &cli, DocumentType: &docIMG, RuntimeMode: &runtimeHosted, Provider: &providerOpenAI, ModelName: &modelImage, ImageCount: 2, ClientIP: &clientIP, ForwardedFor: &forwarded, UserAgent: &userAgent, RequestHost: &host, RequestPath: &path, RequestMethod: &method, CreatedAt: mockBaseTime.Add(-18 * time.Minute)},
		{ID: 403, RequestID: &requestID3, FingerprintHash: "fp_mock_paid_external_device", Mode: model.UsageModePaid, Action: model.UsageActionStatus, APIKeyID: &key202, Result: model.UsageResultAllowed, CLIVersion: &cli, DocumentType: &docPPTX, RuntimeMode: &runtimeExternal, UserID: &owner103, BilledUnits: 1, UnitType: "document", ClientIP: &clientIP, ForwardedFor: &forwarded, UserAgent: &userAgent, RequestHost: &host, RequestPath: &path, RequestMethod: &method, CreatedAt: mockBaseTime.Add(-42 * time.Minute)},
	}
	filtered := make([]model.UsageEvent, 0, len(rows))
	for _, event := range rows {
		if filter.Mode != "" && string(event.Mode) != filter.Mode {
			continue
		}
		if filter.Result != "" && string(event.Result) != filter.Result {
			continue
		}
		if filter.ReasonCode != "" && (event.ReasonCode == nil || !strings.Contains(strings.ToLower(*event.ReasonCode), strings.ToLower(filter.ReasonCode))) {
			continue
		}
		if filter.Fingerprint != "" && !strings.Contains(strings.ToLower(event.FingerprintHash), strings.ToLower(filter.Fingerprint)) {
			continue
		}
		if filter.APIKeyID != nil && (event.APIKeyID == nil || *event.APIKeyID != *filter.APIKeyID) {
			continue
		}
		if filter.UserID != nil && (event.UserID == nil || *event.UserID != *filter.UserID) {
			continue
		}
		if filter.ClientIP != "" && (event.ClientIP == nil || !strings.Contains(*event.ClientIP, filter.ClientIP)) {
			continue
		}
		if filter.RequestID != "" && (event.RequestID == nil || !strings.Contains(*event.RequestID, filter.RequestID)) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func mockOrders() []model.Order {
	note := "Mock order currently under manual refund review."
	targetID := uint64(203)
	return []model.Order{
		{ID: 501, UserID: 101, Status: model.OrderStatusPaid, Currency: "usd", AmountTotal: 9900, PackCode: "hosted-1200", PackName: "Hosted 1200", PackKind: model.PackKindHostedCredits, CreditAmount: 1200, CreatedAt: mockBaseTime.Add(-3 * time.Hour), UpdatedAt: mockBaseTime.Add(-2 * time.Hour)},
		{ID: 502, UserID: 103, Status: model.OrderStatusPending, Currency: "usd", AmountTotal: 4900, PackCode: "external-500", PackName: "External 500", PackKind: model.PackKindExternalGeneration, QuotaAmount: 500, TargetAPIKeyID: &targetID, CreatedAt: mockBaseTime.Add(-90 * time.Minute), UpdatedAt: mockBaseTime.Add(-90 * time.Minute)},
		{ID: 503, UserID: 102, Status: model.OrderStatusFailed, Currency: "usd", AmountTotal: 2900, PackCode: "hosted-300", PackName: "Hosted 300", PackKind: model.PackKindHostedCredits, CreditAmount: 300, Note: &note, CreatedAt: mockBaseTime.Add(-40 * time.Minute), UpdatedAt: mockBaseTime.Add(-35 * time.Minute)},
	}
}

func mockBillingEvents() []model.BillingEvent {
	order501 := uint64(501)
	order503 := uint64(503)
	errMsg := "card_declined: test webhook payload kept for admin wrapping validation"
	processed := mockBaseTime.Add(-2 * time.Hour)
	return []model.BillingEvent{
		{ID: 601, OrderID: &order501, EventID: "evt_mock_checkout_completed_501", EventType: "checkout.session.completed", Status: model.BillingEventStatusHandled, PayloadJSON: "{}", ProcessedAt: &processed, CreatedAt: mockBaseTime.Add(-2*time.Hour - 2*time.Minute)},
		{ID: 602, EventID: "evt_mock_customer_updated_ignored", EventType: "customer.updated", Status: model.BillingEventStatusIgnored, PayloadJSON: "{}", ProcessedAt: &processed, CreatedAt: mockBaseTime.Add(-75 * time.Minute)},
		{ID: 603, OrderID: &order503, EventID: "evt_mock_payment_failed_with_long_identifier", EventType: "payment_intent.payment_failed", Status: model.BillingEventStatusFailed, PayloadJSON: "{}", ErrorMessage: &errMsg, CreatedAt: mockBaseTime.Add(-35 * time.Minute)},
	}
}

func mockGrowth() *GrowthSnapshot {
	activated := mockBaseTime.Add(-20 * time.Hour)
	rewarded := mockBaseTime.Add(-19 * time.Hour)
	return &GrowthSnapshot{
		RewardGrants: []model.RewardGrant{
			{ID: 701, UserID: 101, SourceType: model.RewardSourceInviteActivation, IdempotencyKey: "reward_invite_mock_101_103", AmountTotal: 100, AmountUsed: 24, Reason: "Invite activation reward", MetadataJSON: `{"invite_code":"AVA2026"}`, CreatedAt: mockBaseTime.Add(-20 * time.Hour), UpdatedAt: mockBaseTime.Add(-2 * time.Hour)},
			{ID: 702, UserID: 103, SourceType: model.RewardSourceAdminManual, IdempotencyKey: "reward_manual_mock_long_key_for_table", AmountTotal: 250, AmountUsed: 0, Reason: "Manual onboarding extension", MetadataJSON: `{}`, CreatedAt: mockBaseTime.Add(-5 * time.Hour), UpdatedAt: mockBaseTime.Add(-5 * time.Hour)},
		},
		HostedCreditGrants: []model.HostedCreditGrant{
			{ID: 711, UserID: 101, APIKeyID: 201, SourceType: model.HostedCreditGrantSourceSignup, IdempotencyKey: "hosted_signup_mock_101", CreditAmount: 100, Reason: "Signup hosted credits", MetadataJSON: `{}`, CreatedAt: mockBaseTime.Add(-72 * time.Hour)},
			{ID: 712, UserID: 103, APIKeyID: 202, SourceType: model.HostedCreditGrantSourceInviteActivation, IdempotencyKey: "hosted_invite_mock_101_103", CreditAmount: 100, Reason: "Invite activation hosted credits", MetadataJSON: `{}`, CreatedAt: mockBaseTime.Add(-20 * time.Hour)},
		},
		Referrals: []model.UserReferral{
			{ID: 721, InviterUserID: 101, InvitedUserID: 103, InviteCode: "AVA2026", RegisteredAt: mockBaseTime.Add(-24 * time.Hour), ActivatedAt: &activated, RewardGrantedAt: &rewarded, CreatedAt: mockBaseTime.Add(-24 * time.Hour), UpdatedAt: rewarded},
		},
		DiscordConnections: []model.DiscordConnection{
			{ID: 731, UserID: 101, DiscordUserID: "118812341234123412", Username: "ava.mock", GuildMember: true, ConnectedAt: mockBaseTime.Add(-3 * time.Hour), RewardGrantedAt: &rewarded, CreatedAt: mockBaseTime.Add(-3 * time.Hour), UpdatedAt: rewarded},
			{ID: 732, UserID: 104, DiscordUserID: "229912341234123412", Username: "regional-review-owner", GuildMember: false, ConnectedAt: mockBaseTime.Add(-2 * time.Hour), CreatedAt: mockBaseTime.Add(-2 * time.Hour), UpdatedAt: mockBaseTime.Add(-2 * time.Hour)},
		},
	}
}

func mockQuotaSources(filter QuotaSourcesFilter) *QuotaSources {
	growth := mockGrowth()
	var rewards []model.RewardGrant
	for _, grant := range growth.RewardGrants {
		if filter.UserID != 0 && grant.UserID != filter.UserID {
			continue
		}
		rewards = append(rewards, grant)
	}
	keys := mockAPIKeys(nil)
	var paid []model.APIKey
	var hosted []model.APIKey
	for _, key := range keys {
		if filter.UserID != 0 && (key.OwnerUserID == nil || *key.OwnerUserID != filter.UserID) {
			continue
		}
		if filter.KeyPrefix != "" && !strings.Contains(strings.ToLower(key.KeyPrefix), strings.ToLower(filter.KeyPrefix)) {
			continue
		}
		if key.QuotaTotal != nil {
			paid = append(paid, key)
		}
		if key.SupportsHosted() || key.CreditBalance > 0 {
			hosted = append(hosted, key)
		}
	}
	return &QuotaSources{RewardGrants: rewards, PaidExternalKeys: paid, HostedKeys: hosted}
}

func mockHostedPricingRules() []model.HostedPricingRule {
	markup := 3000
	return []model.HostedPricingRule{
		{ID: 801, DocumentProfile: "text", Provider: "openai", Model: "gpt-4.1", TextModelKey: "text_default", PromptPer1KCostMicrousd: 2, OutputPer1KCostMicrousd: 8, ReasoningPer1KCostMicrousd: 8, MinimumChargeCredits: 2, MarkupBPS: &markup, Enabled: true},
		{ID: 802, DocumentProfile: "image", Provider: "openai", Model: "gpt-image-2", ImageModelKey: "image_default", ImagePerAssetCostMicrousd: 100000, MinimumChargeCredits: 10, MarkupBPS: &markup, Enabled: true},
	}
}

func mockHostedBillingConfig() *HostedBillingConfig {
	return &HostedBillingConfig{
		Settings: model.HostedPricingSetting{ID: 1, MarkupBPS: 3000, Currency: "usd", CreditsPerUSD: 100},
		ModelConfigs: []model.HostedModelPricingConfig{
			{ID: 811, Key: "text_default", Kind: model.HostedModelPricingKindText, Provider: "openai", Model: "gpt-4.1", PromptPer1MCostMicrousd: 2000, OutputPer1MCostMicrousd: 8000, ReasoningPer1MCostMicrousd: 8000, PromptPer1MCostCredits: 1, OutputPer1MCostCredits: 1, ReasoningPer1MCostCredits: 1, Enabled: true},
			{ID: 812, Key: "image_default", Kind: model.HostedModelPricingKindImage, Provider: "openai", Model: "gpt-image-2", PromptPer1MCostMicrousd: 0, OutputPer1MCostMicrousd: 0, ReasoningPer1MCostMicrousd: 0, Enabled: true},
		},
		Rules: mockHostedPricingRules(),
		Packs: []model.HostedCreditPack{
			{ID: 821, Code: "hosted-300", Name: "Hosted 300", Description: "300 hosted credits for low-volume runs.", Currency: "usd", AmountTotal: 2900, CreditAmount: 300, Enabled: true},
			{ID: 822, Code: "hosted-1200", Name: "Hosted 1200", Description: "1200 hosted credits for team validation.", Currency: "usd", AmountTotal: 9900, CreditAmount: 1200, Enabled: true},
		},
	}
}

func intPtr(value int) *int {
	return &value
}

func toID(value uint64) string {
	return strconv.FormatUint(value, 10)
}
