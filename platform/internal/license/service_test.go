package license

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
	rewardsvc "github.com/officecli/officecli/platform/internal/reward"
)

type fakeAPIKeyStore struct {
	mu  sync.Mutex
	key *model.APIKey
}

func (f *fakeAPIKeyStore) FindByHash(_ context.Context, _ string) (*model.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.key == nil {
		return nil, nil
	}
	copied := *f.key
	return &copied, nil
}
func (f *fakeAPIKeyStore) TouchLastUsedAt(_ context.Context, _ uint64, usedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.key != nil {
		f.key.LastUsedAt = &usedAt
	}
	return nil
}
func (f *fakeAPIKeyStore) ConsumePaidByHash(_ context.Context, _ string) (*model.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.key == nil {
		return nil, nil
	}
	if f.key.QuotaTotal != nil && f.key.PaidQuotaRemaining() <= 0 {
		return nil, ErrPaidQuotaExhausted
	}
	f.key.QuotaUsed++
	copied := *f.key
	return &copied, nil
}

type fakeFreeQuotaStore struct {
	mu     sync.Mutex
	quotas map[string]*model.DailyFreeQuota
}

func newFakeFreeQuotaStore() *fakeFreeQuotaStore {
	return &fakeFreeQuotaStore{quotas: map[string]*model.DailyFreeQuota{}}
}

func (f *fakeFreeQuotaStore) GetOrCreateByFingerprint(_ context.Context, fingerprint string, usageDate string, defaultLimit int) (*model.DailyFreeQuota, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	quota, ok := f.quotas[fingerprint+"|"+usageDate]
	if ok {
		copied := *quota
		return &copied, false, nil
	}
	quota = &model.DailyFreeQuota{FingerprintHash: fingerprint, UsageDate: usageDate, DailyLimit: defaultLimit, DailyUsed: 0}
	f.quotas[fingerprint+"|"+usageDate] = quota
	copied := *quota
	return &copied, true, nil
}

func (f *fakeFreeQuotaStore) GetByFingerprint(_ context.Context, fingerprint string, usageDate string) (*model.DailyFreeQuota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	quota := *f.quotas[fingerprint+"|"+usageDate]
	return &quota, nil
}

func (f *fakeFreeQuotaStore) Consume(_ context.Context, fingerprint string, usageDate string, defaultLimit int) (*model.DailyFreeQuota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fingerprint + "|" + usageDate
	quota := f.quotas[key]
	if quota == nil {
		quota = &model.DailyFreeQuota{FingerprintHash: fingerprint, UsageDate: usageDate, DailyLimit: defaultLimit, DailyUsed: 0}
		f.quotas[key] = quota
	}
	if quota.DailyUsed >= quota.DailyLimit {
		return nil, ErrQuotaExhausted
	}
	quota.DailyUsed++
	copied := *quota
	return &copied, nil
}

type fakeUsageStore struct {
	mu        sync.Mutex
	byRequest map[string]*model.UsageEvent
	events    []*model.UsageEvent
}

func newFakeUsageStore() *fakeUsageStore {
	return &fakeUsageStore{byRequest: map[string]*model.UsageEvent{}}
}

func (f *fakeUsageStore) Create(_ context.Context, event *model.UsageEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := *event
	f.events = append(f.events, &cloned)
	if event.RequestID != nil {
		f.byRequest[*event.RequestID] = &cloned
	}
	return nil
}

func (f *fakeUsageStore) FindByRequestID(_ context.Context, requestID string) (*model.UsageEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	event := f.byRequest[requestID]
	if event == nil {
		return nil, nil
	}
	cloned := *event
	return &cloned, nil
}

type fakeIdemStore struct {
	mu      sync.Mutex
	results map[string]*ConsumeResponse
	locks   map[string]bool
}

func newFakeIdemStore() *fakeIdemStore {
	return &fakeIdemStore{results: map[string]*ConsumeResponse{}, locks: map[string]bool{}}
}

func (f *fakeIdemStore) GetConsumeResult(_ context.Context, requestID string) (*ConsumeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if result, ok := f.results[requestID]; ok {
		copied := *result
		return &copied, nil
	}
	return nil, nil
}
func (f *fakeIdemStore) SaveConsumeResult(_ context.Context, requestID string, resp *ConsumeResponse, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := *resp
	f.results[requestID] = &copied
	return nil
}
func (f *fakeIdemStore) AcquireConsumeLock(_ context.Context, key string, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locks[key] {
		return false, nil
	}
	f.locks[key] = true
	return true, nil
}
func (f *fakeIdemStore) ReleaseConsumeLock(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.locks, key)
	return nil
}

type fakeRewardManager struct {
	mu        sync.Mutex
	balances  map[uint64]int
	consumeBy []uint64
}

func newFakeRewardManager() *fakeRewardManager {
	return &fakeRewardManager{balances: map[uint64]int{}}
}

func (f *fakeRewardManager) Balance(_ context.Context, userID uint64) (*rewardsvc.Balance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &rewardsvc.Balance{UserID: userID, Remaining: f.balances[userID]}, nil
}

func (f *fakeRewardManager) Consume(_ context.Context, userID uint64) (*rewardsvc.ConsumeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.balances[userID] <= 0 {
		return nil, rewardsvc.ErrQuotaExhausted
	}
	f.balances[userID]--
	f.consumeBy = append(f.consumeBy, userID)
	return &rewardsvc.ConsumeResult{Remaining: f.balances[userID]}, nil
}

type fakeReferralActivator struct {
	mu        sync.Mutex
	activated []uint64
	err       error
}

func (f *fakeReferralActivator) ActivateReferral(_ context.Context, invitedUserID uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.activated = append(f.activated, invitedUserID)
	return &growthsvc.RewardGrantResult{Created: true}, nil
}

func TestCheckCreatesQuotaForNewMachine(t *testing.T) {
	quotas := newFakeFreeQuotaStore()
	usage := newFakeUsageStore()
	svc := NewService(&fakeAPIKeyStore{}, quotas, usage, newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)

	resp, err := svc.Check(context.Background(), CheckRequest{FingerprintHash: "fp-1", Action: "generate"})
	require.NoError(t, err)
	require.True(t, resp.Allowed)
	require.Equal(t, model.AccessModeFree, resp.AccessMode)
	require.Equal(t, 10, resp.FreeLimit)
	require.Equal(t, 10, resp.FreeRemaining)
	require.NotNil(t, resp.CommitToken)
	require.Equal(t, model.AccessModeFree, resp.CommitToken.AccessMode)
}

func TestCheckBlocksWhenFreeQuotaExhausted(t *testing.T) {
	quotas := newFakeFreeQuotaStore()
	quotas.quotas["fp-2|"+time.Now().UTC().Format("2006-01-02")] = &model.DailyFreeQuota{FingerprintHash: "fp-2", UsageDate: time.Now().UTC().Format("2006-01-02"), DailyLimit: 1, DailyUsed: 1}
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)

	resp, err := svc.Check(context.Background(), CheckRequest{FingerprintHash: "fp-2", Action: "generate"})
	require.NoError(t, err)
	require.False(t, resp.Allowed)
	require.Equal(t, model.AccessModeBlocked, resp.AccessMode)
	require.Equal(t, "free_quota_exhausted", resp.ReasonCode)
}

func TestCheckPaidKeyStatuses(t *testing.T) {
	now := time.Now().UTC()
	quota10 := 10
	quota0 := 1
	cases := []struct {
		name    string
		key     *model.APIKey
		allowed bool
		reason  string
	}{
		{name: "valid", key: &model.APIKey{ID: 1, Status: model.APIKeyStatusActive, PlanName: "pro", QuotaTotal: &quota10, QuotaUsed: 2}, allowed: true},
		{name: "disabled", key: &model.APIKey{ID: 2, Status: model.APIKeyStatusDisabled, QuotaTotal: &quota10}, reason: "disabled_api_key"},
		{name: "expired", key: &model.APIKey{ID: 3, Status: model.APIKeyStatusActive, ExpiresAt: &now, QuotaTotal: &quota10}, reason: "expired_api_key"},
		{name: "paid quota exhausted", key: &model.APIKey{ID: 4, Status: model.APIKeyStatusActive, QuotaTotal: &quota0, QuotaUsed: 1}, reason: "paid_quota_exhausted"},
		{name: "invalid", key: nil, reason: "invalid_api_key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeAPIKeyStore{key: tc.key}
			svc := NewService(store, newFakeFreeQuotaStore(), newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)
			if tc.key != nil && tc.name == "expired" {
				past := now.Add(-time.Hour)
				tc.key.ExpiresAt = &past
			}
			resp, err := svc.Check(context.Background(), CheckRequest{FingerprintHash: "fp", APIKey: "demo", Action: "status"})
			require.NoError(t, err)
			require.Equal(t, tc.allowed, resp.Allowed)
			if tc.allowed {
				require.Equal(t, model.AccessModePaid, resp.AccessMode)
				require.Equal(t, 10, resp.PaidQuotaTotal)
				require.Equal(t, 2, resp.PaidQuotaUsed)
				require.Equal(t, 8, resp.PaidQuotaRemaining)
				require.NotNil(t, resp.CommitToken)
				require.Equal(t, model.AccessModePaid, resp.CommitToken.AccessMode)
			} else {
				require.Equal(t, tc.reason, resp.ReasonCode)
			}
		})
	}
}

func TestCheckPrefersRewardBeforeFree(t *testing.T) {
	rewards := newFakeRewardManager()
	rewards.balances[88] = 4
	quotas := newFakeFreeQuotaStore()
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeUsageStore(), newFakeIdemStore(), rewards, nil, "salt", 10, time.Hour)

	resp, err := svc.Check(context.Background(), CheckRequest{
		FingerprintHash: "fp-reward",
		UserID:          88,
		Action:          "generate",
	})
	require.NoError(t, err)
	require.True(t, resp.Allowed)
	require.Equal(t, model.AccessModeReward, resp.AccessMode)
	require.Equal(t, 4, resp.RewardRemaining)
	require.NotNil(t, resp.CommitToken)
	require.Equal(t, uint64(88), resp.CommitToken.UserID)
	require.Equal(t, model.AccessModeReward, resp.CommitToken.AccessMode)
	require.Empty(t, quotas.quotas)
}

func TestConsumeRewardIsIdempotent(t *testing.T) {
	rewards := newFakeRewardManager()
	rewards.balances[77] = 2
	referrals := &fakeReferralActivator{}
	svc := NewService(&fakeAPIKeyStore{}, newFakeFreeQuotaStore(), newFakeUsageStore(), newFakeIdemStore(), rewards, referrals, "salt", 10, time.Hour)

	first, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-reward",
		UserID:          77,
		RequestID:       "req-reward",
		UsageType:       "generate",
		AccessMode:      model.AccessModeReward,
	})
	require.NoError(t, err)
	require.Equal(t, model.AccessModeReward, first.AccessMode)
	require.Equal(t, 1, first.RewardRemaining)
	require.Equal(t, 1, first.Remaining)

	second, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-reward",
		UserID:          77,
		RequestID:       "req-reward",
		UsageType:       "generate",
		AccessMode:      model.AccessModeReward,
	})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, []uint64{77}, rewards.consumeBy)
	require.Equal(t, []uint64{77}, referrals.activated)
}

func TestConsumeFreeIsIdempotentAndConcurrentSafe(t *testing.T) {
	quotas := newFakeFreeQuotaStore()
	today := time.Now().UTC().Format("2006-01-02")
	quotas.quotas["fp-3|"+today] = &model.DailyFreeQuota{FingerprintHash: "fp-3", UsageDate: today, DailyLimit: 1, DailyUsed: 0}
	usage := newFakeUsageStore()
	idem := newFakeIdemStore()
	svc := NewService(&fakeAPIKeyStore{}, quotas, usage, idem, nil, nil, "salt", 10, time.Hour)

	first, err := svc.Consume(context.Background(), ConsumeRequest{FingerprintHash: "fp-3", RequestID: "req-1", UsageType: "generate", AccessMode: model.AccessModeFree})
	require.NoError(t, err)
	require.Equal(t, 1, first.FreeUsed)
	require.Equal(t, 0, first.FreeRemaining)
	require.Equal(t, 0, first.Remaining)

	second, err := svc.Consume(context.Background(), ConsumeRequest{FingerprintHash: "fp-3", RequestID: "req-1", UsageType: "generate", AccessMode: model.AccessModeFree})
	require.NoError(t, err)
	require.Equal(t, first, second)

	quotas.quotas["fp-4|"+today] = &model.DailyFreeQuota{FingerprintHash: "fp-4", UsageDate: today, DailyLimit: 1, DailyUsed: 0}
	svc2 := NewService(&fakeAPIKeyStore{}, quotas, newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)
	var wg sync.WaitGroup
	var successCount int
	var mu sync.Mutex
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc2.Consume(context.Background(), ConsumeRequest{FingerprintHash: "fp-4", RequestID: string(rune('a' + i)), UsageType: "generate", AccessMode: model.AccessModeFree})
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, 1, successCount)
	require.Equal(t, 1, quotas.quotas["fp-4|"+today].DailyUsed)
}

func TestConsumePaidIsIdempotent(t *testing.T) {
	quotaTotal := 3
	apiStore := &fakeAPIKeyStore{key: &model.APIKey{ID: 9, Status: model.APIKeyStatusActive, PlanName: "pro", KeyPrefix: "cop_live_abcd", QuotaTotal: &quotaTotal, QuotaUsed: 1}}
	svc := NewService(apiStore, newFakeFreeQuotaStore(), newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)

	first, err := svc.Consume(context.Background(), ConsumeRequest{FingerprintHash: "fp-paid", RequestID: "req-paid", UsageType: "generate", AccessMode: model.AccessModePaid, APIKey: "demo"})
	require.NoError(t, err)
	require.Equal(t, model.AccessModePaid, first.AccessMode)
	require.Equal(t, 3, first.PaidQuotaTotal)
	require.Equal(t, 2, first.PaidQuotaUsed)
	require.Equal(t, 1, first.PaidQuotaRemaining)
	require.Equal(t, 1, first.Remaining)

	second, err := svc.Consume(context.Background(), ConsumeRequest{FingerprintHash: "fp-paid", RequestID: "req-paid", UsageType: "generate", AccessMode: model.AccessModePaid, APIKey: "demo"})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, 2, apiStore.key.QuotaUsed)
}

func TestConsumeFreeActivatesReferralWhenUserIDPresent(t *testing.T) {
	quotas := newFakeFreeQuotaStore()
	today := time.Now().UTC().Format("2006-01-02")
	quotas.quotas["fp-ref|"+today] = &model.DailyFreeQuota{FingerprintHash: "fp-ref", UsageDate: today, DailyLimit: 2, DailyUsed: 0}
	referrals := &fakeReferralActivator{}
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeUsageStore(), newFakeIdemStore(), nil, referrals, "salt", 10, time.Hour)

	resp, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-ref",
		UserID:          123,
		RequestID:       "req-ref",
		UsageType:       "generate",
		AccessMode:      model.AccessModeFree,
	})
	require.NoError(t, err)
	require.Equal(t, model.AccessModeFree, resp.AccessMode)
	require.Equal(t, []uint64{123}, referrals.activated)
}

func TestConsumeIgnoresMissingReferral(t *testing.T) {
	quotas := newFakeFreeQuotaStore()
	today := time.Now().UTC().Format("2006-01-02")
	quotas.quotas["fp-no-ref|"+today] = &model.DailyFreeQuota{FingerprintHash: "fp-no-ref", UsageDate: today, DailyLimit: 2, DailyUsed: 0}
	referrals := &fakeReferralActivator{err: growthsvc.ErrReferralNotFound}
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeUsageStore(), newFakeIdemStore(), nil, referrals, "salt", 10, time.Hour)

	resp, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-no-ref",
		UserID:          456,
		RequestID:       "req-no-ref",
		UsageType:       "generate",
		AccessMode:      model.AccessModeFree,
	})
	require.NoError(t, err)
	require.Equal(t, model.AccessModeFree, resp.AccessMode)
}

func TestAdjustQuotaAffectsCheckRemaining(t *testing.T) {
	quotas := newFakeFreeQuotaStore()
	today := time.Now().UTC().Format("2006-01-02")
	quotas.quotas["fp-5|"+today] = &model.DailyFreeQuota{FingerprintHash: "fp-5", UsageDate: today, DailyLimit: 2, DailyUsed: 1}
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)

	resp, err := svc.Check(context.Background(), CheckRequest{FingerprintHash: "fp-5", Action: "generate"})
	require.NoError(t, err)
	require.Equal(t, 1, resp.FreeRemaining)

	quotas.mu.Lock()
	quotas.quotas["fp-5|"+today].DailyLimit = 5
	quotas.mu.Unlock()

	resp, err = svc.Check(context.Background(), CheckRequest{FingerprintHash: "fp-5", Action: "generate"})
	require.NoError(t, err)
	require.Equal(t, 4, resp.FreeRemaining)
}

func TestConsumePaidRequiresAPIKey(t *testing.T) {
	quotaTotal := 3
	apiStore := &fakeAPIKeyStore{key: &model.APIKey{ID: 9, Status: model.APIKeyStatusActive, PlanName: "pro", KeyPrefix: "cop_live_abcd", QuotaTotal: &quotaTotal, QuotaUsed: 1}}
	svc := NewService(apiStore, newFakeFreeQuotaStore(), newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)

	_, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-paid",
		RequestID:       "req-missing-key",
		UsageType:       "generate",
		AccessMode:      model.AccessModePaid,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "api_key is required")
	require.Equal(t, 1, apiStore.key.QuotaUsed)
}

func TestConsumeRestoreExistingPaidUsageWithoutAPIKey(t *testing.T) {
	usage := newFakeUsageStore()
	requestID := "req-existing-paid"
	usage.byRequest[requestID] = &model.UsageEvent{
		RequestID: &requestID,
		Mode:      model.UsageModePaid,
	}
	svc := NewService(&fakeAPIKeyStore{}, newFakeFreeQuotaStore(), usage, newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)

	resp, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-paid",
		RequestID:       requestID,
		UsageType:       "generate",
		AccessMode:      model.AccessModePaid,
	})
	require.NoError(t, err)
	require.Equal(t, model.AccessModePaid, resp.AccessMode)
	require.Zero(t, resp.Remaining)
}

func TestConsumeRestoreExistingFreeUsageReturnsCurrentQuota(t *testing.T) {
	quotas := newFakeFreeQuotaStore()
	today := time.Now().UTC().Format("2006-01-02")
	quotas.quotas["fp-restore|"+today] = &model.DailyFreeQuota{FingerprintHash: "fp-restore", UsageDate: today, DailyLimit: 5, DailyUsed: 2}
	usage := newFakeUsageStore()
	requestID := "req-existing-free"
	usage.byRequest[requestID] = &model.UsageEvent{
		RequestID:       &requestID,
		FingerprintHash: "fp-restore",
		Mode:            model.UsageModeFree,
		CreatedAt:       time.Now().UTC(),
	}
	svc := NewService(&fakeAPIKeyStore{}, quotas, usage, newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)

	resp, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-restore",
		RequestID:       requestID,
		UsageType:       "generate",
		AccessMode:      model.AccessModeFree,
	})
	require.NoError(t, err)
	require.Equal(t, model.AccessModeFree, resp.AccessMode)
	require.Equal(t, 2, resp.FreeUsed)
	require.Equal(t, 3, resp.FreeRemaining)
	require.Equal(t, 3, resp.Remaining)
}

func TestFreeQuotaResetsAcrossDays(t *testing.T) {
	quotas := newFakeFreeQuotaStore()
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", 10, time.Hour)
	dayOne := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return dayOne }

	for i := 0; i < 10; i++ {
		resp, err := svc.Consume(context.Background(), ConsumeRequest{
			FingerprintHash: "fp-day-reset",
			RequestID:       "req-day-1-" + string(rune('a'+i)),
			UsageType:       "generate",
			AccessMode:      model.AccessModeFree,
		})
		require.NoError(t, err)
		require.Equal(t, 9-i, resp.FreeRemaining)
	}

	checkResp, err := svc.Check(context.Background(), CheckRequest{FingerprintHash: "fp-day-reset", Action: "generate"})
	require.NoError(t, err)
	require.False(t, checkResp.Allowed)
	require.Equal(t, "free_quota_exhausted", checkResp.ReasonCode)

	svc.clock = func() time.Time { return dayOne.Add(24 * time.Hour) }
	nextDay, err := svc.Check(context.Background(), CheckRequest{FingerprintHash: "fp-day-reset", Action: "generate"})
	require.NoError(t, err)
	require.True(t, nextDay.Allowed)
	require.Equal(t, 10, nextDay.FreeRemaining)
}
