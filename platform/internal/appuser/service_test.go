package appuser

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/apikey"
	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
)

type fakeStore struct {
	owned          bool
	updateValues   map[string]any
	updateCalls    int
	auditLogCalls  int
	apiKeysByOwner []model.APIKey
	user           *model.User
	rewardGrants   []model.RewardGrant
	referrals      []model.UserReferral
	discord        *model.DiscordConnection
	creditGrants   map[string]*model.HostedCreditGrant
	hostedLedgers  map[string]*model.UserHostedCreditLedger
	hostedAccount  *model.UserHostedCreditAccount
}

func (f *fakeStore) CountUserAPIKeys(_ context.Context, _ uint64) (int64, error) {
	return int64(len(f.apiKeysByOwner)), nil
}
func (f *fakeStore) FindAPIKeysByOwner(_ context.Context, _ uint64) ([]model.APIKey, error) {
	return f.apiKeysByOwner, nil
}
func (f *fakeStore) ListAppUsageEvents(_ context.Context, _ uint64) ([]model.UsageEvent, error) {
	return nil, nil
}
func (f *fakeStore) GetUserByID(_ context.Context, _ uint64) (*model.User, error) {
	return f.user, nil
}
func (f *fakeStore) ListRewardGrantsByUser(_ context.Context, _ uint64) ([]model.RewardGrant, error) {
	return f.rewardGrants, nil
}
func (f *fakeStore) ListReferralsByInviterUserID(_ context.Context, _ uint64) ([]model.UserReferral, error) {
	return f.referrals, nil
}
func (f *fakeStore) FindDiscordConnectionByUserID(_ context.Context, _ uint64) (*model.DiscordConnection, error) {
	return f.discord, nil
}
func (f *fakeStore) CreateAuditLog(_ context.Context, _, _, _, _ string) error {
	f.auditLogCalls++
	return nil
}
func (f *fakeStore) AppCreateAPIKey(_ context.Context, userID uint64, planName, _, prefix, ciphertext string) (*model.APIKey, error) {
	key := model.APIKey{ID: uint64(len(f.apiKeysByOwner) + 1), OwnerUserID: &userID, PlanName: planName, KeyPrefix: prefix, KeyCiphertext: &ciphertext, Status: model.APIKeyStatusActive, AllowedModes: "external_only"}
	f.apiKeysByOwner = append(f.apiKeysByOwner, key)
	return &key, nil
}
func (f *fakeStore) FindHostedCreditGrantByIdempotencyKey(_ context.Context, key string) (*model.HostedCreditGrant, error) {
	if f.creditGrants == nil {
		return nil, nil
	}
	grant := f.creditGrants[key]
	if grant == nil {
		return nil, nil
	}
	copied := *grant
	return &copied, nil
}
func (f *fakeStore) GrantHostedCreditsToAPIKey(_ context.Context, apiKeyID, userID uint64, source model.HostedCreditGrantSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.HostedCreditGrant, bool, error) {
	if f.creditGrants == nil {
		f.creditGrants = map[string]*model.HostedCreditGrant{}
	}
	if grant := f.creditGrants[idempotencyKey]; grant != nil {
		copied := *grant
		return &copied, false, nil
	}
	for i := range f.apiKeysByOwner {
		if f.apiKeysByOwner[i].ID == apiKeyID {
			f.apiKeysByOwner[i].CreditBalance += creditAmount
			f.apiKeysByOwner[i].HostedEnabled = true
			if f.apiKeysByOwner[i].AllowedModes == "" || f.apiKeysByOwner[i].AllowedModes == "external_only" {
				f.apiKeysByOwner[i].AllowedModes = "hybrid"
			}
			defaultRuntimeMode := "hosted"
			f.apiKeysByOwner[i].DefaultRuntimeMode = &defaultRuntimeMode
			break
		}
	}
	grant := &model.HostedCreditGrant{ID: uint64(len(f.creditGrants) + 1), UserID: userID, APIKeyID: apiKeyID, SourceType: source, IdempotencyKey: idempotencyKey, CreditAmount: creditAmount, Reason: reason, MetadataJSON: metadataJSON}
	f.creditGrants[idempotencyKey] = grant
	copied := *grant
	return &copied, true, nil
}
func (f *fakeStore) GetHostedCreditAccountByUser(_ context.Context, userID uint64) (*model.UserHostedCreditAccount, error) {
	if f.hostedAccount == nil {
		return &model.UserHostedCreditAccount{UserID: userID}, nil
	}
	copied := *f.hostedAccount
	return &copied, nil
}
func (f *fakeStore) GrantHostedCreditsToUser(_ context.Context, userID uint64, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.UserHostedCreditLedger, *model.UserHostedCreditAccount, bool, error) {
	if f.hostedLedgers == nil {
		f.hostedLedgers = map[string]*model.UserHostedCreditLedger{}
	}
	if ledger := f.hostedLedgers[idempotencyKey]; ledger != nil {
		copied := *ledger
		account, _ := f.GetHostedCreditAccountByUser(context.Background(), userID)
		return &copied, account, false, nil
	}
	if f.hostedAccount == nil {
		f.hostedAccount = &model.UserHostedCreditAccount{UserID: userID}
	}
	f.hostedAccount.CreditBalance += creditAmount
	ledger := &model.UserHostedCreditLedger{ID: uint64(len(f.hostedLedgers) + 1), UserID: userID, SourceType: source, IdempotencyKey: idempotencyKey, CreditDelta: creditAmount, Reason: reason, MetadataJSON: metadataJSON}
	f.hostedLedgers[idempotencyKey] = ledger
	copied := *ledger
	account := *f.hostedAccount
	return &copied, &account, true, nil
}
func (f *fakeStore) GrantHostedCreditsToFingerprint(_ context.Context, fingerprint string, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.FingerprintCreditLedger, *model.FingerprintCreditAccount, bool, error) {
	ledger := &model.FingerprintCreditLedger{FingerprintHash: fingerprint, SourceType: source, IdempotencyKey: idempotencyKey, CreditDelta: creditAmount, Reason: reason, MetadataJSON: metadataJSON}
	account := &model.FingerprintCreditAccount{FingerprintHash: fingerprint, CreditBalance: creditAmount, BonusGranted: true}
	return ledger, account, true, nil
}
func (f *fakeStore) TransferAnonymousCreditsToUser(_ context.Context, _ string, _ uint64) (int, error) {
	return 0, nil
}
func (f *fakeStore) UpdateAPIKey(_ context.Context, _ uint64, values map[string]any) error {
	f.updateCalls++
	f.updateValues = values
	return nil
}
func (f *fakeStore) IsAPIKeyOwnedByUser(_ context.Context, _, _ uint64) (bool, error) {
	return f.owned, nil
}
func (f *fakeStore) ListUserHostedCreditLedger(_ context.Context, _ uint64, _ bool) ([]model.UserHostedCreditLedger, error) {
	return nil, nil
}

type fakeBilling struct{}

func (fakeBilling) ListOrdersByUser(_ context.Context, _ uint64) ([]model.Order, error) {
	return nil, nil
}
func (fakeBilling) Pricing() []model.PricingPack { return nil }

type fakeBillingWithData struct {
	orders  []model.Order
	pricing []model.PricingPack
}

func (f fakeBillingWithData) ListOrdersByUser(_ context.Context, _ uint64) ([]model.Order, error) {
	return f.orders, nil
}

func (f fakeBillingWithData) Pricing() []model.PricingPack { return f.pricing }

type usageStore struct {
	*fakeStore
	usage []model.UsageEvent
}

func (s *usageStore) ListAppUsageEvents(_ context.Context, _ uint64) ([]model.UsageEvent, error) {
	return s.usage, nil
}

type fakeGrowthManager struct {
	connectCalls int
	grantCalls   int
	connection   *model.DiscordConnection
	grantResult  *growthsvc.RewardGrantResult
}

func (f *fakeGrowthManager) ConnectDiscord(_ context.Context, _ uint64, _, _ string, guildMember bool) (*model.DiscordConnection, error) {
	f.connectCalls++
	if f.connection == nil {
		f.connection = &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   guildMember,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		}
	}
	copied := *f.connection
	return &copied, nil
}

func (f *fakeGrowthManager) GrantDiscordJoinReward(_ context.Context, _ uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error) {
	f.grantCalls++
	if f.grantResult != nil {
		return f.grantResult, nil
	}
	return &growthsvc.RewardGrantResult{
		Created: false,
		Grant: &model.RewardGrant{
			SourceType:     model.RewardSourceDiscordJoin,
			IdempotencyKey: "discord-join:42:discord-42",
			AmountTotal:    rewardAmount,
		},
	}, nil
}

func TestUpdateAPIKeyOnlyPersistsStatusAndNote(t *testing.T) {
	t.Parallel()

	status := string(model.APIKeyStatusDisabled)
	note := "rotate after handoff"
	store := &fakeStore{owned: true}
	cipher := testAPIKeyCipher(t)
	svc := NewService(store, fakeBillingWithData{
		orders: []model.Order{
			{ID: 1, PackKind: model.PackKindExternalGeneration},
			{ID: 2, PackKind: model.PackKindHostedCredits},
		},
		pricing: []model.PricingPack{
			{Code: "external-100", AmountTotal: 500, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)},
			{Code: "hosted-300", AmountTotal: 2900, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)},
		},
	}, "salt", cipher)

	err := svc.UpdateAPIKey(context.Background(), 42, 7, UpdateAPIKeyRequest{
		Status: &status,
		Note:   &note,
	})

	require.NoError(t, err)
	require.Equal(t, 1, store.updateCalls)
	require.Equal(t, map[string]any{
		"status": status,
		"note":   note,
	}, store.updateValues)
	require.Equal(t, 1, store.auditLogCalls)
	_, hasQuota := store.updateValues["quota_total"]
	require.False(t, hasQuota)
	_, hasPlan := store.updateValues["plan_name"]
	require.False(t, hasPlan)
}

func TestUpdateAPIKeyRejectsForeignKey(t *testing.T) {
	t.Parallel()

	status := string(model.APIKeyStatusDisabled)
	store := &fakeStore{owned: false}
	cipher := testAPIKeyCipher(t)
	svc := NewService(store, fakeBillingWithData{
		orders: []model.Order{
			{ID: 1, PackKind: model.PackKindExternalGeneration},
			{ID: 2, PackKind: model.PackKindHostedCredits},
		},
		pricing: []model.PricingPack{
			{Code: "external-100", AmountTotal: 500, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)},
			{Code: "hosted-300", AmountTotal: 2900, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)},
		},
	}, "salt", cipher)

	err := svc.UpdateAPIKey(context.Background(), 42, 9, UpdateAPIKeyRequest{Status: &status})

	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.Zero(t, store.updateCalls)
	require.Zero(t, store.auditLogCalls)
}

func TestOverviewIncludesRewardInviteAndDiscordState(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		apiKeysByOwner: []model.APIKey{
			{QuotaTotal: intPtr(10), QuotaUsed: 3},
		},
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 10, AmountUsed: 4},
			{AmountTotal: 3, AmountUsed: 1},
		},
		referrals: []model.UserReferral{
			{InvitedUserID: 100},
			{InvitedUserID: 101, ActivatedAt: timePtr()},
		},
		discord:       &model.DiscordConnection{UserID: 42, GuildMember: true},
		hostedAccount: &model.UserHostedCreditAccount{UserID: 42, CreditBalance: 180},
		hostedLedgers: map[string]*model.UserHostedCreditLedger{
			"signup-hosted-credits:42": {UserID: 42, SourceType: model.HostedCreditLedgerSourceSignupBonus, IdempotencyKey: "signup-hosted-credits:42", CreditDelta: SignupHostedCreditBonus},
		},
	}
	cipher := testAPIKeyCipher(t)
	svc := NewService(store, fakeBillingWithData{
		orders: []model.Order{
			{ID: 1, PackKind: model.PackKindExternalGeneration},
			{ID: 2, PackKind: model.PackKindHostedCredits},
		},
		pricing: []model.PricingPack{
			{Code: "external-100", AmountTotal: 500, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)},
			{Code: "hosted-300", AmountTotal: 2900, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)},
		},
	}, "salt", cipher)

	overview, err := svc.Overview(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 7, overview.TotalRemaining)
	require.Equal(t, 180, overview.HostedCreditBalance)
	require.Equal(t, 8, overview.RewardRemaining)
	require.Equal(t, "invite-xyz", overview.InviteCode)
	require.Equal(t, growthsvc.MaxReferralsPerInviter, overview.InviteLimit)
	require.Equal(t, growthsvc.MaxReferralsPerInviter-2, overview.InviteRemaining)
	require.Equal(t, growthsvc.InviteActivationRewardAmount, overview.RewardPerInvite)
	require.Equal(t, 2, overview.ReferralCount)
	require.Equal(t, 1, overview.ActivatedReferralCount)
	require.True(t, overview.DiscordConnected)
	require.True(t, overview.DiscordGuildMember)
	require.Equal(t, 2, overview.RecentOrdersCount)
	require.Len(t, overview.Pricing, 2)
	require.Equal(t, "external-100", overview.Pricing[0].Code)
}

func TestOverviewGrantsSignupHostedCreditsToUserWithoutCreatingAPIKey(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
	}
	svc := NewService(store, fakeBilling{}, "salt", testAPIKeyCipher(t))

	overview, err := svc.Overview(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, SignupHostedCreditBonus, overview.HostedCreditBalance)
	require.Empty(t, store.apiKeysByOwner)

	ledger := store.hostedLedgers["signup-hosted-credits:42"]
	require.NotNil(t, ledger)
	require.Equal(t, model.HostedCreditLedgerSourceSignupBonus, ledger.SourceType)
	require.Equal(t, SignupHostedCreditBonus, ledger.CreditDelta)
}

func TestEnsureSignupHostedCreditsIsIdempotent(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	svc := NewService(store, fakeBilling{}, "salt", testAPIKeyCipher(t))

	require.NoError(t, svc.EnsureSignupHostedCredits(context.Background(), 42))
	require.NoError(t, svc.EnsureSignupHostedCredits(context.Background(), 42))

	account, err := store.GetHostedCreditAccountByUser(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, SignupHostedCreditBonus, account.CreditBalance)
	require.Len(t, store.hostedLedgers, 1)

	ledger := store.hostedLedgers["signup-hosted-credits:42"]
	require.NotNil(t, ledger)
	require.Equal(t, model.HostedCreditLedgerSourceSignupBonus, ledger.SourceType)
	require.Equal(t, SignupHostedCreditBonus, ledger.CreditDelta)
	require.Equal(t, "new user signup hosted credits", ledger.Reason)
}

func TestListAPIKeysReturnsCustomerSafeView(t *testing.T) {
	t.Parallel()

	lastUsedAt := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		apiKeysByOwner: []model.APIKey{{
			ID:                 7,
			KeyPrefix:          "cop_live_demo",
			KeyCiphertext:      stringPtr("ciphertext-demo"),
			Status:             model.APIKeyStatusActive,
			PlanName:           "Growth",
			AllowedModes:       "hybrid",
			HostedEnabled:      true,
			QuotaTotal:         intPtr(120),
			QuotaUsed:          80,
			CreditBalance:      200,
			DefaultRuntimeMode: stringPtr("hosted"),
			LastUsedAt:         &lastUsedAt,
			CreatedAt:          time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		}},
	}
	svc := NewService(store, fakeBilling{}, "salt", testAPIKeyCipher(t))

	keys, err := svc.ListAPIKeys(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, "cop_live_demo", keys[0].KeyPrefix)
	require.True(t, keys[0].PlaintextAvailable)
	require.Equal(t, 40, keys[0].QuotaRemaining)
	require.Zero(t, keys[0].CreditBalance)
}

func TestListUsageEventsIncludesHostedEventsWithoutCostDetails(t *testing.T) {
	t.Parallel()

	svc := NewService(&usageStore{
		fakeStore: &fakeStore{},
		usage: []model.UsageEvent{
			{ID: 1, Mode: model.UsageModePaid},
			{
				ID:                    2,
				Mode:                  model.UsageModeHosted,
				UnitType:              "credit",
				BilledUnits:           6,
				ModelName:             stringPtr("gpt-5.4"),
				SettledCredits:        6,
				MarkupBPS:             3000,
				HostedPricingRuleID:   3,
				UpstreamCostMicrousd:  15232,
				UncappedChargeCredits: 6,
				ProfitMicrousd:        44768,
				CapApplied:            true,
			},
			{ID: 3, Mode: model.UsageModeFree},
		},
	}, fakeBilling{}, "salt", testAPIKeyCipher(t))

	events, err := svc.ListUsageEvents(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, uint64(1), events[0].ID)
	require.Equal(t, uint64(2), events[1].ID)
	require.Equal(t, uint64(3), events[2].ID)

	raw, err := json.Marshal(events[1])
	require.NoError(t, err)
	payload := string(raw)
	require.Contains(t, payload, `"settled_credits":6`)
	require.NotContains(t, payload, "markup_bps")
	require.NotContains(t, payload, "hosted_pricing_rule_id")
	require.NotContains(t, payload, "upstream_cost_microusd")
	require.NotContains(t, payload, "uncapped_charge_credits")
	require.NotContains(t, payload, "profit_microusd")
	require.NotContains(t, payload, "cap_applied")
}

func TestGetAPIKeyPlaintextReturnsStoredValueForOwnedKey(t *testing.T) {
	t.Parallel()

	cipher := testAPIKeyCipher(t)
	ciphertext, err := cipher.Encrypt("cop_live_secret_123")
	require.NoError(t, err)
	store := &fakeStore{
		apiKeysByOwner: []model.APIKey{{
			ID:            7,
			KeyPrefix:     "cop_live_demo",
			KeyCiphertext: &ciphertext,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Growth",
		}},
	}
	svc := NewService(store, fakeBilling{}, "salt", cipher)

	plaintext, err := svc.GetAPIKeyPlaintext(context.Background(), 42, 7)
	require.NoError(t, err)
	require.Equal(t, "cop_live_secret_123", plaintext)
	require.Equal(t, 1, store.auditLogCalls)
}

func TestGetAPIKeyPlaintextRejectsLegacyKey(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		apiKeysByOwner: []model.APIKey{{
			ID:        7,
			KeyPrefix: "cop_live_legacy",
			Status:    model.APIKeyStatusActive,
			PlanName:  "Legacy",
		}},
	}
	svc := NewService(store, fakeBilling{}, "salt", testAPIKeyCipher(t))

	_, err := svc.GetAPIKeyPlaintext(context.Background(), 42, 7)
	require.ErrorIs(t, err, apikey.ErrPlaintextUnavailable)
	require.Zero(t, store.auditLogCalls)
}

func TestGetAPIKeyPlaintextRejectsForeignKey(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		apiKeysByOwner: []model.APIKey{{
			ID:        8,
			KeyPrefix: "cop_live_other",
			Status:    model.APIKeyStatusActive,
			PlanName:  "Other",
		}},
	}
	svc := NewService(store, fakeBilling{}, "salt", testAPIKeyCipher(t))

	_, err := svc.GetAPIKeyPlaintext(context.Background(), 42, 7)
	require.ErrorIs(t, err, apikey.ErrAPIKeyNotFound)
}

func TestGrowthReturnsUserFacingRewardReferralAndDiscordData(t *testing.T) {
	t.Parallel()

	registeredAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	connectedAt := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	rewardGrantedAt := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	store := &fakeStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{
				SourceType:   model.RewardSourceInviteActivation,
				AmountTotal:  10,
				AmountUsed:   4,
				Reason:       "invite activation reward",
				MetadataJSON: `{"invite_code":"invite-xyz"}`,
			},
		},
		referrals: []model.UserReferral{
			{
				InviteCode:      "invite-xyz",
				RegisteredAt:    registeredAt,
				RewardGrantedAt: &rewardGrantedAt,
			},
		},
		discord: &model.DiscordConnection{
			UserID:          42,
			Username:        "officecli-user",
			GuildMember:     true,
			ConnectedAt:     connectedAt,
			RewardGrantedAt: &rewardGrantedAt,
		},
	}
	svc := NewService(store, fakeBilling{}, "salt", testAPIKeyCipher(t))

	growth, err := svc.Growth(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "invite-xyz", growth.InviteCode)
	require.Equal(t, growthsvc.MaxReferralsPerInviter, growth.InviteLimit)
	require.Equal(t, growthsvc.MaxReferralsPerInviter-1, growth.InviteRemaining)
	require.Equal(t, growthsvc.InviteActivationRewardAmount, growth.RewardPerInvite)
	require.Equal(t, 6, growth.RewardRemaining)
	require.Len(t, growth.RewardGrants, 1)
	require.Equal(t, 6, growth.RewardGrants[0].Remaining)
	require.Len(t, growth.Referrals, 1)
	require.Equal(t, registeredAt, growth.Referrals[0].RegisteredAt)
	require.NotNil(t, growth.DiscordConnection)
	require.Equal(t, "officecli-user", growth.DiscordConnection.Username)
	require.True(t, growth.DiscordConnection.GuildMember)
	require.Equal(t, connectedAt, growth.DiscordConnection.ConnectedAt)
	require.Equal(t, "verified", growth.DiscordConnection.VerificationStatus)
}

func TestConnectDiscordReturnsBlockedVerificationStateWithoutReward(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 4, AmountUsed: 1},
		},
		discord: &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   false,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		},
	}
	growth := &fakeGrowthManager{
		connection: &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   false,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		},
	}
	svc := NewService(store, fakeBilling{}, "salt", testAPIKeyCipher(t), growth)

	response, err := svc.ConnectDiscord(context.Background(), 42, ConnectDiscordRequest{
		DiscordUserID: "discord-42",
		Username:      "officecli-user",
	})

	require.NoError(t, err)
	require.False(t, response.RewardGranted)
	require.Equal(t, 3, response.RewardRemaining)
	require.Equal(t, "verification_blocked", response.Connection.VerificationStatus)
	require.Equal(t, DiscordGuildVerificationBlockedReason, response.Connection.VerificationBlockedReason)
	require.Equal(t, 1, growth.connectCalls)
	require.Zero(t, growth.grantCalls)
}

func TestDiscordStatusReturnsConnectionAndRewardBalance(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		user: &model.User{ID: 42, InviteCode: "invite-xyz"},
		rewardGrants: []model.RewardGrant{
			{AmountTotal: 5, AmountUsed: 2},
		},
		discord: &model.DiscordConnection{
			UserID:        42,
			DiscordUserID: "discord-42",
			Username:      "officecli-user",
			GuildMember:   false,
			ConnectedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		},
	}
	svc := NewService(store, fakeBilling{}, "salt", testAPIKeyCipher(t))

	status, err := svc.DiscordStatus(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 3, status.RewardRemaining)
	require.NotNil(t, status.Connection)
	require.Equal(t, "verification_blocked", status.Connection.VerificationStatus)
}

func testAPIKeyCipher(t *testing.T) *apikey.Cipher {
	t.Helper()
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	return cipher
}

func intPtr(v int) *int { return &v }

func timePtr() *time.Time {
	now := time.Now().UTC()
	return &now
}

func stringPtr(v string) *string { return &v }
