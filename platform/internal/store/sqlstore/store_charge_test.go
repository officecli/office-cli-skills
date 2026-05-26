package sqlstore

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/model"
)

func newChargeTestStore(t *testing.T, dsn string) (*Store, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.APIKey{},
		&model.UserHostedCreditAccount{},
		&model.UserHostedCreditLedger{},
		&model.FingerprintCreditAccount{},
		&model.FingerprintCreditLedger{},
	))
	return NewWithDB(db), db
}

func TestChargeHostedCreditsByUserDeductsAndIsIdempotent(t *testing.T) {
	store, db := newChargeTestStore(t, "file:charge_user_basic?mode=memory&cache=shared")

	_, _, _, err := store.GrantHostedCreditsToUser(context.Background(), 11, model.HostedCreditLedgerSourcePurchase, "order:1", 100, "purchase", "{}")
	require.NoError(t, err)

	usageEventID := uint64(42)
	account, err := store.ChargeHostedCreditsByUser(context.Background(), 11, "req-charge-1", 30, &usageEventID)
	require.NoError(t, err)
	require.Equal(t, 70, account.CreditBalance)

	// Second call with same requestID must be a no-op (idempotent).
	account2, err := store.ChargeHostedCreditsByUser(context.Background(), 11, "req-charge-1", 30, &usageEventID)
	require.NoError(t, err)
	require.Equal(t, 70, account2.CreditBalance)

	var ledgers []model.UserHostedCreditLedger
	require.NoError(t, db.Where("source_type = ?", model.HostedCreditLedgerSourceCharge).Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
	require.Equal(t, -30, ledgers[0].CreditDelta)
	require.NotNil(t, ledgers[0].UsageEventID)
	require.Equal(t, uint64(42), *ledgers[0].UsageEventID)
	require.Equal(t, "charge:req-charge-1", ledgers[0].IdempotencyKey)
}

func TestChargeHostedCreditsByUserClampsBalanceAtZero(t *testing.T) {
	store, db := newChargeTestStore(t, "file:charge_user_clamp?mode=memory&cache=shared")

	_, _, _, err := store.GrantHostedCreditsToUser(context.Background(), 12, model.HostedCreditLedgerSourcePurchase, "order:1", 3, "purchase", "{}")
	require.NoError(t, err)

	account, err := store.ChargeHostedCreditsByUser(context.Background(), 12, "req-overdraft", 5, nil)
	require.NoError(t, err)
	require.Equal(t, 0, account.CreditBalance, "balance must clamp at zero")

	var ledger model.UserHostedCreditLedger
	require.NoError(t, db.Where("source_type = ?", model.HostedCreditLedgerSourceCharge).First(&ledger).Error)
	// Plan §5 Phase 1: credit_delta = -credits, balance clamped to 0.
	require.Equal(t, -5, ledger.CreditDelta)
}

func TestChargeHostedCreditsByUserConcurrentSameRequestIDProducesSingleLedger(t *testing.T) {
	// SQLite has table-level locking which prevents true parallel write
	// transactions; we serialize requests through a single goroutine pool
	// of size 1 to verify idempotency contract. The production target is
	// Postgres where row-level `SELECT ... FOR UPDATE` enforces concurrent
	// admission; AC-1.2's 100-way concurrency is validated on Postgres in
	// integration. For this unit test, sequential 100 calls with the same
	// requestID must still produce exactly one charge ledger row.
	store, db := newChargeTestStore(t, "file:charge_user_concurrent?mode=memory&cache=shared")

	_, _, _, err := store.GrantHostedCreditsToUser(context.Background(), 13, model.HostedCreditLedgerSourcePurchase, "order:1", 10_000, "purchase", "{}")
	require.NoError(t, err)

	const n = 100
	for i := 0; i < n; i++ {
		_, err := store.ChargeHostedCreditsByUser(context.Background(), 13, "req-concurrent", 7, nil)
		require.NoError(t, err)
	}

	var count int64
	require.NoError(t, db.Model(&model.UserHostedCreditLedger{}).Where("idempotency_key = ?", "charge:req-concurrent").Count(&count).Error)
	require.Equal(t, int64(1), count, "exactly one charge ledger row per requestID")

	var account model.UserHostedCreditAccount
	require.NoError(t, db.Where("user_id = ?", 13).First(&account).Error)
	require.Equal(t, 9_993, account.CreditBalance, "balance reflects exactly one charge of 7")
}

func TestChargeHostedCreditsByUserParallelSameRequestIDIsSafe(t *testing.T) {
	// Limited parallelism check on SQLite: race a small fan-out via the same
	// in-memory DB. SQLite serializes writes so we cap at 4 to avoid the
	// table-lock false-positive while still exercising the duplicate-key
	// recovery branch.
	store, db := newChargeTestStore(t, "file:charge_user_parallel?mode=memory&cache=shared")
	_, _, _, err := store.GrantHostedCreditsToUser(context.Background(), 16, model.HostedCreditLedgerSourcePurchase, "order:1", 1000, "purchase", "{}")
	require.NoError(t, err)

	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = store.ChargeHostedCreditsByUser(context.Background(), 16, "req-parallel", 9, nil)
		}()
	}
	wg.Wait()

	var count int64
	require.NoError(t, db.Model(&model.UserHostedCreditLedger{}).Where("idempotency_key = ?", "charge:req-parallel").Count(&count).Error)
	require.LessOrEqual(t, count, int64(1), "no more than one charge ledger row even under racing writers")
}

func TestChargeAndChargeFailedAreMutuallyExclusivePerRequestID(t *testing.T) {
	store, db := newChargeTestStore(t, "file:charge_user_mutex?mode=memory&cache=shared")

	_, _, _, err := store.GrantHostedCreditsToUser(context.Background(), 14, model.HostedCreditLedgerSourcePurchase, "order:1", 100, "purchase", "{}")
	require.NoError(t, err)

	// Successful charge first.
	_, err = store.ChargeHostedCreditsByUser(context.Background(), 14, "req-mutex", 10, nil)
	require.NoError(t, err)

	// Now write charge_failed with the same requestID — different idempotency
	// namespace ("charge:" vs "charge_failed:") so the UNIQUE constraint does
	// not block it. Mutual exclusion is enforced at the service layer; the
	// store guarantees the rows have distinct keys.
	meta := `{"upstream_request_id":"u-1","upstream_http_status":200,"decode_error":"json: invalid","pricing_rule_id":1,"reason":"decode_failure"}`
	err = store.WriteChargeFailedLedgerForUser(context.Background(), 14, "req-mutex", 10, meta)
	require.NoError(t, err)

	var rows []model.UserHostedCreditLedger
	require.NoError(t, db.Where("user_id = ? AND (source_type = ? OR source_type = ?)", 14, model.HostedCreditLedgerSourceCharge, model.HostedCreditLedgerSourceChargeFailedPostUpstream).Find(&rows).Error)
	// Both rows present with distinct idempotency keys.
	require.Len(t, rows, 2)
	keys := map[string]bool{}
	for _, r := range rows {
		keys[r.IdempotencyKey] = true
	}
	require.True(t, keys["charge:req-mutex"], "charge ledger present")
	require.True(t, keys["charge_failed:req-mutex"], "charge_failed ledger present")

	// charge_failed must not modify balance.
	var account model.UserHostedCreditAccount
	require.NoError(t, db.Where("user_id = ?", 14).First(&account).Error)
	require.Equal(t, 90, account.CreditBalance)
}

func TestWriteChargeFailedLedgerForUserIsIdempotent(t *testing.T) {
	store, db := newChargeTestStore(t, "file:charge_failed_user_idem?mode=memory&cache=shared")
	_, _, _, err := store.GrantHostedCreditsToUser(context.Background(), 15, model.HostedCreditLedgerSourcePurchase, "order:1", 50, "purchase", "{}")
	require.NoError(t, err)

	meta := `{"upstream_request_id":"u-2","upstream_http_status":200,"decode_error":"timeout","pricing_rule_id":2,"reason":"client_timeout"}`
	require.NoError(t, store.WriteChargeFailedLedgerForUser(context.Background(), 15, "req-fail", 3, meta))
	require.NoError(t, store.WriteChargeFailedLedgerForUser(context.Background(), 15, "req-fail", 3, meta))

	var count int64
	require.NoError(t, db.Model(&model.UserHostedCreditLedger{}).Where("idempotency_key = ?", "charge_failed:req-fail").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestChargeHostedCreditsByFingerprintBasicFlow(t *testing.T) {
	store, db := newChargeTestStore(t, "file:charge_fp_basic?mode=memory&cache=shared")

	_, _, _, err := store.GrantHostedCreditsToFingerprint(context.Background(), "fp-1", model.HostedCreditLedgerSourceAnonymousSignupBonus, "bonus:fp-1", 60, "anon bonus", "{}")
	require.NoError(t, err)

	usageEventID := uint64(99)
	account, err := store.ChargeHostedCreditsByFingerprint(context.Background(), "fp-1", "req-fp-1", 20, &usageEventID)
	require.NoError(t, err)
	require.Equal(t, 40, account.CreditBalance)

	// Idempotency.
	_, err = store.ChargeHostedCreditsByFingerprint(context.Background(), "fp-1", "req-fp-1", 20, &usageEventID)
	require.NoError(t, err)

	var rows []model.FingerprintCreditLedger
	require.NoError(t, db.Where("source_type = ?", model.HostedCreditLedgerSourceCharge).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].UsageEventID)
	require.Equal(t, uint64(99), *rows[0].UsageEventID)
}

func TestChargeAPIKeyCreditsBasicAndIdempotent(t *testing.T) {
	store, db := newChargeTestStore(t, "file:charge_apikey_basic?mode=memory&cache=shared")

	ownerID := uint64(21)
	require.NoError(t, db.Create(&model.User{ID: ownerID, Email: "o@example.com", Name: "o", InviteCode: "INV", Status: model.UserStatusActive}).Error)
	apiKey := &model.APIKey{
		OwnerUserID:   &ownerID,
		KeyHash:       "hash-charge-1",
		KeyPrefix:     "oc_test",
		Status:        model.APIKeyStatusActive,
		PlanName:      "test",
		HostedEnabled: true,
		AllowedModes:  "hybrid",
	}
	require.NoError(t, db.Create(apiKey).Error)

	_, _, _, err := store.GrantHostedCreditsToUser(context.Background(), ownerID, model.HostedCreditLedgerSourcePurchase, "order:1", 200, "purchase", "{}")
	require.NoError(t, err)

	usageEventID := uint64(7)
	key, err := store.ChargeAPIKeyCredits(context.Background(), apiKey.ID, "req-key-1", 30, &usageEventID)
	require.NoError(t, err)
	require.Equal(t, 170, key.CreditBalance)

	// Repeat — same requestID → no double charge.
	key2, err := store.ChargeAPIKeyCredits(context.Background(), apiKey.ID, "req-key-1", 30, &usageEventID)
	require.NoError(t, err)
	require.Equal(t, 170, key2.CreditBalance)

	var ledgers []model.UserHostedCreditLedger
	require.NoError(t, db.Where("source_type = ?", model.HostedCreditLedgerSourceCharge).Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
}

func TestPrecheckHostedCreditsByUserReturnsExhaustedBelowMin(t *testing.T) {
	store, _ := newChargeTestStore(t, "file:precheck_user?mode=memory&cache=shared")

	// No account exists → exhausted.
	err := store.PrecheckHostedCreditsByUser(context.Background(), 31, 1)
	require.ErrorIs(t, err, ErrHostedCreditsExhausted)

	_, _, _, err = store.GrantHostedCreditsToUser(context.Background(), 31, model.HostedCreditLedgerSourcePurchase, "order:1", 5, "purchase", "{}")
	require.NoError(t, err)

	require.NoError(t, store.PrecheckHostedCreditsByUser(context.Background(), 31, 5))
	require.ErrorIs(t, store.PrecheckHostedCreditsByUser(context.Background(), 31, 6), ErrHostedCreditsExhausted)
}

func TestPrecheckHostedCreditsByFingerprintReturnsExhaustedBelowMin(t *testing.T) {
	store, _ := newChargeTestStore(t, "file:precheck_fp?mode=memory&cache=shared")
	require.ErrorIs(t, store.PrecheckHostedCreditsByFingerprint(context.Background(), "fp-x", 1), ErrHostedCreditsExhausted)
	_, _, _, err := store.GrantHostedCreditsToFingerprint(context.Background(), "fp-x", model.HostedCreditLedgerSourceAnonymousSignupBonus, "bonus:fp-x", 4, "bonus", "{}")
	require.NoError(t, err)
	require.NoError(t, store.PrecheckHostedCreditsByFingerprint(context.Background(), "fp-x", 4))
	require.ErrorIs(t, store.PrecheckHostedCreditsByFingerprint(context.Background(), "fp-x", 5), ErrHostedCreditsExhausted)
}

func TestPrecheckAPIKeyCreditsReturnsExhausted(t *testing.T) {
	store, db := newChargeTestStore(t, "file:precheck_apikey?mode=memory&cache=shared")

	ownerID := uint64(41)
	require.NoError(t, db.Create(&model.User{ID: ownerID, Email: fmt.Sprintf("%d@x", ownerID), Name: "n", InviteCode: "INV2", Status: model.UserStatusActive}).Error)
	apiKey := &model.APIKey{
		OwnerUserID:   &ownerID,
		KeyHash:       "hash-precheck",
		KeyPrefix:     "oc",
		Status:        model.APIKeyStatusActive,
		PlanName:      "p",
		HostedEnabled: true,
		AllowedModes:  "hybrid",
	}
	require.NoError(t, db.Create(apiKey).Error)

	require.ErrorIs(t, store.PrecheckAPIKeyCredits(context.Background(), apiKey.ID, 1), ErrHostedCreditsExhausted)

	_, _, _, err := store.GrantHostedCreditsToUser(context.Background(), ownerID, model.HostedCreditLedgerSourcePurchase, "order:1", 10, "purchase", "{}")
	require.NoError(t, err)
	require.NoError(t, store.PrecheckAPIKeyCredits(context.Background(), apiKey.ID, 10))
	require.ErrorIs(t, store.PrecheckAPIKeyCredits(context.Background(), apiKey.ID, 11), ErrHostedCreditsExhausted)
}

func TestChargeRejectsBlankRequestID(t *testing.T) {
	store, _ := newChargeTestStore(t, "file:charge_blank_req?mode=memory&cache=shared")
	_, err := store.ChargeHostedCreditsByUser(context.Background(), 51, "  ", 1, nil)
	require.Error(t, err)
	_, err = store.ChargeHostedCreditsByFingerprint(context.Background(), "fp", "", 1, nil)
	require.Error(t, err)
}
