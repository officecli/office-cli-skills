package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	growthsvc "github.com/officecli/officecli-internal/platform/internal/growth"
	"github.com/officecli/officecli-internal/platform/internal/model"
	rewardsvc "github.com/officecli/officecli-internal/platform/internal/reward"
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
	if f.key.Status != model.APIKeyStatusActive {
		return nil, errors.New("api key is disabled")
	}
	if f.key.QuotaTotal != nil && f.key.PaidQuotaRemaining() <= 0 {
		return nil, ErrPaidQuotaExhausted
	}
	f.key.QuotaUsed++
	copied := *f.key
	return &copied, nil
}

type fakeFingerprintCreditStore struct {
	mu       sync.Mutex
	accounts map[string]*model.FingerprintCreditAccount
}

func newFakeFingerprintCreditStore() *fakeFingerprintCreditStore {
	return &fakeFingerprintCreditStore{accounts: map[string]*model.FingerprintCreditAccount{}}
}

func (f *fakeFingerprintCreditStore) GetHostedCreditAccountByFingerprint(_ context.Context, fingerprint string) (*model.FingerprintCreditAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	account, ok := f.accounts[fingerprint]
	if !ok {
		return &model.FingerprintCreditAccount{FingerprintHash: fingerprint}, nil
	}
	copied := *account
	return &copied, nil
}

func (f *fakeFingerprintCreditStore) ensureSignupCredits(fingerprint string, amount int) *model.FingerprintCreditAccount {
	f.mu.Lock()
	defer f.mu.Unlock()
	if account, ok := f.accounts[fingerprint]; ok {
		copied := *account
		return &copied
	}
	account := &model.FingerprintCreditAccount{FingerprintHash: fingerprint, CreditBalance: amount, BonusGranted: true}
	f.accounts[fingerprint] = account
	copied := *account
	return &copied
}

type fakeAnonymousGranter struct {
	store  *fakeFingerprintCreditStore
	amount int
}

func newFakeAnonymousGranter(store *fakeFingerprintCreditStore, amount int) *fakeAnonymousGranter {
	return &fakeAnonymousGranter{store: store, amount: amount}
}

func (g *fakeAnonymousGranter) EnsureAnonymousSignupCredits(_ context.Context, fingerprint string) (*model.FingerprintCreditAccount, error) {
	return g.store.ensureSignupCredits(fingerprint, g.amount), nil
}

func dummyFingerprintCreditStore() *fakeFingerprintCreditStore {
	return newFakeFingerprintCreditStore()
}

func dummyAnonymousGranter() *fakeAnonymousGranter {
	return newFakeAnonymousGranter(newFakeFingerprintCreditStore(), 100)
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
	amounts   []int
	err       error
}

func (f *fakeReferralActivator) ActivateReferral(_ context.Context, invitedUserID uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.activated = append(f.activated, invitedUserID)
	f.amounts = append(f.amounts, rewardAmount)
	return &growthsvc.RewardGrantResult{Created: true}, nil
}

func testCheckRequest(fingerprint, action string) CheckRequest {
	return CheckRequest{
		FingerprintHash: fingerprint,
		RequestNonce:    "nonce-" + fingerprint + "-" + action,
		Action:          action,
	}
}

func issueTestCommitToken(t *testing.T, svc *Service, req CheckRequest, accessMode model.AccessMode, apiKeyHint string) *CommitToken {
	t.Helper()
	token, err := svc.issueCommitToken(req, accessMode, apiKeyHint)
	require.NoError(t, err)
	return token
}

func resignTestCommitToken(t *testing.T, svc *Service, token *CommitToken) {
	t.Helper()
	token.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(svc.proof.privateKey, []byte(commitTokenPayload(*token))))
}

// TestAnonymousFingerprintGetsStarterCredits verifies that an anonymous Check
// auto-grants the per-device starter balance and returns hosted access.
func TestAnonymousFingerprintGetsStarterCredits(t *testing.T) {
	quotas := newFakeFingerprintCreditStore()
	usage := newFakeUsageStore()
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeAnonymousGranter(quotas, 100), usage, newFakeIdemStore(), nil, nil, "salt", time.Hour)

	resp, err := svc.Check(context.Background(), testCheckRequest("fp-1", "generate"))
	require.NoError(t, err)
	require.True(t, resp.Allowed)
	require.Equal(t, model.AccessModeHosted, resp.AccessMode)
	require.Equal(t, 100, resp.CreditBalance)
	require.NotNil(t, resp.QuotaSnapshot)
	require.Equal(t, "fingerprint", resp.QuotaSnapshot.CreditAccount.OwnerKind)
	require.Equal(t, 100, resp.QuotaSnapshot.CreditAccount.Available)
	require.NotNil(t, resp.CommitToken)
	require.Equal(t, model.AccessModeHosted, resp.CommitToken.AccessMode)
}

// TestAnonymousZeroBalanceBlocks verifies that an exhausted fingerprint account
// returns blocked access with a credit-exhausted reason code.
func TestAnonymousZeroBalanceBlocks(t *testing.T) {
	quotas := newFakeFingerprintCreditStore()
	quotas.accounts["fp-empty"] = &model.FingerprintCreditAccount{FingerprintHash: "fp-empty", CreditBalance: 0, BonusGranted: true}
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeAnonymousGranter(quotas, 100), newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", time.Hour)

	resp, err := svc.Check(context.Background(), testCheckRequest("fp-empty", "generate"))
	require.NoError(t, err)
	require.False(t, resp.Allowed)
	require.Equal(t, model.AccessModeBlocked, resp.AccessMode)
	require.Equal(t, "hosted_credit_exhausted", resp.ReasonCode)
}

// TestCheckRespectsExternalRuntimeMode keeps the long-standing behavior that an
// explicit external runtime mode does not consume credits.
func TestCheckRespectsExternalRuntimeMode(t *testing.T) {
	quotas := newFakeFingerprintCreditStore()
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeAnonymousGranter(quotas, 100), newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", time.Hour)

	req := testCheckRequest("fp-external", "generate")
	req.RuntimeMode = "external"
	resp, err := svc.Check(context.Background(), req)
	require.NoError(t, err)
	require.True(t, resp.Allowed)
	require.Equal(t, model.AccessModeFree, resp.AccessMode)
	require.NotNil(t, resp.CommitToken)
}

func TestCheckPaidAPIKey(t *testing.T) {
	quota10 := 10
	quota0 := 0
	now := time.Now().UTC().Add(time.Hour)

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
			svc := NewService(store, dummyFingerprintCreditStore(), dummyAnonymousGranter(), newFakeUsageStore(), newFakeIdemStore(), nil, nil, "salt", time.Hour)
			if tc.key != nil && tc.name == "expired" {
				past := now.Add(-time.Hour)
				tc.key.ExpiresAt = &past
			}
			req := testCheckRequest("fp", "status")
			req.APIKey = "demo"
			resp, err := svc.Check(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, tc.allowed, resp.Allowed)
			if tc.allowed {
				require.Equal(t, model.AccessModePaid, resp.AccessMode)
				require.Equal(t, 10, resp.PaidQuotaTotal)
				require.Equal(t, 2, resp.PaidQuotaUsed)
				require.Equal(t, 8, resp.PaidQuotaRemaining)
				require.NotNil(t, resp.QuotaSnapshot)
				require.Equal(t, 8, resp.QuotaSnapshot.PaidExternalQuota.CurrentKeyRemaining)
				require.NotNil(t, resp.CommitToken)
				require.Equal(t, model.AccessModePaid, resp.CommitToken.AccessMode)
			} else {
				require.Equal(t, tc.reason, resp.ReasonCode)
			}
		})
	}
}

func TestCheckPrefersRewardForLoggedInUser(t *testing.T) {
	rewards := newFakeRewardManager()
	rewards.balances[88] = 4
	quotas := newFakeFingerprintCreditStore()
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeAnonymousGranter(quotas, 100), newFakeUsageStore(), newFakeIdemStore(), rewards, nil, "salt", time.Hour)

	req := testCheckRequest("fp-reward", "generate")
	req.UserID = 88
	resp, err := svc.Check(context.Background(), req)
	require.NoError(t, err)
	require.True(t, resp.Allowed)
	require.Equal(t, model.AccessModeReward, resp.AccessMode)
	require.Equal(t, 4, resp.RewardRemaining)
	require.NotNil(t, resp.QuotaSnapshot)
	require.Equal(t, 4, resp.QuotaSnapshot.RewardQuota.Remaining)
	require.NotNil(t, resp.CommitToken)
	require.Equal(t, uint64(88), resp.CommitToken.UserID)
	require.Equal(t, model.AccessModeReward, resp.CommitToken.AccessMode)
}

func TestConsumeRewardIsIdempotent(t *testing.T) {
	rewards := newFakeRewardManager()
	rewards.balances[77] = 2
	referrals := &fakeReferralActivator{}
	svc := NewService(&fakeAPIKeyStore{}, dummyFingerprintCreditStore(), dummyAnonymousGranter(), newFakeUsageStore(), newFakeIdemStore(), rewards, referrals, "salt", time.Hour)
	checkReq := testCheckRequest("fp-reward", "generate")
	checkReq.UserID = 77
	token := issueTestCommitToken(t, svc, checkReq, model.AccessModeReward, "")
	token.RequestID = "req-reward"
	resignTestCommitToken(t, svc, token)

	first, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-reward",
		UserID:          77,
		RequestID:       "req-reward",
		UsageType:       "generate",
		AccessMode:      model.AccessModeReward,
		CommitToken:     token,
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
		CommitToken:     token,
	})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, []uint64{77}, rewards.consumeBy)
	require.Equal(t, []uint64{77}, referrals.activated)
}

// TestConsumeAnonymousRecordsUsage verifies that anonymous consume records a
// usage event (for telemetry / referral activation) without touching the
// fingerprint credit balance — settlement happens via the hostedllm path.
func TestConsumeAnonymousRecordsUsage(t *testing.T) {
	quotas := newFakeFingerprintCreditStore()
	quotas.accounts["fp-anon"] = &model.FingerprintCreditAccount{FingerprintHash: "fp-anon", CreditBalance: 100, BonusGranted: true}
	usage := newFakeUsageStore()
	svc := NewService(&fakeAPIKeyStore{}, quotas, newFakeAnonymousGranter(quotas, 100), usage, newFakeIdemStore(), nil, nil, "salt", time.Hour)

	checkReq := testCheckRequest("fp-anon", "generate")
	token := issueTestCommitToken(t, svc, checkReq, model.AccessModeFree, "")
	token.RequestID = "req-anon"
	resignTestCommitToken(t, svc, token)

	resp, err := svc.Consume(context.Background(), ConsumeRequest{
		FingerprintHash: "fp-anon",
		RequestID:       "req-anon",
		UsageType:       "generate",
		AccessMode:      model.AccessModeFree,
		CommitToken:     token,
	})
	require.NoError(t, err)
	require.Equal(t, model.AccessModeFree, resp.AccessMode)
	require.Len(t, usage.events, 1)
	require.Equal(t, model.UsageModeFree, usage.events[0].Mode)
	// Balance unchanged — hostedllm settles via its own reserve/settle path.
	require.Equal(t, 100, quotas.accounts["fp-anon"].CreditBalance)
}
