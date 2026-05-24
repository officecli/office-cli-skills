package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.UserHostedCreditAccount{},
		&model.UserHostedCreditLedger{},
		&model.FingerprintCreditAccount{},
		&model.FingerprintCreditLedger{},
		&model.APIKey{},
	))
	return db
}

func seedTestData(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&model.UserHostedCreditAccount{
		UserID: 1, CreditBalance: 500, CreditReserved: 50,
	}).Error)
	require.NoError(t, db.Create(&model.UserHostedCreditAccount{
		UserID: 2, CreditBalance: 100, CreditReserved: 0,
	}).Error)

	require.NoError(t, db.Create(&model.FingerprintCreditAccount{
		FingerprintHash: "abc123", CreditBalance: 200, CreditReserved: 30,
	}).Error)
	require.NoError(t, db.Create(&model.FingerprintCreditAccount{
		FingerprintHash: "def456", CreditBalance: 100, CreditReserved: 0,
	}).Error)

	ownerID := uint64(1)
	require.NoError(t, db.Create(&model.APIKey{
		KeyHash: "hash1", KeyPrefix: "cop_1", Status: model.APIKeyStatusActive,
		PlanName: "test", OwnerUserID: &ownerID,
		CreditBalance: 500, CreditReserved: 20,
	}).Error)
	require.NoError(t, db.Create(&model.APIKey{
		KeyHash: "hash2", KeyPrefix: "cop_2", Status: model.APIKeyStatusActive,
		PlanName: "test", CreditBalance: 100, CreditReserved: 0,
	}).Error)
}

func TestDryRunDoesNotModifyDB(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	stderr, err := os.CreateTemp("", "cutover-test-*")
	require.NoError(t, err)
	defer os.Remove(stderr.Name())
	defer stderr.Close()

	require.NoError(t, execute(db, false, "all", stderr, nil))

	var userAcct model.UserHostedCreditAccount
	require.NoError(t, db.First(&userAcct, "user_id = ?", 1).Error)
	require.Equal(t, 50, userAcct.CreditReserved, "dry-run should not change credit_reserved")
	require.Equal(t, 500, userAcct.CreditBalance, "dry-run should not change credit_balance")

	var fpAcct model.FingerprintCreditAccount
	require.NoError(t, db.First(&fpAcct, "fingerprint_hash = ?", "abc123").Error)
	require.Equal(t, 30, fpAcct.CreditReserved)

	var apiKey model.APIKey
	require.NoError(t, db.First(&apiKey, "key_hash = ?", "hash1").Error)
	require.Equal(t, 20, apiKey.CreditReserved)

	var userLedgerCount int64
	db.Model(&model.UserHostedCreditLedger{}).Where("source_type = ?", model.HostedCreditLedgerSourceReservedCutover).Count(&userLedgerCount)
	require.Equal(t, int64(0), userLedgerCount, "dry-run should not write ledger entries")

	var fpLedgerCount int64
	db.Model(&model.FingerprintCreditLedger{}).Where("source_type = ?", model.HostedCreditLedgerSourceReservedCutover).Count(&fpLedgerCount)
	require.Equal(t, int64(0), fpLedgerCount)
}

func TestCommitZerosReservedAndWritesLedger(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	stderr, err := os.CreateTemp("", "cutover-test-*")
	require.NoError(t, err)
	defer os.Remove(stderr.Name())
	defer stderr.Close()

	for _, tbl := range []string{"user_hosted_credit_accounts", "fingerprint_credit_accounts", "api_keys"} {
		r, err := processTable(t.Context(), db, tbl, true, stderr)
		require.NoError(t, err)
		require.Greater(t, r.Cleaned, 0, "should clean at least one row in %s", tbl)
	}

	var userAcct model.UserHostedCreditAccount
	require.NoError(t, db.First(&userAcct, "user_id = ?", 1).Error)
	require.Equal(t, 0, userAcct.CreditReserved)
	require.Equal(t, 500, userAcct.CreditBalance, "credit_balance should not change")

	var userAcct2 model.UserHostedCreditAccount
	require.NoError(t, db.First(&userAcct2, "user_id = ?", 2).Error)
	require.Equal(t, 0, userAcct2.CreditReserved, "already-zero should stay zero")

	var fpAcct model.FingerprintCreditAccount
	require.NoError(t, db.First(&fpAcct, "fingerprint_hash = ?", "abc123").Error)
	require.Equal(t, 0, fpAcct.CreditReserved)
	require.Equal(t, 200, fpAcct.CreditBalance)

	var apiKey model.APIKey
	require.NoError(t, db.First(&apiKey, "key_hash = ?", "hash1").Error)
	require.Equal(t, 0, apiKey.CreditReserved)
	require.Equal(t, 500, apiKey.CreditBalance)

	var userLedger model.UserHostedCreditLedger
	require.NoError(t, db.Where("idempotency_key = ?", "cutover:v1:user:1").First(&userLedger).Error)
	require.Equal(t, model.HostedCreditLedgerSourceReservedCutover, userLedger.SourceType)
	require.Equal(t, 0, userLedger.CreditDelta)
	require.Equal(t, -50, userLedger.ReservedDelta)

	var fpLedger model.FingerprintCreditLedger
	require.NoError(t, db.Where("idempotency_key = ?", "cutover:v1:fingerprint:abc123").First(&fpLedger).Error)
	require.Equal(t, model.HostedCreditLedgerSourceReservedCutover, fpLedger.SourceType)
	require.Equal(t, -30, fpLedger.ReservedDelta)

	var apiKeyLedger model.UserHostedCreditLedger
	require.NoError(t, db.Where("idempotency_key = ?", "cutover:v1:apikey:1").First(&apiKeyLedger).Error)
	require.Equal(t, model.HostedCreditLedgerSourceReservedCutover, apiKeyLedger.SourceType)
	require.Equal(t, -20, apiKeyLedger.ReservedDelta)
}

func TestIdempotentRerun(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	stderr, err := os.CreateTemp("", "cutover-test-*")
	require.NoError(t, err)
	defer os.Remove(stderr.Name())
	defer stderr.Close()

	// First run: commit changes.
	for _, tbl := range []string{"user_hosted_credit_accounts", "fingerprint_credit_accounts", "api_keys"} {
		_, err := processTable(t.Context(), db, tbl, true, stderr)
		require.NoError(t, err)
	}

	// Count ledger entries after first run.
	var userLedgerCount1 int64
	db.Model(&model.UserHostedCreditLedger{}).Where("source_type = ?", model.HostedCreditLedgerSourceReservedCutover).Count(&userLedgerCount1)
	var fpLedgerCount1 int64
	db.Model(&model.FingerprintCreditLedger{}).Where("source_type = ?", model.HostedCreditLedgerSourceReservedCutover).Count(&fpLedgerCount1)
	require.Greater(t, userLedgerCount1, int64(0))
	require.Greater(t, fpLedgerCount1, int64(0))

	// Simulate a re-run scenario: manually bump credit_reserved back up to
	// re-trigger the WHERE clause so the idempotency guard is exercised.
	db.Model(&model.UserHostedCreditAccount{}).Where("user_id = ?", 1).Update("credit_reserved", 50)
	db.Model(&model.FingerprintCreditAccount{}).Where("fingerprint_hash = ?", "abc123").Update("credit_reserved", 30)
	db.Model(&model.APIKey{}).Where("key_hash = ?", "hash1").Update("credit_reserved", 20)

	// Second run should skip all because ledger entries already exist.
	var results []tableResult
	for _, tbl := range []string{"user_hosted_credit_accounts", "fingerprint_credit_accounts", "api_keys"} {
		r, err := processTable(t.Context(), db, tbl, true, stderr)
		require.NoError(t, err)
		results = append(results, r)
	}

	for _, r := range results {
		require.Equal(t, 0, r.Cleaned, "second run should clean nothing in %s", r.Table)
	}
	totalSkipped := 0
	for _, r := range results {
		totalSkipped += r.Skipped
	}
	require.Greater(t, totalSkipped, 0, "second run should report skipped rows")

	// Verify no new ledger entries were created.
	var userLedgerCount2 int64
	db.Model(&model.UserHostedCreditLedger{}).Where("source_type = ?", model.HostedCreditLedgerSourceReservedCutover).Count(&userLedgerCount2)
	var fpLedgerCount2 int64
	db.Model(&model.FingerprintCreditLedger{}).Where("source_type = ?", model.HostedCreditLedgerSourceReservedCutover).Count(&fpLedgerCount2)
	require.Equal(t, userLedgerCount1, userLedgerCount2, "no new user ledger entries on re-run")
	require.Equal(t, fpLedgerCount1, fpLedgerCount2, "no new fingerprint ledger entries on re-run")
}

func TestSingleTableMode(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	stderr, err := os.CreateTemp("", "cutover-test-*")
	require.NoError(t, err)
	defer os.Remove(stderr.Name())
	defer stderr.Close()

	r, err := processTable(t.Context(), db, "user_hosted_credit_accounts", true, stderr)
	require.NoError(t, err)
	require.Equal(t, 1, r.Cleaned)

	var fpAcct model.FingerprintCreditAccount
	require.NoError(t, db.First(&fpAcct, "fingerprint_hash = ?", "abc123").Error)
	require.Equal(t, 30, fpAcct.CreditReserved, "fingerprint table should be untouched")

	var apiKey model.APIKey
	require.NoError(t, db.First(&apiKey, "key_hash = ?", "hash1").Error)
	require.Equal(t, 20, apiKey.CreditReserved, "api_keys table should be untouched")
}

func TestInvalidTableFlag(t *testing.T) {
	db := setupTestDB(t)
	stderr, err := os.CreateTemp("", "cutover-test-*")
	require.NoError(t, err)
	defer os.Remove(stderr.Name())
	defer stderr.Close()

	err = execute(db, false, "nonexistent", stderr, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid --table value")
}
