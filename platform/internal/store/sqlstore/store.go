package sqlstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/officecli/officecli/platform/internal/billing"
	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
)

type Store struct {
	db *gorm.DB
}

type UsageEventFilter struct {
	Mode        string
	Result      string
	ReasonCode  string
	Fingerprint string
	APIKeyID    *uint64
	StartTime   *time.Time
	EndTime     *time.Time
}

type schemaMigration struct {
	Version   string    `gorm:"column:version;primaryKey"`
	AppliedAt time.Time `gorm:"column:applied_at;autoCreateTime"`
}

func (schemaMigration) TableName() string { return "schema_migrations" }

func New(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func NewWithDB(db *gorm.DB) *Store { return &Store{db: db} }

func IsDuplicateError(err error) bool {
	var pgErr *pgconn.PgError
	return (errors.As(err, &pgErr) && pgErr.Code == "23505") || errors.Is(err, gorm.ErrDuplicatedKey)
}

func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (s *Store) EnsureMigrations(ctx context.Context) error {
	if err := s.db.WithContext(ctx).Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`).Error; err != nil {
		return err
	}

	files, err := filepath.Glob("migrations/postgres/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, path := range files {
		version := migrationVersion(path)
		applied, err := s.isMigrationApplied(ctx, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := s.ApplyMigrationFile(ctx, path); err != nil {
			return fmt.Errorf("apply migration %s: %w", filepath.Base(path), err)
		}
		if err := s.db.WithContext(ctx).Create(&schemaMigration{Version: version}).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func migrationVersion(path string) string {
	base := filepath.Base(path)
	parts := strings.SplitN(base, "_", 2)
	return strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
}

func (s *Store) isMigrationApplied(ctx context.Context, version string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&schemaMigration{}).Where("version = ?", version).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) ApplyMigrationFile(ctx context.Context, path string) error {
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec(string(sqlBytes)).Error
}

func (s *Store) FindAPIKeyByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	var key model.APIKey
	if err := s.db.WithContext(ctx).Where("key_hash = ?", hash).First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return s.withEffectiveAPIKeyStatus(ctx, key)
}

func (s *Store) FindAPIKeyByID(ctx context.Context, id uint64) (*model.APIKey, error) {
	var key model.APIKey
	if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return withRemaining(key), nil
}

func (s *Store) FindAPIKeysByOwner(ctx context.Context, userID uint64) ([]model.APIKey, error) {
	var keys []model.APIKey
	if err := s.db.WithContext(ctx).Where("owner_user_id = ?", userID).Order("created_at desc").Find(&keys).Error; err != nil {
		return nil, err
	}
	for i := range keys {
		keys[i] = *withRemaining(keys[i])
	}
	return keys, nil
}

func (s *Store) TouchAPIKeyLastUsedAt(ctx context.Context, id uint64, usedAt time.Time) error {
	return s.db.WithContext(ctx).Model(&model.APIKey{}).Where("id = ?", id).Update("last_used_at", usedAt).Error
}

func (s *Store) FindUserAIGatewayAPIKeyByUserID(ctx context.Context, userID uint64) (*model.UserAIGatewayAPIKey, error) {
	var key model.UserAIGatewayAPIKey
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &key, nil
}

func (s *Store) ClaimUserAIGatewayAPIKeyCreation(ctx context.Context, userID uint64, upstreamName string) (*model.UserAIGatewayAPIKey, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	upstreamName = strings.TrimSpace(upstreamName)
	if upstreamName == "" {
		return nil, fmt.Errorf("upstream_name is required")
	}
	var key model.UserAIGatewayAPIKey
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&key).Error
	if err == nil {
		if key.Status == model.UserAIGatewayAPIKeyStatusError {
			updates := map[string]any{
				"status":        model.UserAIGatewayAPIKeyStatusCreating,
				"upstream_name": upstreamName,
				"last_error":    "",
			}
			if err := s.db.WithContext(ctx).Model(&model.UserAIGatewayAPIKey{}).Where("id = ?", key.ID).Updates(updates).Error; err != nil {
				return nil, err
			}
			claimed, err := s.FindUserAIGatewayAPIKeyByUserID(ctx, userID)
			if claimed != nil {
				claimed.CreationClaimed = true
			}
			return claimed, err
		}
		return &key, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	key = model.UserAIGatewayAPIKey{
		UserID:       userID,
		Status:       model.UserAIGatewayAPIKeyStatusCreating,
		UpstreamName: upstreamName,
	}
	if err := s.db.WithContext(ctx).Create(&key).Error; err != nil {
		if IsDuplicateError(err) {
			return s.ClaimUserAIGatewayAPIKeyCreation(ctx, userID, upstreamName)
		}
		return nil, err
	}
	key.CreationClaimed = true
	return &key, nil
}

func (s *Store) ActivateUserAIGatewayAPIKey(ctx context.Context, userID uint64, ciphertext, prefix, upstreamID, upstreamName string) (*model.UserAIGatewayAPIKey, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	updates := map[string]any{
		"status":         model.UserAIGatewayAPIKeyStatusActive,
		"key_ciphertext": strings.TrimSpace(ciphertext),
		"key_prefix":     strings.TrimSpace(prefix),
		"upstream_id":    strings.TrimSpace(upstreamID),
		"upstream_name":  strings.TrimSpace(upstreamName),
		"last_error":     "",
	}
	if updates["key_ciphertext"] == "" {
		return nil, fmt.Errorf("key_ciphertext is required")
	}
	if updates["key_prefix"] == "" {
		return nil, fmt.Errorf("key_prefix is required")
	}
	if updates["upstream_name"] == "" {
		return nil, fmt.Errorf("upstream_name is required")
	}
	if err := s.db.WithContext(ctx).Model(&model.UserAIGatewayAPIKey{}).Where("user_id = ?", userID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.FindUserAIGatewayAPIKeyByUserID(ctx, userID)
}

func (s *Store) MarkUserAIGatewayAPIKeyCreationError(ctx context.Context, userID uint64, upstreamName, message string) (*model.UserAIGatewayAPIKey, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	updates := map[string]any{
		"status":        model.UserAIGatewayAPIKeyStatusError,
		"upstream_name": strings.TrimSpace(upstreamName),
		"last_error":    strings.TrimSpace(message),
	}
	if updates["upstream_name"] == "" {
		return nil, fmt.Errorf("upstream_name is required")
	}
	if err := s.db.WithContext(ctx).Model(&model.UserAIGatewayAPIKey{}).Where("user_id = ?", userID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.FindUserAIGatewayAPIKeyByUserID(ctx, userID)
}

func (s *Store) GetOrCreateFreeQuota(ctx context.Context, fingerprint string, defaultLimit int) (*model.FreeQuota, bool, error) {
	var quota model.FreeQuota
	err := s.db.WithContext(ctx).Where("fingerprint_hash = ?", fingerprint).First(&quota).Error
	if err == nil {
		return &quota, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	quota = model.FreeQuota{FingerprintHash: fingerprint, FreeLimit: defaultLimit}
	if err := s.db.WithContext(ctx).Create(&quota).Error; err != nil {
		if IsDuplicateError(err) {
			return s.GetOrCreateFreeQuota(ctx, fingerprint, defaultLimit)
		}
		return nil, false, err
	}
	return &quota, true, nil
}

func (s *Store) GetOrCreateDailyFreeQuota(ctx context.Context, fingerprint string, usageDate string, documentType string, defaultLimit int) (*model.DailyFreeQuota, bool, error) {
	var quota model.DailyFreeQuota
	err := s.db.WithContext(ctx).Where("fingerprint_hash = ? AND usage_date = ? AND document_type = ?", fingerprint, usageDate, documentType).First(&quota).Error
	if err == nil {
		return &quota, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	quota = model.DailyFreeQuota{
		FingerprintHash: fingerprint,
		UsageDate:       usageDate,
		DocumentType:    documentType,
		DailyLimit:      defaultLimit,
		DailyUsed:       0,
	}
	if err := s.db.WithContext(ctx).Create(&quota).Error; err != nil {
		if IsDuplicateError(err) {
			return s.GetOrCreateDailyFreeQuota(ctx, fingerprint, usageDate, documentType, defaultLimit)
		}
		return nil, false, err
	}
	return &quota, true, nil
}

func (s *Store) GetFreeQuotaByFingerprint(ctx context.Context, fingerprint string) (*model.FreeQuota, error) {
	var quota model.FreeQuota
	if err := s.db.WithContext(ctx).Where("fingerprint_hash = ?", fingerprint).First(&quota).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &quota, nil
}

func (s *Store) GetDailyFreeQuota(ctx context.Context, fingerprint string, usageDate string, documentType string) (*model.DailyFreeQuota, error) {
	var quota model.DailyFreeQuota
	if err := s.db.WithContext(ctx).Where("fingerprint_hash = ? AND usage_date = ? AND document_type = ?", fingerprint, usageDate, documentType).First(&quota).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &quota, nil
}

func (s *Store) ConsumeFreeQuota(ctx context.Context, fingerprint string) (*model.FreeQuota, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	var quota model.FreeQuota
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("fingerprint_hash = ?", fingerprint).First(&quota).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if quota.FreeUsed >= quota.FreeLimit {
		tx.Rollback()
		return nil, fmt.Errorf("free quota exhausted")
	}
	quota.FreeUsed++
	if err := tx.Save(&quota).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &quota, nil
}

func (s *Store) ConsumeDailyFreeQuota(ctx context.Context, fingerprint string, usageDate string, documentType string, defaultLimit int) (*model.DailyFreeQuota, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	var quota model.DailyFreeQuota
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("fingerprint_hash = ? AND usage_date = ? AND document_type = ?", fingerprint, usageDate, documentType).
		First(&quota).Error
	switch {
	case err == nil:
	case errors.Is(err, gorm.ErrRecordNotFound):
		quota = model.DailyFreeQuota{
			FingerprintHash: fingerprint,
			UsageDate:       usageDate,
			DocumentType:    documentType,
			DailyLimit:      defaultLimit,
			DailyUsed:       0,
		}
		if err := tx.Create(&quota).Error; err != nil {
			tx.Rollback()
			if IsDuplicateError(err) {
				return s.ConsumeDailyFreeQuota(ctx, fingerprint, usageDate, documentType, defaultLimit)
			}
			return nil, err
		}
	default:
		tx.Rollback()
		return nil, err
	}

	if quota.DailyUsed >= quota.DailyLimit {
		tx.Rollback()
		return nil, fmt.Errorf("free quota exhausted")
	}
	quota.DailyUsed++
	if err := tx.Save(&quota).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &quota, nil
}

func (s *Store) ConsumePaidQuotaByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	var key model.APIKey
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key_hash = ?", hash).First(&key).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if key.Status != model.APIKeyStatusActive {
		tx.Rollback()
		return nil, fmt.Errorf("api key is disabled")
	}
	disabled, err := ownerUserDisabledTx(tx, key.OwnerUserID)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if disabled {
		tx.Rollback()
		return nil, fmt.Errorf("api key is disabled")
	}
	if key.QuotaTotal != nil && key.PaidQuotaRemaining() <= 0 {
		tx.Rollback()
		return nil, fmt.Errorf("paid quota exhausted")
	}
	key.QuotaUsed++
	if err := tx.Save(&key).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return withRemaining(key), nil
}

func (s *Store) AddPaidQuotaToAPIKey(ctx context.Context, apiKeyID uint64, quotaAmount int) (*model.APIKey, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	var key model.APIKey
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, apiKeyID).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if key.QuotaTotal == nil {
		total := 0
		key.QuotaTotal = &total
	}
	*key.QuotaTotal += quotaAmount
	if err := tx.Save(&key).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return withRemaining(key), nil
}

func (s *Store) AddCreditBalanceToAPIKey(ctx context.Context, apiKeyID uint64, creditAmount int) (*model.APIKey, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	var key model.APIKey
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, apiKeyID).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	key.CreditBalance += creditAmount
	key.HostedEnabled = true
	switch key.AllowedModes {
	case "", "external_only":
		key.AllowedModes = "hybrid"
	}
	if key.DefaultRuntimeMode == nil || strings.TrimSpace(*key.DefaultRuntimeMode) == "" || strings.TrimSpace(*key.DefaultRuntimeMode) == "external" {
		defaultRuntimeMode := "hosted"
		key.DefaultRuntimeMode = &defaultRuntimeMode
	}
	if err := tx.Save(&key).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return withRemaining(key), nil
}

func (s *Store) ReserveCreditsByHash(ctx context.Context, hash string, credits int) (*model.APIKey, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	var key model.APIKey
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key_hash = ?", hash).First(&key).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if key.Status != model.APIKeyStatusActive {
		tx.Rollback()
		return nil, fmt.Errorf("api key is disabled")
	}
	disabled, err := ownerUserDisabledTx(tx, key.OwnerUserID)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if disabled {
		tx.Rollback()
		return nil, fmt.Errorf("api key is disabled")
	}
	if key.AvailableCredits() < credits {
		tx.Rollback()
		return nil, fmt.Errorf("hosted credits exhausted")
	}
	key.CreditReserved += credits
	if err := tx.Save(&key).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return withRemaining(key), nil
}

func (s *Store) ReleaseReservedCredits(ctx context.Context, apiKeyID uint64, reserved int) (*model.APIKey, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	var key model.APIKey
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, apiKeyID).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	key.CreditReserved -= reserved
	if key.CreditReserved < 0 {
		key.CreditReserved = 0
	}
	if err := tx.Save(&key).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return withRemaining(key), nil
}

func (s *Store) SettleReservedCredits(ctx context.Context, apiKeyID uint64, reserved int, settled int) (*model.APIKey, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	var key model.APIKey
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, apiKeyID).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	key.CreditReserved -= reserved
	if key.CreditReserved < 0 {
		key.CreditReserved = 0
	}
	key.CreditBalance -= settled
	if key.CreditBalance < 0 {
		key.CreditBalance = 0
	}
	if err := tx.Save(&key).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return withRemaining(key), nil
}

func (s *Store) CreateUsageEvent(ctx context.Context, event *model.UsageEvent) error {
	return s.db.WithContext(ctx).Create(event).Error
}

func (s *Store) HostedPricingSettings(ctx context.Context) (*model.HostedPricingSetting, error) {
	var settings model.HostedPricingSetting
	err := s.db.WithContext(ctx).First(&settings, 1).Error
	switch {
	case err == nil:
		return &settings, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}
	settings = model.HostedPricingSetting{ID: 1, MarkupBPS: 3000, Currency: "usd", CreditsPerUSD: 100}
	if err := s.db.WithContext(ctx).Create(&settings).Error; err != nil {
		if IsDuplicateError(err) {
			return s.HostedPricingSettings(ctx)
		}
		return nil, err
	}
	return &settings, nil
}

func (s *Store) UpdateHostedPricingSettings(ctx context.Context, settings model.HostedPricingSetting) (*model.HostedPricingSetting, error) {
	current, err := s.HostedPricingSettings(ctx)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{
		"markup_bps":      settings.MarkupBPS,
		"currency":        strings.ToLower(strings.TrimSpace(settings.Currency)),
		"credits_per_usd": settings.CreditsPerUSD,
	}
	if updates["currency"] == "" {
		updates["currency"] = current.Currency
	}
	if settings.CreditsPerUSD <= 0 {
		updates["credits_per_usd"] = current.CreditsPerUSD
		if current.CreditsPerUSD <= 0 {
			updates["credits_per_usd"] = 100
		}
	}
	if err := s.db.WithContext(ctx).Model(&model.HostedPricingSetting{}).Where("id = ?", current.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.HostedPricingSettings(ctx)
}

func (s *Store) ListHostedModelPricingConfigs(ctx context.Context, enabledOnly bool) ([]model.HostedModelPricingConfig, error) {
	if err := s.ensureDefaultHostedModelPricingConfigs(ctx); err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Model(&model.HostedModelPricingConfig{})
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	var configs []model.HostedModelPricingConfig
	err := query.Order("kind asc, key asc, id asc").Find(&configs).Error
	return configs, err
}

func (s *Store) CreateHostedModelPricingConfig(ctx context.Context, config *model.HostedModelPricingConfig) error {
	return s.db.WithContext(ctx).Create(config).Error
}

func (s *Store) UpdateHostedModelPricingConfig(ctx context.Context, id uint64, values map[string]any) (*model.HostedModelPricingConfig, error) {
	if err := s.db.WithContext(ctx).Model(&model.HostedModelPricingConfig{}).Where("id = ?", id).Updates(values).Error; err != nil {
		return nil, err
	}
	var config model.HostedModelPricingConfig
	if err := s.db.WithContext(ctx).First(&config, id).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *Store) ensureDefaultHostedModelPricingConfigs(ctx context.Context) error {
	defaults := []model.HostedModelPricingConfig{
		{
			Key:      "text_default",
			Kind:     model.HostedModelPricingKindText,
			Provider: "openai",
			Model:    "gpt-4.1",
			Enabled:  true,
		},
		{
			Key:      "image_default",
			Kind:     model.HostedModelPricingKindImage,
			Provider: "openai",
			Model:    "gpt-image-2",
			Enabled:  true,
		},
	}
	for _, cfg := range defaults {
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoNothing: true}).Create(&cfg).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListHostedPricingRules(ctx context.Context, enabledOnly bool) ([]model.HostedPricingRule, error) {
	query := s.db.WithContext(ctx).Model(&model.HostedPricingRule{})
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	var rules []model.HostedPricingRule
	err := query.Order("document_profile asc, provider asc, model asc, id asc").Find(&rules).Error
	return rules, err
}

func (s *Store) CreateHostedPricingRule(ctx context.Context, rule *model.HostedPricingRule) error {
	return s.db.WithContext(ctx).Create(rule).Error
}

func (s *Store) UpdateHostedPricingRule(ctx context.Context, id uint64, values map[string]any) (*model.HostedPricingRule, error) {
	if err := s.db.WithContext(ctx).Model(&model.HostedPricingRule{}).Where("id = ?", id).Updates(values).Error; err != nil {
		return nil, err
	}
	var rule model.HostedPricingRule
	if err := s.db.WithContext(ctx).First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *Store) ListHostedCreditPacks(ctx context.Context, enabledOnly bool) ([]model.HostedCreditPack, error) {
	query := s.db.WithContext(ctx).Model(&model.HostedCreditPack{})
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	var packs []model.HostedCreditPack
	err := query.Order("amount_total asc, id asc").Find(&packs).Error
	return packs, err
}

func (s *Store) CreateHostedCreditPack(ctx context.Context, pack *model.HostedCreditPack) error {
	return s.db.WithContext(ctx).Create(pack).Error
}

func (s *Store) UpdateHostedCreditPack(ctx context.Context, id uint64, values map[string]any) (*model.HostedCreditPack, error) {
	if err := s.db.WithContext(ctx).Model(&model.HostedCreditPack{}).Where("id = ?", id).Updates(values).Error; err != nil {
		return nil, err
	}
	var pack model.HostedCreditPack
	if err := s.db.WithContext(ctx).First(&pack, id).Error; err != nil {
		return nil, err
	}
	return &pack, nil
}

func (s *Store) FindUsageEventByRequestID(ctx context.Context, requestID string) (*model.UsageEvent, error) {
	var event model.UsageEvent
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

func (s *Store) FindRewardGrantByIdempotencyKey(ctx context.Context, key string) (*model.RewardGrant, error) {
	var grant model.RewardGrant
	if err := s.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&grant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &grant, nil
}

func (s *Store) CreateRewardGrant(ctx context.Context, grant *model.RewardGrant) error {
	return s.db.WithContext(ctx).Create(grant).Error
}

func (s *Store) SaveRewardGrant(ctx context.Context, grant *model.RewardGrant) error {
	return s.db.WithContext(ctx).Save(grant).Error
}

func (s *Store) ListRewardGrantsByUser(ctx context.Context, userID uint64) ([]model.RewardGrant, error) {
	var grants []model.RewardGrant
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at asc, id asc").
		Find(&grants).Error
	return grants, err
}

func (s *Store) ListRewardGrants(ctx context.Context) ([]model.RewardGrant, error) {
	var grants []model.RewardGrant
	err := s.db.WithContext(ctx).
		Order("created_at desc, id desc").
		Find(&grants).Error
	return grants, err
}

func (s *Store) ConsumeRewardGrant(ctx context.Context, userID uint64) (*model.RewardGrant, int, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer rollbackOnPanic(tx)

	var grants []model.RewardGrant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		Order("created_at asc, id asc").
		Find(&grants).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	var selected *model.RewardGrant
	for i := range grants {
		if grants[i].Remaining() <= 0 {
			continue
		}
		grants[i].AmountUsed++
		if err := tx.Model(&model.RewardGrant{}).
			Where("id = ?", grants[i].ID).
			Update("amount_used", grants[i].AmountUsed).Error; err != nil {
			tx.Rollback()
			return nil, 0, err
		}
		copied := grants[i]
		selected = &copied
		break
	}
	if selected == nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("reward quota exhausted")
	}

	var remaining int64
	if err := tx.Model(&model.RewardGrant{}).
		Select("COALESCE(SUM(amount_total - amount_used), 0)").
		Where("user_id = ?", userID).
		Scan(&remaining).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return selected, int(remaining), nil
}

func (s *Store) ListUsageEvents(ctx context.Context, filter UsageEventFilter) ([]model.UsageEvent, error) {
	query := s.db.WithContext(ctx).Model(&model.UsageEvent{})
	if filter.Mode != "" {
		query = query.Where("mode = ?", filter.Mode)
	}
	if filter.Result != "" {
		query = query.Where("result = ?", filter.Result)
	}
	if filter.ReasonCode != "" {
		query = query.Where("reason_code = ?", filter.ReasonCode)
	}
	if filter.Fingerprint != "" {
		query = query.Where("fingerprint_hash LIKE ?", "%"+filter.Fingerprint+"%")
	}
	if filter.APIKeyID != nil {
		query = query.Where("api_key_id = ?", *filter.APIKeyID)
	}
	if filter.StartTime != nil {
		query = query.Where("created_at >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		query = query.Where("created_at <= ?", *filter.EndTime)
	}
	var events []model.UsageEvent
	err := query.Order("created_at desc").Limit(200).Find(&events).Error
	return events, err
}

func (s *Store) CreateAuditLog(ctx context.Context, action, targetType, targetID string, payload string) error {
	return s.db.WithContext(ctx).Create(&model.AdminAuditLog{
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		PayloadJSON: payload,
	}).Error
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]model.APIKey, error) {
	var keys []model.APIKey
	if err := s.db.WithContext(ctx).Order("created_at desc").Find(&keys).Error; err != nil {
		return nil, err
	}
	for i := range keys {
		keys[i] = *withRemaining(keys[i])
	}
	return keys, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, key *model.APIKey) error {
	return s.db.WithContext(ctx).Create(key).Error
}

func (s *Store) UpdateAPIKey(ctx context.Context, id uint64, values map[string]any) error {
	return s.db.WithContext(ctx).Model(&model.APIKey{}).Where("id = ?", id).Updates(values).Error
}

func (s *Store) ListDailyFreeQuotas(ctx context.Context, fingerprint string, usageDate string) ([]model.DailyFreeQuota, error) {
	query := s.db.WithContext(ctx).Model(&model.DailyFreeQuota{})
	if fingerprint != "" {
		query = query.Where("fingerprint_hash LIKE ?", "%"+fingerprint+"%")
	}
	if usageDate != "" {
		query = query.Where("usage_date = ?", usageDate)
	}
	var quotas []model.DailyFreeQuota
	if err := query.Order("updated_at desc").Find(&quotas).Error; err != nil {
		return nil, err
	}
	return quotas, nil
}

func (s *Store) UpdateDailyFreeQuota(ctx context.Context, id uint64, dailyLimit int) error {
	return s.db.WithContext(ctx).Model(&model.DailyFreeQuota{}).Where("id = ?", id).Update("daily_limit", dailyLimit).Error
}

func (s *Store) SaveGoogleUser(ctx context.Context, googleSub, email, name string, avatarURL *string) (*model.User, error) {
	var user model.User
	err := s.db.WithContext(ctx).Where("google_sub = ?", googleSub).First(&user).Error
	switch {
	case err == nil:
		user.Email = email
		user.Name = name
		user.AvatarURL = avatarURL
		if user.InviteCode == "" {
			user.InviteCode = buildInviteCode(user.ID)
		}
		if err := s.db.WithContext(ctx).Save(&user).Error; err != nil {
			return nil, err
		}
		return &user, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	user = model.User{
		GoogleSub: googleSub,
		Email:     email,
		Name:      name,
		AvatarURL: avatarURL,
		// Create with a deterministic temporary value, then rewrite to the final
		// ID-derived invite code once the auto-increment ID is known.
		InviteCode: buildPendingInviteCode(googleSub),
		Status:     model.UserStatusActive,
	}
	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	user.InviteCode = buildInviteCode(user.ID)
	if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Update("invite_code", user.InviteCode).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func buildInviteCode(userID uint64) string {
	code := strconv.FormatUint(userID, 36)
	for len(code) < 6 {
		code = "0" + code
	}
	return "invite-" + code
}

func buildPendingInviteCode(googleSub string) string {
	if googleSub == "" {
		return "pending-invite"
	}
	return "pending-" + googleSub
}

func (s *Store) FindUserByInviteCode(ctx context.Context, inviteCode string) (*model.User, error) {
	var user model.User
	if err := s.db.WithContext(ctx).Where("invite_code = ?", inviteCode).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (s *Store) GetUserByID(ctx context.Context, id uint64) (*model.User, error) {
	var user model.User
	if err := s.db.WithContext(ctx).First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (s *Store) ListUsers(ctx context.Context, query string) ([]model.User, error) {
	var users []model.User
	db := s.db.WithContext(ctx).Model(&model.User{})
	normalized := strings.TrimSpace(query)
	if normalized != "" {
		if id, err := strconv.ParseUint(normalized, 10, 64); err == nil {
			db = db.Where("id = ?", id)
		} else {
			db = db.Where("email LIKE ?", "%"+normalized+"%")
		}
	}
	if err := db.Order("created_at desc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Store) ListReferralsByInviterUserID(ctx context.Context, inviterUserID uint64) ([]model.UserReferral, error) {
	var referrals []model.UserReferral
	if err := s.db.WithContext(ctx).Where("inviter_user_id = ?", inviterUserID).Order("created_at desc").Find(&referrals).Error; err != nil {
		return nil, err
	}
	return referrals, nil
}

func (s *Store) ListReferrals(ctx context.Context) ([]model.UserReferral, error) {
	var referrals []model.UserReferral
	if err := s.db.WithContext(ctx).Order("created_at desc, id desc").Find(&referrals).Error; err != nil {
		return nil, err
	}
	return referrals, nil
}

func (s *Store) FindReferralByInvitedUserID(ctx context.Context, invitedUserID uint64) (*model.UserReferral, error) {
	var referral model.UserReferral
	if err := s.db.WithContext(ctx).Where("invited_user_id = ?", invitedUserID).First(&referral).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &referral, nil
}

func (s *Store) CountReferralsByInviterUserID(ctx context.Context, inviterUserID uint64) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.UserReferral{}).Where("inviter_user_id = ?", inviterUserID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) SaveReferral(ctx context.Context, referral *model.UserReferral) error {
	return s.db.WithContext(ctx).Save(referral).Error
}

func (s *Store) RegisterReferralWithinLimit(ctx context.Context, inviterUserID, invitedUserID uint64, inviteCode string, registeredAt time.Time) (*model.UserReferral, error) {
	var referral *model.UserReferral
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inviter model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ?", inviterUserID).
			First(&inviter).Error; err != nil {
			return err
		}

		var existing model.UserReferral
		if err := tx.Where("invited_user_id = ?", invitedUserID).First(&existing).Error; err == nil {
			referral = &existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var count int64
		if err := tx.Model(&model.UserReferral{}).Where("inviter_user_id = ?", inviterUserID).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(growthsvc.MaxReferralsPerInviter) {
			return growthsvc.ErrInviteLimitReached
		}

		record := &model.UserReferral{
			InviterUserID: inviterUserID,
			InvitedUserID: invitedUserID,
			InviteCode:    inviteCode,
			RegisteredAt:  registeredAt,
		}
		if err := tx.Create(record).Error; err != nil {
			if isDuplicateConstraintError(err) {
				if loadErr := tx.Where("invited_user_id = ?", invitedUserID).First(&existing).Error; loadErr == nil {
					referral = &existing
					return nil
				}
			}
			return err
		}

		referral = record
		return nil
	})
	if err != nil {
		return nil, err
	}
	return referral, nil
}

func isDuplicateConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return IsDuplicateError(err) || errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (s *Store) FindDiscordConnectionByUserID(ctx context.Context, userID uint64) (*model.DiscordConnection, error) {
	var connection model.DiscordConnection
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&connection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &connection, nil
}

func (s *Store) FindDiscordConnectionByDiscordUserID(ctx context.Context, discordUserID string) (*model.DiscordConnection, error) {
	var connection model.DiscordConnection
	if err := s.db.WithContext(ctx).Where("discord_user_id = ?", discordUserID).First(&connection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &connection, nil
}

func (s *Store) SaveDiscordConnection(ctx context.Context, connection *model.DiscordConnection) error {
	return s.db.WithContext(ctx).Save(connection).Error
}

func (s *Store) ListDiscordConnections(ctx context.Context) ([]model.DiscordConnection, error) {
	var connections []model.DiscordConnection
	if err := s.db.WithContext(ctx).Order("created_at desc, id desc").Find(&connections).Error; err != nil {
		return nil, err
	}
	return connections, nil
}

func (s *Store) UpdateUser(ctx context.Context, id uint64, values map[string]any) error {
	return s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(values).Error
}

func (s *Store) DisableAPIKeysByOwnerUserID(ctx context.Context, userID uint64) error {
	return s.db.WithContext(ctx).
		Model(&model.APIKey{}).
		Where("owner_user_id = ? AND status <> ?", userID, model.APIKeyStatusDisabled).
		Update("status", model.APIKeyStatusDisabled).Error
}

func (s *Store) GetOrCreateStripeCustomer(ctx context.Context, userID uint64, customerID string) (*model.StripeCustomer, error) {
	var existing model.StripeCustomer
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&existing).Error
	switch {
	case err == nil:
		if existing.StripeCustomerID != customerID {
			existing.StripeCustomerID = customerID
			if err := s.db.WithContext(ctx).Save(&existing).Error; err != nil {
				return nil, err
			}
		}
		return &existing, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}

	record := model.StripeCustomer{UserID: userID, StripeCustomerID: customerID}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Store) GetStripeCustomerByUserID(ctx context.Context, userID uint64) (*model.StripeCustomer, error) {
	var customer model.StripeCustomer
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &customer, nil
}

func (s *Store) CreateOrder(ctx context.Context, order *model.Order) error {
	return s.db.WithContext(ctx).Create(order).Error
}

func (s *Store) GetOrderByID(ctx context.Context, id uint64) (*model.Order, error) {
	var order model.Order
	if err := s.db.WithContext(ctx).First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

func (s *Store) GetOrderByCheckoutSessionID(ctx context.Context, sessionID string) (*model.Order, error) {
	var order model.Order
	if err := s.db.WithContext(ctx).Where("stripe_checkout_session_id = ?", sessionID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

func (s *Store) FinalizeOrderPayment(ctx context.Context, params billing.FinalizeOrderPaymentParams) (*model.Order, bool, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, false, tx.Error
	}
	defer rollbackOnPanic(tx)

	var order model.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, params.OrderID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if order.Status != model.OrderStatusPending {
		tx.Rollback()
		return &order, false, nil
	}

	order.Status = model.OrderStatusPaid
	if paymentIntentID := strings.TrimSpace(params.PaymentIntentID); paymentIntentID != "" {
		order.StripePaymentIntentID = &paymentIntentID
	}
	if customerID := strings.TrimSpace(params.CustomerID); customerID != "" {
		order.StripeCustomerID = &customerID
	}
	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		return nil, false, err
	}

	if customerID := strings.TrimSpace(params.CustomerID); customerID != "" {
		if err := upsertStripeCustomerTx(tx, order.UserID, customerID); err != nil {
			tx.Rollback()
			return nil, false, err
		}
	}

	if order.TargetAPIKeyID != nil {
		switch order.PackKind {
		case model.PackKindHostedCredits:
			if err := addCreditBalanceToAPIKeyTx(tx, *order.TargetAPIKeyID, order.CreditAmount); err != nil {
				tx.Rollback()
				return nil, false, err
			}
		default:
			if err := addPaidQuotaToAPIKeyTx(tx, *order.TargetAPIKeyID, order.QuotaAmount); err != nil {
				tx.Rollback()
				return nil, false, err
			}
		}
	}

	if params.BillingEvent != nil {
		event := *params.BillingEvent
		event.OrderID = &order.ID
		if event.ProcessedAt == nil {
			processedAt := time.Now().UTC()
			event.ProcessedAt = &processedAt
		}
		if err := tx.Create(&event).Error; err != nil {
			tx.Rollback()
			return nil, false, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, false, err
	}
	return &order, true, nil
}

func (s *Store) ListOrdersByUser(ctx context.Context, userID uint64) ([]model.Order, error) {
	var orders []model.Order
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at desc").Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func (s *Store) ListOrders(ctx context.Context) ([]model.Order, error) {
	var orders []model.Order
	if err := s.db.WithContext(ctx).Order("created_at desc").Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func (s *Store) UpdateOrder(ctx context.Context, id uint64, values map[string]any) error {
	return s.db.WithContext(ctx).Model(&model.Order{}).Where("id = ?", id).Updates(values).Error
}

func upsertStripeCustomerTx(tx *gorm.DB, userID uint64, customerID string) error {
	var existing model.StripeCustomer
	err := tx.Where("user_id = ?", userID).First(&existing).Error
	switch {
	case err == nil:
		if existing.StripeCustomerID == customerID {
			return nil
		}
		existing.StripeCustomerID = customerID
		return tx.Save(&existing).Error
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return err
	}
	return tx.Create(&model.StripeCustomer{UserID: userID, StripeCustomerID: customerID}).Error
}

func addPaidQuotaToAPIKeyTx(tx *gorm.DB, apiKeyID uint64, quotaAmount int) error {
	var key model.APIKey
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, apiKeyID).Error; err != nil {
		return err
	}
	if key.QuotaTotal == nil {
		total := 0
		key.QuotaTotal = &total
	}
	*key.QuotaTotal += quotaAmount
	return tx.Save(&key).Error
}

func addCreditBalanceToAPIKeyTx(tx *gorm.DB, apiKeyID uint64, creditAmount int) error {
	var key model.APIKey
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, apiKeyID).Error; err != nil {
		return err
	}
	key.CreditBalance += creditAmount
	key.HostedEnabled = true
	switch key.AllowedModes {
	case "", "external_only":
		key.AllowedModes = "hybrid"
	}
	if key.DefaultRuntimeMode == nil || strings.TrimSpace(*key.DefaultRuntimeMode) == "" || strings.TrimSpace(*key.DefaultRuntimeMode) == "external" {
		defaultRuntimeMode := "hosted"
		key.DefaultRuntimeMode = &defaultRuntimeMode
	}
	return tx.Save(&key).Error
}

func (s *Store) CreateBillingEvent(ctx context.Context, event *model.BillingEvent) error {
	return s.db.WithContext(ctx).Create(event).Error
}

func (s *Store) GetBillingEventByEventID(ctx context.Context, eventID string) (*model.BillingEvent, error) {
	var event model.BillingEvent
	if err := s.db.WithContext(ctx).Where("event_id = ?", eventID).First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

func (s *Store) ListBillingEvents(ctx context.Context) ([]model.BillingEvent, error) {
	var events []model.BillingEvent
	if err := s.db.WithContext(ctx).Order("created_at desc").Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) Overview(ctx context.Context) (*model.OverviewStats, error) {
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)
	stats := &model.OverviewStats{}

	if err := s.db.WithContext(ctx).Model(&model.APIKey{}).Count(&stats.TotalAPIKeys).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.APIKey{}).Where("status = ?", model.APIKeyStatusActive).Count(&stats.ActiveAPIKeys).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.APIKey{}).Where("status = ?", model.APIKeyStatusDisabled).Count(&stats.DisabledAPIKeys).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.APIKey{}).Where("expires_at IS NOT NULL AND expires_at < ?", now).Count(&stats.ExpiredAPIKeys).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.DailyFreeQuota{}).Distinct("fingerprint_hash").Count(&stats.FreeMachines).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.UsageEvent{}).Where("created_at >= ? AND charged = ?", dayAgo, true).Count(&stats.ConsumesLast24h).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.UsageEvent{}).Where("created_at >= ? AND result = ?", dayAgo, model.UsageResultBlocked).Count(&stats.BlockedLast24h).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.UsageEvent{}).Where("created_at >= ? AND charged = ?", dayAgo, false).Count(&stats.ChecksLast24h).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.Order{}).Where("created_at >= ? AND status = ?", dayAgo, model.OrderStatusPaid).Count(&stats.PaidOrdersLast24h).Error; err != nil {
		return nil, err
	}

	var paidQuotaAdded int64
	if err := s.db.WithContext(ctx).Model(&model.Order{}).Select("COALESCE(SUM(quota_amount), 0)").Where("created_at >= ? AND status = ?", dayAgo, model.OrderStatusPaid).Scan(&paidQuotaAdded).Error; err != nil {
		return nil, err
	}
	stats.PaidQuotaAddedLast24h = paidQuotaAdded

	var keys []model.APIKey
	if err := s.db.WithContext(ctx).Find(&keys).Error; err != nil {
		return nil, err
	}
	for _, key := range keys {
		stats.RemainingPaidQuota += int64(key.PaidQuotaRemaining())
	}

	return stats, nil
}

func (s *Store) SeedDemoData(ctx context.Context, defaultFreeLimit int) error {
	planCode := "basic"
	note := "demo active key"
	quotaTotal := 100
	active := model.APIKey{
		KeyHash:    "demo-hash",
		KeyPrefix:  "cop_demo",
		Status:     model.APIKeyStatusActive,
		PlanName:   "Demo Plan",
		PlanCode:   &planCode,
		Note:       &note,
		QuotaTotal: &quotaTotal,
	}
	_ = s.db.WithContext(ctx).FirstOrCreate(&active, model.APIKey{KeyHash: active.KeyHash}).Error

	quota := model.FreeQuota{FingerprintHash: "demo-fingerprint", FreeLimit: defaultFreeLimit, FreeUsed: 2}
	_ = s.db.WithContext(ctx).FirstOrCreate(&quota, model.FreeQuota{FingerprintHash: quota.FingerprintHash}).Error
	return nil
}

func (s *Store) AdminCreateAPIKey(ctx context.Context, ownerUserID *uint64, planName string, expiresAt *time.Time, note *string, planCode *string, hash, prefix, ciphertext string, quotaTotal *int) (*model.APIKey, error) {
	defaultRuntimeMode := "external"
	key := &model.APIKey{
		OwnerUserID:        ownerUserID,
		KeyHash:            hash,
		KeyPrefix:          prefix,
		KeyCiphertext:      &ciphertext,
		Status:             model.APIKeyStatusActive,
		PlanName:           planName,
		AllowedModes:       "external_only",
		HostedEnabled:      false,
		DefaultRuntimeMode: &defaultRuntimeMode,
		ExpiresAt:          expiresAt,
		Note:               note,
		PlanCode:           planCode,
		QuotaTotal:         quotaTotal,
	}
	return key, s.CreateAPIKey(ctx, key)
}

func (s *Store) AppCreateAPIKey(ctx context.Context, userID uint64, planName, hash, prefix, ciphertext string) (*model.APIKey, error) {
	defaultRuntimeMode := "external"
	key := &model.APIKey{
		OwnerUserID:        &userID,
		KeyHash:            hash,
		KeyPrefix:          prefix,
		KeyCiphertext:      &ciphertext,
		Status:             model.APIKeyStatusActive,
		PlanName:           planName,
		AllowedModes:       "external_only",
		HostedEnabled:      false,
		DefaultRuntimeMode: &defaultRuntimeMode,
	}
	return key, s.CreateAPIKey(ctx, key)
}

func (s *Store) ListAppUsageEvents(ctx context.Context, userID uint64) ([]model.UsageEvent, error) {
	var events []model.UsageEvent
	err := s.db.WithContext(ctx).
		Table("usage_events").
		Select("usage_events.*").
		Joins("JOIN api_keys ON api_keys.id = usage_events.api_key_id").
		Where("api_keys.owner_user_id = ?", userID).
		Order("usage_events.created_at desc").
		Limit(200).
		Scan(&events).Error
	return events, err
}

func (s *Store) IsAPIKeyOwnedByUser(ctx context.Context, apiKeyID, userID uint64) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.APIKey{}).Where("id = ? AND owner_user_id = ?", apiKeyID, userID).Count(&count).Error
	return count > 0, err
}

func (s *Store) CountUserAPIKeys(ctx context.Context, userID uint64) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.APIKey{}).Where("owner_user_id = ?", userID).Count(&count).Error
	return count, err
}

func rollbackOnPanic(tx *gorm.DB) {
	if r := recover(); r != nil {
		tx.Rollback()
		panic(r)
	}
}

func withRemaining(key model.APIKey) *model.APIKey {
	if key.QuotaTotal != nil {
		remaining := key.PaidQuotaRemaining()
		key.QuotaRemaining = &remaining
	}
	key.PlaintextAvailable = key.HasStoredPlaintext()
	return &key
}

func (s *Store) withEffectiveAPIKeyStatus(ctx context.Context, key model.APIKey) (*model.APIKey, error) {
	if key.OwnerUserID == nil {
		return withRemaining(key), nil
	}
	disabled, err := ownerUserDisabledWithDB(s.db.WithContext(ctx), *key.OwnerUserID)
	if err != nil {
		return nil, err
	}
	if disabled {
		key.Status = model.APIKeyStatusDisabled
	}
	return withRemaining(key), nil
}

func ownerUserDisabledTx(tx *gorm.DB, ownerUserID *uint64) (bool, error) {
	if ownerUserID == nil || *ownerUserID == 0 {
		return false, nil
	}
	return ownerUserDisabledWithDB(tx, *ownerUserID)
}

func ownerUserDisabledWithDB(db *gorm.DB, userID uint64) (bool, error) {
	var user model.User
	if err := db.Select("status").First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return false, err
	}
	return user.Status == model.UserStatusDisabled, nil
}

func JSONString(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
