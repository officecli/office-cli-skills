package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("cutover-reserved", flag.ExitOnError)
	commit := fs.Bool("commit", false, "actually write changes (default: dry-run)")
	table := fs.String("table", "all", "table to process: user_hosted_credit_accounts, fingerprint_credit_accounts, api_keys, or all")
	dsn := fs.String("dsn", "", "PostgreSQL DSN (default: env POSTGRES_DSN)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if os.Getenv("OFFICECLI_ALLOW_CUTOVER") != "1" {
		return fmt.Errorf("safety gate: set OFFICECLI_ALLOW_CUTOVER=1 to run this tool")
	}

	connStr := *dsn
	if connStr == "" {
		connStr = os.Getenv("POSTGRES_DSN")
	}
	if connStr == "" {
		connStr = "host=127.0.0.1 port=5432 user=officecli password=officecli dbname=officecli_platform sslmode=disable TimeZone=UTC"
	}

	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	return execute(db, *commit, *table, os.Stderr, os.Stdin)
}

type tableResult struct {
	Table   string
	Cleaned int
	Skipped int
}

func execute(db *gorm.DB, commit bool, table string, stderr *os.File, stdin *os.File) error {
	tables := []string{"user_hosted_credit_accounts", "fingerprint_credit_accounts", "api_keys"}
	if table != "all" {
		valid := false
		for _, t := range tables {
			if t == table {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid --table value %q; must be one of: user_hosted_credit_accounts, fingerprint_credit_accounts, api_keys, all", table)
		}
		tables = []string{table}
	}

	ctx := context.Background()

	if !commit {
		fmt.Fprintln(stderr, "[DRY-RUN] no writes will be performed")
		fmt.Fprintln(stderr, "")
		var results []tableResult
		for _, tbl := range tables {
			r, err := processTable(ctx, db, tbl, false, stderr)
			if err != nil {
				return fmt.Errorf("process %s: %w", tbl, err)
			}
			results = append(results, r)
		}
		printReport(stderr, results)
		return nil
	}

	// Preview pass — count what will be changed without writing.
	fmt.Fprintln(stderr, "[PREVIEW] scanning for rows with credit_reserved > 0 ...")
	fmt.Fprintln(stderr, "")
	var preview []tableResult
	for _, tbl := range tables {
		r, err := processTable(ctx, db, tbl, false, stderr)
		if err != nil {
			return fmt.Errorf("preview %s: %w", tbl, err)
		}
		preview = append(preview, r)
	}

	if !hasWork(preview) {
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "nothing to do — all rows already clean or skipped")
		printReport(stderr, preview)
		return nil
	}

	fmt.Fprintln(stderr, "")
	for i := 5; i > 0; i-- {
		fmt.Fprintf(stderr, "\rcommitting in %d seconds... ", i)
		time.Sleep(time.Second)
	}
	fmt.Fprintln(stderr, "")
	fmt.Fprint(stderr, "PROCEED? (yes/no): ")

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "yes" {
		return fmt.Errorf("aborted by user")
	}

	fmt.Fprintln(stderr, "")
	var results []tableResult
	for _, tbl := range tables {
		r, err := processTable(ctx, db, tbl, true, stderr)
		if err != nil {
			return fmt.Errorf("commit %s: %w", tbl, err)
		}
		results = append(results, r)
	}
	printReport(stderr, results)
	return nil
}

func printReport(stderr *os.File, results []tableResult) {
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "=== Final Report ===")
	for _, r := range results {
		fmt.Fprintf(stderr, "  %s: cleaned=%d skipped=%d total=%d\n", r.Table, r.Cleaned, r.Skipped, r.Cleaned+r.Skipped)
	}
}

func hasWork(results []tableResult) bool {
	for _, r := range results {
		if r.Cleaned > 0 {
			return true
		}
	}
	return false
}

func processTable(ctx context.Context, db *gorm.DB, table string, commit bool, stderr *os.File) (tableResult, error) {
	switch table {
	case "user_hosted_credit_accounts":
		return processUserAccounts(ctx, db, commit, stderr)
	case "fingerprint_credit_accounts":
		return processFingerprintAccounts(ctx, db, commit, stderr)
	case "api_keys":
		return processAPIKeys(ctx, db, commit, stderr)
	default:
		return tableResult{}, fmt.Errorf("unknown table %q", table)
	}
}

func processUserAccounts(ctx context.Context, db *gorm.DB, commit bool, stderr *os.File) (tableResult, error) {
	result := tableResult{Table: "user_hosted_credit_accounts"}

	var accounts []model.UserHostedCreditAccount
	if err := db.WithContext(ctx).Where("credit_reserved > 0").Find(&accounts).Error; err != nil {
		return result, err
	}

	for _, acct := range accounts {
		idemKey := fmt.Sprintf("cutover:v1:user:%d", acct.UserID)

		var existing model.UserHostedCreditLedger
		err := db.WithContext(ctx).Where("idempotency_key = ?", idemKey).First(&existing).Error
		if err == nil {
			result.Skipped++
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return result, err
		}

		if !commit {
			fmt.Fprintf(stderr, "  [dry-run] would zero user_id=%d credit_reserved=%d idem=%s\n", acct.UserID, acct.CreditReserved, idemKey)
			result.Cleaned++
			continue
		}

		if err := cutoverUserAccountTx(ctx, db, acct, idemKey); err != nil {
			return result, fmt.Errorf("user_id=%d: %w", acct.UserID, err)
		}
		result.Cleaned++
	}

	fmt.Fprintf(stderr, "[%s] cleaned=%d skipped=%d\n", result.Table, result.Cleaned, result.Skipped)
	return result, nil
}

func cutoverUserAccountTx(ctx context.Context, db *gorm.DB, acct model.UserHostedCreditAccount, idemKey string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.UserHostedCreditAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", acct.UserID).First(&locked).Error; err != nil {
			return err
		}
		if locked.CreditReserved <= 0 {
			return nil
		}
		prevReserved := locked.CreditReserved
		locked.CreditReserved = 0
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		return tx.Create(&model.UserHostedCreditLedger{
			UserID:         acct.UserID,
			SourceType:     model.HostedCreditLedgerSourceReservedCutover,
			IdempotencyKey: idemKey,
			CreditDelta:    0,
			ReservedDelta:  -prevReserved,
			Reason:         "reserved_cutover",
			MetadataJSON:   fmt.Sprintf(`{"prev_reserved":%d}`, prevReserved),
		}).Error
	})
}

func processFingerprintAccounts(ctx context.Context, db *gorm.DB, commit bool, stderr *os.File) (tableResult, error) {
	result := tableResult{Table: "fingerprint_credit_accounts"}

	var accounts []model.FingerprintCreditAccount
	if err := db.WithContext(ctx).Where("credit_reserved > 0").Find(&accounts).Error; err != nil {
		return result, err
	}

	for _, acct := range accounts {
		idemKey := fmt.Sprintf("cutover:v1:fingerprint:%s", acct.FingerprintHash)

		var existing model.FingerprintCreditLedger
		err := db.WithContext(ctx).Where("idempotency_key = ?", idemKey).First(&existing).Error
		if err == nil {
			result.Skipped++
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return result, err
		}

		if !commit {
			fmt.Fprintf(stderr, "  [dry-run] would zero fingerprint=%s credit_reserved=%d idem=%s\n", acct.FingerprintHash, acct.CreditReserved, idemKey)
			result.Cleaned++
			continue
		}

		if err := cutoverFingerprintAccountTx(ctx, db, acct, idemKey); err != nil {
			return result, fmt.Errorf("fingerprint=%s: %w", acct.FingerprintHash, err)
		}
		result.Cleaned++
	}

	fmt.Fprintf(stderr, "[%s] cleaned=%d skipped=%d\n", result.Table, result.Cleaned, result.Skipped)
	return result, nil
}

func cutoverFingerprintAccountTx(ctx context.Context, db *gorm.DB, acct model.FingerprintCreditAccount, idemKey string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.FingerprintCreditAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("fingerprint_hash = ?", acct.FingerprintHash).First(&locked).Error; err != nil {
			return err
		}
		if locked.CreditReserved <= 0 {
			return nil
		}
		prevReserved := locked.CreditReserved
		locked.CreditReserved = 0
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		return tx.Create(&model.FingerprintCreditLedger{
			FingerprintHash: acct.FingerprintHash,
			SourceType:      model.HostedCreditLedgerSourceReservedCutover,
			IdempotencyKey:  idemKey,
			CreditDelta:     0,
			ReservedDelta:   -prevReserved,
			Reason:          "reserved_cutover",
			MetadataJSON:    fmt.Sprintf(`{"prev_reserved":%d}`, prevReserved),
		}).Error
	})
}

func processAPIKeys(ctx context.Context, db *gorm.DB, commit bool, stderr *os.File) (tableResult, error) {
	result := tableResult{Table: "api_keys"}

	var keys []model.APIKey
	if err := db.WithContext(ctx).Where("credit_reserved > 0").Find(&keys).Error; err != nil {
		return result, err
	}

	for _, key := range keys {
		idemKey := fmt.Sprintf("cutover:v1:apikey:%d", key.ID)

		var existing model.UserHostedCreditLedger
		err := db.WithContext(ctx).Where("idempotency_key = ?", idemKey).First(&existing).Error
		if err == nil {
			result.Skipped++
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return result, err
		}

		if !commit {
			fmt.Fprintf(stderr, "  [dry-run] would zero api_key_id=%d credit_reserved=%d idem=%s\n", key.ID, key.CreditReserved, idemKey)
			result.Cleaned++
			continue
		}

		if err := cutoverAPIKeyTx(ctx, db, key, idemKey); err != nil {
			return result, fmt.Errorf("api_key_id=%d: %w", key.ID, err)
		}
		result.Cleaned++
	}

	fmt.Fprintf(stderr, "[%s] cleaned=%d skipped=%d\n", result.Table, result.Cleaned, result.Skipped)
	return result, nil
}

func cutoverAPIKeyTx(ctx context.Context, db *gorm.DB, key model.APIKey, idemKey string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.APIKey
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, key.ID).Error; err != nil {
			return err
		}
		if locked.CreditReserved <= 0 {
			return nil
		}
		prevReserved := locked.CreditReserved
		if err := tx.Model(&model.APIKey{}).Where("id = ?", key.ID).Update("credit_reserved", 0).Error; err != nil {
			return err
		}
		if locked.OwnerUserID != nil && *locked.OwnerUserID != 0 {
			return tx.Create(&model.UserHostedCreditLedger{
				UserID:         *locked.OwnerUserID,
				SourceType:     model.HostedCreditLedgerSourceReservedCutover,
				IdempotencyKey: idemKey,
				CreditDelta:    0,
				ReservedDelta:  -prevReserved,
				Reason:         "reserved_cutover",
				MetadataJSON:   fmt.Sprintf(`{"api_key_id":%d,"prev_reserved":%d}`, key.ID, prevReserved),
			}).Error
		}
		return nil
	})
}
