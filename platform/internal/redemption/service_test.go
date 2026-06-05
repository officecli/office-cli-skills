package redemption

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
	"github.com/officecli/officecli/platform/internal/store/sqlstore"
)

// fakeStore implements Store. It is deliberately simple: maps by ID and by
// code, plus an in-memory list of redemptions.
type fakeStore struct {
	redeemMu         sync.Mutex
	nextCodeID       uint64
	nextRedemptionID uint64
	codes            map[uint64]*model.RedemptionCode
	codeByCode       map[string]uint64
	redemptions      []model.RedemptionCodeRedemption
	balances         map[uint64]int
	ledgerSeq        uint64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		codes:      map[uint64]*model.RedemptionCode{},
		codeByCode: map[string]uint64{},
		balances:   map[uint64]int{},
	}
}

func (f *fakeStore) CreateRedemptionCode(_ context.Context, c *model.RedemptionCode) error {
	if _, exists := f.codeByCode[c.Code]; exists {
		return fmt.Errorf("redemption code already exists")
	}
	f.nextCodeID++
	c.ID = f.nextCodeID
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	copied := *c
	f.codes[c.ID] = &copied
	f.codeByCode[c.Code] = c.ID
	return nil
}

func (f *fakeStore) GetRedemptionCodeByID(_ context.Context, id uint64) (*model.RedemptionCode, error) {
	if c, ok := f.codes[id]; ok {
		copied := *c
		return &copied, nil
	}
	return nil, sqlstore.ErrRedemptionCodeNotFound
}

func (f *fakeStore) GetRedemptionCodeByCode(_ context.Context, code string) (*model.RedemptionCode, error) {
	id, ok := f.codeByCode[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return nil, sqlstore.ErrRedemptionCodeNotFound
	}
	copied := *f.codes[id]
	return &copied, nil
}

func (f *fakeStore) ListRedemptionCodes(_ context.Context, filter sqlstore.RedemptionCodeFilter) ([]model.RedemptionCode, int64, error) {
	var out []model.RedemptionCode
	for _, c := range f.codes {
		if filter.Status != "" && c.Status != filter.Status {
			continue
		}
		if filter.CodeContains != "" && !strings.Contains(strings.ToUpper(c.Code), strings.ToUpper(filter.CodeContains)) {
			continue
		}
		out = append(out, *c)
	}
	return out, int64(len(out)), nil
}

func (f *fakeStore) UpdateRedemptionCode(_ context.Context, id uint64, u sqlstore.RedemptionCodeUpdates) (*model.RedemptionCode, error) {
	c, ok := f.codes[id]
	if !ok {
		return nil, sqlstore.ErrRedemptionCodeNotFound
	}
	if u.CreditAmount != nil {
		c.CreditAmount = *u.CreditAmount
	}
	if u.ClearMaxLimit {
		c.MaxRedemptions = nil
	} else if u.MaxRedemptions != nil {
		v := *u.MaxRedemptions
		c.MaxRedemptions = &v
	}
	if u.PerUserLimit != nil {
		c.PerUserLimit = *u.PerUserLimit
	}
	if u.ClearExpiresAt {
		c.ExpiresAt = nil
	} else if u.ExpiresAt != nil {
		t := *u.ExpiresAt
		c.ExpiresAt = &t
	}
	if u.Notes != nil {
		c.Notes = strings.TrimSpace(*u.Notes)
	}
	c.UpdatedAt = time.Now()
	out := *c
	return &out, nil
}

func (f *fakeStore) SetRedemptionCodeStatus(_ context.Context, id uint64, status model.RedemptionCodeStatus) (*model.RedemptionCode, error) {
	c, ok := f.codes[id]
	if !ok {
		return nil, sqlstore.ErrRedemptionCodeNotFound
	}
	c.Status = status
	c.UpdatedAt = time.Now()
	out := *c
	return &out, nil
}

func (f *fakeStore) DeleteRedemptionCode(_ context.Context, id uint64) error {
	c, ok := f.codes[id]
	if !ok {
		return sqlstore.ErrRedemptionCodeNotFound
	}
	delete(f.codeByCode, c.Code)
	delete(f.codes, id)
	return nil
}

func (f *fakeStore) ListRedemptionRecords(_ context.Context, filter sqlstore.RedemptionRecordFilter) ([]model.RedemptionCodeRedemption, int64, error) {
	var out []model.RedemptionCodeRedemption
	for _, r := range f.redemptions {
		if filter.RedemptionCodeID != 0 && r.RedemptionCodeID != filter.RedemptionCodeID {
			continue
		}
		if filter.Code != "" && r.Code != strings.ToUpper(filter.Code) {
			continue
		}
		if filter.UserID != 0 && r.UserID != filter.UserID {
			continue
		}
		if filter.Source != "" && r.Source != filter.Source {
			continue
		}
		out = append(out, r)
	}
	return out, int64(len(out)), nil
}

func (f *fakeStore) RedeemCode(_ context.Context, rc sqlstore.RedeemContext) (*sqlstore.RedeemResult, error) {
	f.redeemMu.Lock()
	defer f.redeemMu.Unlock()
	id, ok := f.codeByCode[strings.ToUpper(strings.TrimSpace(rc.Code))]
	if !ok {
		return nil, sqlstore.ErrRedemptionCodeNotFound
	}
	c := f.codes[id]
	now := rc.Now
	if now.IsZero() {
		now = time.Now()
	}
	if !c.IsEnabled() {
		return nil, sqlstore.ErrRedemptionCodeDisabled
	}
	if c.IsExpired(now) {
		return nil, sqlstore.ErrRedemptionCodeExpired
	}
	if c.IsExhausted() {
		return nil, sqlstore.ErrRedemptionCodeExhausted
	}
	userClaimed := 0
	for _, r := range f.redemptions {
		if r.RedemptionCodeID == c.ID && r.UserID == rc.UserID {
			userClaimed++
		}
	}
	if userClaimed >= c.PerUserLimit {
		return nil, sqlstore.ErrRedemptionCodeAlreadyClaimed
	}
	f.balances[rc.UserID] += c.CreditAmount
	c.RedemptionsUsed++
	f.ledgerSeq++
	f.nextRedemptionID++
	idempKey := fmt.Sprintf("redemption:%d:%d:%d", c.ID, rc.UserID, userClaimed)
	redemption := model.RedemptionCodeRedemption{
		ID:                  f.nextRedemptionID,
		RedemptionCodeID:    c.ID,
		Code:                c.Code,
		UserID:              rc.UserID,
		CreditAmount:        c.CreditAmount,
		LedgerEntryID:       f.ledgerSeq,
		Source:              rc.Source,
		ClientIP:            rc.ClientIP,
		UserAgent:           rc.UserAgent,
		IdempotencyKey:      idempKey,
		PerUserLimitAtClaim: c.PerUserLimit,
		RedeemedAt:          now,
	}
	f.redemptions = append(f.redemptions, redemption)
	account := &model.UserHostedCreditAccount{
		UserID:        rc.UserID,
		CreditBalance: f.balances[rc.UserID],
	}
	ledger := &model.UserHostedCreditLedger{
		ID:          f.ledgerSeq,
		UserID:      rc.UserID,
		CreditDelta: c.CreditAmount,
	}
	return &sqlstore.RedeemResult{
		Code:         c,
		Redemption:   &redemption,
		LedgerEntry:  ledger,
		Account:      account,
		CreditAmount: c.CreditAmount,
	}, nil
}

// ---- Tests ----

func ptrInt(v int) *int { return &v }

func newServiceWithCode(t *testing.T, code string, modify func(*model.RedemptionCode)) (*Service, *fakeStore, *model.RedemptionCode) {
	t.Helper()
	store := newFakeStore()
	svc := NewService(store)
	created, err := svc.CreateCode(context.Background(), CreateCodeRequest{
		Code:         code,
		CreditAmount: 100,
		PerUserLimit: 1,
	})
	require.NoError(t, err)
	require.Equal(t, model.RedemptionCodeStatusDisabled, created.Status, "new codes must default to disabled")
	if modify != nil {
		modify(store.codes[created.ID])
	}
	return svc, store, store.codes[created.ID]
}

func TestCreateCodeDefaultsToDisabled(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	code, err := svc.CreateCode(context.Background(), CreateCodeRequest{
		Code:         "welcome10",
		CreditAmount: 10,
	})
	require.NoError(t, err)
	require.Equal(t, model.RedemptionCodeStatusDisabled, code.Status)
	require.Equal(t, "WELCOME10", code.Code, "code should normalise to uppercase")
	require.Equal(t, 1, code.PerUserLimit, "per_user_limit defaults to 1")
}

func TestCreateCodeGeneratesWhenBlank(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	c, err := svc.CreateCode(context.Background(), CreateCodeRequest{
		CreditAmount: 5,
	})
	require.NoError(t, err)
	require.Len(t, c.Code, defaultGeneratedCodeLength)
}

func TestCreateCodeValidatesAmount(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	_, err := svc.CreateCode(context.Background(), CreateCodeRequest{Code: "BAD", CreditAmount: 0})
	require.ErrorIs(t, err, ErrCreditAmountRequired)
}

func TestCreateCodeRejectsInvalidShape(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	_, err := svc.CreateCode(context.Background(), CreateCodeRequest{Code: "ab", CreditAmount: 5})
	require.Error(t, err)
	_, err = svc.CreateCode(context.Background(), CreateCodeRequest{Code: "bad spaces", CreditAmount: 5})
	require.Error(t, err)
}

func TestRedeemSuccess(t *testing.T) {
	svc, store, c := newServiceWithCode(t, "GIFT1000", nil)
	_, err := svc.EnableCode(context.Background(), c.ID)
	require.NoError(t, err)

	resp, err := svc.Redeem(context.Background(), RedeemRequest{
		UserID: 42,
		Code:   "gift1000",
		Source: string(model.RedemptionSourceApp),
	})
	require.NoError(t, err)
	require.Equal(t, "GIFT1000", resp.Code)
	require.Equal(t, 100, resp.CreditAmount)
	require.Equal(t, 100, resp.NewBalance)
	require.Equal(t, 100, store.balances[42])
	require.Len(t, store.redemptions, 1)
}

func TestRedeemRejectsDisabled(t *testing.T) {
	svc, _, _ := newServiceWithCode(t, "DISABLED1", nil)
	_, err := svc.Redeem(context.Background(), RedeemRequest{
		UserID: 1, Code: "DISABLED1", Source: "app",
	})
	require.ErrorIs(t, err, ErrCodeDisabled)
}

func TestRedeemRejectsExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	svc, _, c := newServiceWithCode(t, "EXPIRED1", func(c *model.RedemptionCode) {
		c.ExpiresAt = &past
	})
	_, err := svc.EnableCode(context.Background(), c.ID)
	require.NoError(t, err)
	_, err = svc.Redeem(context.Background(), RedeemRequest{
		UserID: 1, Code: "EXPIRED1", Source: "app",
	})
	require.ErrorIs(t, err, ErrCodeExpired)
}

func TestRedeemRejectsExhausted(t *testing.T) {
	limit := 1
	svc, _, c := newServiceWithCode(t, "EXHAUST1", func(c *model.RedemptionCode) {
		c.MaxRedemptions = &limit
	})
	_, err := svc.EnableCode(context.Background(), c.ID)
	require.NoError(t, err)
	// First user claims it -- fine.
	_, err = svc.Redeem(context.Background(), RedeemRequest{UserID: 1, Code: "EXHAUST1", Source: "app"})
	require.NoError(t, err)
	// Second user is blocked because max_redemptions reached.
	_, err = svc.Redeem(context.Background(), RedeemRequest{UserID: 2, Code: "EXHAUST1", Source: "app"})
	require.ErrorIs(t, err, ErrCodeExhausted)
}

func TestRedeemRejectsPerUserLimit(t *testing.T) {
	svc, _, c := newServiceWithCode(t, "ONCE0001", func(c *model.RedemptionCode) {
		c.PerUserLimit = 1
	})
	_, err := svc.EnableCode(context.Background(), c.ID)
	require.NoError(t, err)
	_, err = svc.Redeem(context.Background(), RedeemRequest{UserID: 1, Code: "ONCE0001", Source: "app"})
	require.NoError(t, err)
	_, err = svc.Redeem(context.Background(), RedeemRequest{UserID: 1, Code: "ONCE0001", Source: "app"})
	require.ErrorIs(t, err, ErrCodeAlreadyClaimed)
}

func TestRedeemUnknownCode(t *testing.T) {
	svc := NewService(newFakeStore())
	_, err := svc.Redeem(context.Background(), RedeemRequest{UserID: 1, Code: "NOPE1234", Source: "app"})
	require.ErrorIs(t, err, ErrCodeNotFound)
}

func TestRedeemLongTermNoExpiryUnlimited(t *testing.T) {
	svc, _, c := newServiceWithCode(t, "FOREVER1", func(c *model.RedemptionCode) {
		c.MaxRedemptions = nil
		c.PerUserLimit = 10
		c.ExpiresAt = nil
	})
	_, err := svc.EnableCode(context.Background(), c.ID)
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		_, err := svc.Redeem(context.Background(), RedeemRequest{UserID: 1, Code: "FOREVER1", Source: "app"})
		require.NoError(t, err)
	}
}

func TestUpdateCodeClearsExpiry(t *testing.T) {
	t1 := time.Now().Add(time.Hour)
	svc, _, c := newServiceWithCode(t, "TIME0001", func(c *model.RedemptionCode) {
		c.ExpiresAt = &t1
	})
	updated, err := svc.UpdateCode(context.Background(), c.ID, UpdateCodeRequest{ClearExpiresAt: true})
	require.NoError(t, err)
	require.Nil(t, updated.ExpiresAt)
}

func TestEnableDisableToggle(t *testing.T) {
	svc, _, c := newServiceWithCode(t, "TOGGLE01", nil)
	enabled, err := svc.EnableCode(context.Background(), c.ID)
	require.NoError(t, err)
	require.Equal(t, model.RedemptionCodeStatusEnabled, enabled.Status)
	disabled, err := svc.DisableCode(context.Background(), c.ID)
	require.NoError(t, err)
	require.Equal(t, model.RedemptionCodeStatusDisabled, disabled.Status)
}

func TestListCodesFilters(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	for _, code := range []string{"PROMO001", "PROMO002", "GIFT0001"} {
		_, err := svc.CreateCode(context.Background(), CreateCodeRequest{Code: code, CreditAmount: 10})
		require.NoError(t, err)
	}
	codes, total, err := svc.ListCodes(context.Background(), ListCodesRequest{CodeContains: "PROMO"})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, codes, 2)
}

// TestConcurrentRedeemAtMostOncePerUser fires N goroutines attempting to
// redeem the same code as the same user. Because per_user_limit=1, only one
// goroutine must succeed; the others must each receive ErrCodeAlreadyClaimed
// (in this in-memory model -- in the real DB the SELECT FOR UPDATE plus
// the UNIQUE(idempotency_key) index serialise this).
func TestConcurrentRedeemAtMostOncePerUser(t *testing.T) {
	svc, store, c := newServiceWithCode(t, "ATONCE01", nil)
	_, err := svc.EnableCode(context.Background(), c.ID)
	require.NoError(t, err)

	const N = 16
	var wg sync.WaitGroup
	var successes int32
	var alreadyClaimed int32
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.Redeem(context.Background(), RedeemRequest{
				UserID: 7,
				Code:   "ATONCE01",
				Source: "app",
			})
			if err == nil {
				atomic.AddInt32(&successes, 1)
				return
			}
			if err == ErrCodeAlreadyClaimed {
				atomic.AddInt32(&alreadyClaimed, 1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, successes, "exactly one redemption must succeed")
	require.EqualValues(t, N-1, alreadyClaimed, "remaining attempts should report already_claimed")
	require.Equal(t, 100, store.balances[7])
	require.Len(t, store.redemptions, 1)
}

// TestPerUserLimitOneEnforcedByIndex sanity-checks the per_user_limit_at_claim
// denormalization: every redemption row produced for a per_user_limit=1 code
// must carry per_user_limit_at_claim=1, so the partial UNIQUE index added in
// migration 027 can rely on it.
func TestPerUserLimitOneEnforcedByIndex(t *testing.T) {
	svc, store, c := newServiceWithCode(t, "SINGLE01", func(c *model.RedemptionCode) {
		c.PerUserLimit = 1
	})
	_, err := svc.EnableCode(context.Background(), c.ID)
	require.NoError(t, err)

	_, err = svc.Redeem(context.Background(), RedeemRequest{UserID: 11, Code: "SINGLE01", Source: "app"})
	require.NoError(t, err)
	require.Len(t, store.redemptions, 1)
	require.Equal(t, 1, store.redemptions[0].PerUserLimitAtClaim,
		"per_user_limit_at_claim must be persisted so the partial UNIQUE index can guard singleton codes")

	// A second attempt by the same user must be rejected before the row is
	// written -- this is what the application-level check guarantees; the
	// migration 027 partial UNIQUE index is the belt-and-braces defense.
	_, err = svc.Redeem(context.Background(), RedeemRequest{UserID: 11, Code: "SINGLE01", Source: "app"})
	require.ErrorIs(t, err, ErrCodeAlreadyClaimed)
	require.Len(t, store.redemptions, 1, "no second row should be created")
}
