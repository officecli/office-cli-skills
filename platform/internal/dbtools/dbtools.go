package dbtools

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type TableSummary struct {
	Table       string
	Rows        int64
	MaxIDSource uint64
	MaxIDTarget uint64
}

var copyTables = []string{
	"users",
	"api_keys",
	"free_quotas",
	"daily_free_quotas",
	"stripe_customers",
	"orders",
	"billing_events",
	"usage_events",
	"reward_grants",
	"user_referrals",
	"discord_connections",
	"admin_audit_logs",
}

func CopyMySQLToPostgres(ctx context.Context, mysqlDSN, postgresDSN string) ([]TableSummary, error) {
	source, err := gorm.Open(mysql.Open(mysqlDSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql source: %w", err)
	}
	target, err := gorm.Open(postgres.Open(postgresDSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open postgres target: %w", err)
	}

	if err := ping(ctx, source); err != nil {
		return nil, fmt.Errorf("ping mysql source: %w", err)
	}
	if err := ping(ctx, target); err != nil {
		return nil, fmt.Errorf("ping postgres target: %w", err)
	}

	if err := truncateTarget(ctx, target); err != nil {
		return nil, err
	}

	summaries := make([]TableSummary, 0, len(copyTables))
	for _, table := range copyTables {
		summary, err := copyTable(ctx, source, target, table)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	for _, table := range copyTables {
		if err := resetSequence(ctx, target, table); err != nil {
			return nil, err
		}
	}
	return summaries, nil
}

func ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func truncateTarget(ctx context.Context, target *gorm.DB) error {
	stmt := "TRUNCATE TABLE " + strings.Join(copyTables, ", ") + " RESTART IDENTITY CASCADE"
	if err := target.WithContext(ctx).Exec(stmt).Error; err != nil {
		return fmt.Errorf("truncate postgres target: %w", err)
	}
	return nil
}

func copyTable(ctx context.Context, source, target *gorm.DB, table string) (TableSummary, error) {
	summary := TableSummary{Table: table}

	var rows []map[string]any
	if err := source.WithContext(ctx).Table(table).Order("id ASC").Find(&rows).Error; err != nil {
		return summary, fmt.Errorf("read mysql %s: %w", table, err)
	}
	if len(rows) > 0 {
		if err := target.WithContext(ctx).Table(table).CreateInBatches(rows, 200).Error; err != nil {
			return summary, fmt.Errorf("write postgres %s: %w", table, err)
		}
	}

	var sourceCount, targetCount int64
	if err := source.WithContext(ctx).Table(table).Count(&sourceCount).Error; err != nil {
		return summary, fmt.Errorf("count mysql %s: %w", table, err)
	}
	if err := target.WithContext(ctx).Table(table).Count(&targetCount).Error; err != nil {
		return summary, fmt.Errorf("count postgres %s: %w", table, err)
	}
	if sourceCount != targetCount {
		return summary, fmt.Errorf("row count mismatch for %s: mysql=%d postgres=%d", table, sourceCount, targetCount)
	}

	sourceMaxID, err := maxID(ctx, source, table)
	if err != nil {
		return summary, fmt.Errorf("max id mysql %s: %w", table, err)
	}
	targetMaxID, err := maxID(ctx, target, table)
	if err != nil {
		return summary, fmt.Errorf("max id postgres %s: %w", table, err)
	}
	if sourceMaxID != targetMaxID {
		return summary, fmt.Errorf("max id mismatch for %s: mysql=%d postgres=%d", table, sourceMaxID, targetMaxID)
	}

	summary.Rows = sourceCount
	summary.MaxIDSource = sourceMaxID
	summary.MaxIDTarget = targetMaxID
	return summary, nil
}

func maxID(ctx context.Context, db *gorm.DB, table string) (uint64, error) {
	var maxID uint64
	if err := db.WithContext(ctx).Raw("SELECT COALESCE(MAX(id), 0) FROM " + table).Scan(&maxID).Error; err != nil {
		return 0, err
	}
	return maxID, nil
}

func resetSequence(ctx context.Context, target *gorm.DB, table string) error {
	var sequenceName string
	if err := target.WithContext(ctx).Raw("SELECT pg_get_serial_sequence(?, 'id')", table).Scan(&sequenceName).Error; err != nil {
		return fmt.Errorf("resolve sequence for %s: %w", table, err)
	}
	if strings.TrimSpace(sequenceName) == "" {
		return nil
	}
	var maxID int64
	if err := target.WithContext(ctx).Raw("SELECT COALESCE(MAX(id), 0) FROM " + table).Scan(&maxID).Error; err != nil {
		return fmt.Errorf("load max id for %s: %w", table, err)
	}
	value := maxID
	called := true
	if value == 0 {
		value = 1
		called = false
	}
	if err := target.WithContext(ctx).Exec("SELECT setval(?::regclass, ?, ?)", sequenceName, value, called).Error; err != nil {
		return fmt.Errorf("reset sequence for %s: %w", table, err)
	}
	return nil
}
