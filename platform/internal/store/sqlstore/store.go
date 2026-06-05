package sqlstore

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	Modes       []string
	Result      string
	Results     []string
	Action      string
	Actions     []string
	ReasonCode  string
	Fingerprint string
	ClientIP    string
	RequestID   string
	APIKeyID    *uint64
	UserID      *uint64
	StartTime   *time.Time
	EndTime     *time.Time
}

var fingerprintSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

const fingerprintQualityDistinctValueLimit = 25

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

func (s *Store) CreateCLILoginChallenge(ctx context.Context, challenge *model.CLILoginChallenge) error {
	return s.db.WithContext(ctx).Create(challenge).Error
}

func (s *Store) GetCLILoginChallengeByChallengeID(ctx context.Context, challengeID string) (*model.CLILoginChallenge, error) {
	var challenge model.CLILoginChallenge
	if err := s.db.WithContext(ctx).Where("challenge_id = ?", challengeID).First(&challenge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &challenge, nil
}

func (s *Store) GetCLILoginChallengeByUserCodeHash(ctx context.Context, userCodeHash string) (*model.CLILoginChallenge, error) {
	var challenge model.CLILoginChallenge
	if err := s.db.WithContext(ctx).Where("user_code_hash = ?", userCodeHash).First(&challenge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &challenge, nil
}

func (s *Store) CompleteCLILoginChallenge(ctx context.Context, challengeID string, userID uint64, exchangeCodeHash string, completedAt time.Time) (*model.CLILoginChallenge, error) {
	updates := map[string]any{
		"user_id":            userID,
		"exchange_code_hash": exchangeCodeHash,
		"status":             model.CLILoginChallengeStatusCompleted,
		"completed_at":       completedAt,
	}
	if err := s.db.WithContext(ctx).Model(&model.CLILoginChallenge{}).
		Where("challenge_id = ? AND status = ?", challengeID, model.CLILoginChallengeStatusPending).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetCLILoginChallengeByChallengeID(ctx, challengeID)
}

func (s *Store) CompleteCLILoginChallengeByUserCodeHash(ctx context.Context, userCodeHash string, userID uint64, completedAt time.Time) (*model.CLILoginChallenge, error) {
	updates := map[string]any{
		"user_id":      userID,
		"status":       model.CLILoginChallengeStatusCompleted,
		"completed_at": completedAt,
	}
	if err := s.db.WithContext(ctx).Model(&model.CLILoginChallenge{}).
		Where("user_code_hash = ? AND status = ?", userCodeHash, model.CLILoginChallengeStatusPending).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetCLILoginChallengeByUserCodeHash(ctx, userCodeHash)
}

func (s *Store) ConsumeCLILoginChallenge(ctx context.Context, challengeID string, consumedAt time.Time) error {
	return s.db.WithContext(ctx).Model(&model.CLILoginChallenge{}).
		Where("challenge_id = ?", challengeID).
		Updates(map[string]any{"status": model.CLILoginChallengeStatusConsumed, "consumed_at": consumedAt}).Error
}

func (s *Store) CreateCLISession(ctx context.Context, session *model.CLISession) error {
	return s.db.WithContext(ctx).Create(session).Error
}

func (s *Store) FindCLISessionByTokenHash(ctx context.Context, tokenHash string) (*model.CLISession, error) {
	var session model.CLISession
	if err := s.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (s *Store) TouchCLISession(ctx context.Context, id uint64, usedAt time.Time) error {
	return s.db.WithContext(ctx).Model(&model.CLISession{}).Where("id = ? AND revoked_at IS NULL", id).Update("last_used_at", usedAt).Error
}

func (s *Store) RevokeCLISession(ctx context.Context, id uint64, revokedAt time.Time) error {
	return s.db.WithContext(ctx).Model(&model.CLISession{}).Where("id = ?", id).Update("revoked_at", revokedAt).Error
}

func (s *Store) RevokeCLISessionsByUser(ctx context.Context, userID uint64, revokedAt time.Time) error {
	return s.db.WithContext(ctx).Model(&model.CLISession{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", revokedAt).Error
}

func (s *Store) ListCLISessionsByUser(ctx context.Context, userID uint64) ([]model.CLISession, error) {
	var sessions []model.CLISession
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at desc").Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
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

func (s *Store) FindHostedCreditGrantByIdempotencyKey(ctx context.Context, key string) (*model.HostedCreditGrant, error) {
	var grant model.HostedCreditGrant
	if err := s.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&grant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &grant, nil
}

func (s *Store) ListHostedCreditGrants(ctx context.Context) ([]model.HostedCreditGrant, error) {
	var grants []model.HostedCreditGrant
	if err := s.db.WithContext(ctx).Order("created_at desc, id desc").Find(&grants).Error; err != nil {
		return nil, err
	}
	return grants, nil
}

func (s *Store) GrantHostedCreditsToAPIKey(ctx context.Context, apiKeyID, userID uint64, source model.HostedCreditGrantSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.HostedCreditGrant, bool, error) {
	if creditAmount <= 0 {
		return nil, false, fmt.Errorf("credit amount must be positive")
	}
	var result *model.HostedCreditGrant
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.HostedCreditGrant
		if err := tx.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err == nil {
			result = &existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var key model.APIKey
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, apiKeyID).Error; err != nil {
			return err
		}
		if key.OwnerUserID == nil || *key.OwnerUserID != userID {
			return fmt.Errorf("target api key does not belong to user")
		}
		if key.Status != model.APIKeyStatusActive {
			return fmt.Errorf("target api key is disabled")
		}

		grant := &model.HostedCreditGrant{
			UserID:         userID,
			APIKeyID:       apiKeyID,
			SourceType:     source,
			IdempotencyKey: strings.TrimSpace(idempotencyKey),
			CreditAmount:   creditAmount,
			Reason:         strings.TrimSpace(reason),
			MetadataJSON:   metadataJSON,
		}
		if grant.MetadataJSON == "" {
			grant.MetadataJSON = "{}"
		}
		if err := tx.Create(grant).Error; err != nil {
			if isDuplicateConstraintError(err) {
				if loadErr := tx.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; loadErr == nil {
					result = &existing
					return nil
				}
			}
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
		if err := tx.Save(&key).Error; err != nil {
			return err
		}
		result = grant
		created = true
		return nil
	})
	return result, created, err
}

func (s *Store) GetHostedCreditAccountByUser(ctx context.Context, userID uint64) (*model.UserHostedCreditAccount, error) {
	var account model.UserHostedCreditAccount
	if err := s.db.WithContext(ctx).First(&account, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.UserHostedCreditAccount{UserID: userID}, nil
		}
		return nil, err
	}
	return &account, nil
}

func (s *Store) GrantHostedCreditsToUser(ctx context.Context, userID uint64, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.UserHostedCreditLedger, *model.UserHostedCreditAccount, bool, error) {
	if userID == 0 {
		return nil, nil, false, fmt.Errorf("user_id is required")
	}
	if creditAmount <= 0 {
		return nil, nil, false, fmt.Errorf("credit amount must be positive")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, nil, false, fmt.Errorf("idempotency_key is required")
	}
	if strings.TrimSpace(metadataJSON) == "" {
		metadataJSON = "{}"
	}
	var ledger *model.UserHostedCreditLedger
	var account *model.UserHostedCreditAccount
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		entry, loaded, didCreate, err := grantHostedCreditsToUserTx(tx, userID, source, idempotencyKey, creditAmount, reason, metadataJSON)
		ledger = entry
		account = loaded
		created = didCreate
		return err
	})
	return ledger, account, created, err
}

func grantHostedCreditsToUserTx(tx *gorm.DB, userID uint64, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.UserHostedCreditLedger, *model.UserHostedCreditAccount, bool, error) {
	var existing model.UserHostedCreditLedger
	if err := tx.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err == nil {
		account, loadErr := lockedHostedCreditAccountTx(tx, userID)
		if loadErr != nil {
			return nil, nil, false, loadErr
		}
		return &existing, account, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, err
	}
	account, err := lockedHostedCreditAccountTx(tx, userID)
	if err != nil {
		return nil, nil, false, err
	}
	account.CreditBalance += creditAmount
	if err := tx.Save(account).Error; err != nil {
		return nil, nil, false, err
	}
	entry := &model.UserHostedCreditLedger{
		UserID:         userID,
		SourceType:     source,
		IdempotencyKey: idempotencyKey,
		CreditDelta:    creditAmount,
		Reason:         strings.TrimSpace(reason),
		MetadataJSON:   metadataJSON,
	}
	if entry.Reason == "" {
		entry.Reason = string(source)
	}
	if strings.TrimSpace(entry.MetadataJSON) == "" {
		entry.MetadataJSON = "{}"
	}
	if err := tx.Create(entry).Error; err != nil {
		if isDuplicateConstraintError(err) {
			var duplicated model.UserHostedCreditLedger
			if loadErr := tx.Where("idempotency_key = ?", idempotencyKey).First(&duplicated).Error; loadErr == nil {
				return &duplicated, account, false, nil
			}
		}
		return nil, nil, false, err
	}
	return entry, account, true, nil
}

func lockedHostedCreditAccountTx(tx *gorm.DB, userID uint64) (*model.UserHostedCreditAccount, error) {
	var account model.UserHostedCreditAccount
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&account).Error
	switch {
	case err == nil:
		return &account, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}
	account = model.UserHostedCreditAccount{UserID: userID}
	if err := tx.Create(&account).Error; err != nil {
		if isDuplicateConstraintError(err) {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&account).Error; err != nil {
				return nil, err
			}
			return &account, nil
		}
		return nil, err
	}
	return &account, nil
}

func (s *Store) GetHostedCreditAccountByFingerprint(ctx context.Context, fingerprint string) (*model.FingerprintCreditAccount, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil, fmt.Errorf("fingerprint is required")
	}
	var account model.FingerprintCreditAccount
	if err := s.db.WithContext(ctx).First(&account, "fingerprint_hash = ?", fingerprint).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.FingerprintCreditAccount{FingerprintHash: fingerprint}, nil
		}
		return nil, err
	}
	return &account, nil
}

func (s *Store) GrantHostedCreditsToFingerprint(ctx context.Context, fingerprint string, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.FingerprintCreditLedger, *model.FingerprintCreditAccount, bool, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil, nil, false, fmt.Errorf("fingerprint is required")
	}
	if creditAmount <= 0 {
		return nil, nil, false, fmt.Errorf("credit amount must be positive")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, nil, false, fmt.Errorf("idempotency_key is required")
	}
	if strings.TrimSpace(metadataJSON) == "" {
		metadataJSON = "{}"
	}
	var ledger *model.FingerprintCreditLedger
	var account *model.FingerprintCreditAccount
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		entry, loaded, didCreate, err := grantHostedCreditsToFingerprintTx(tx, fingerprint, source, idempotencyKey, creditAmount, reason, metadataJSON)
		ledger = entry
		account = loaded
		created = didCreate
		return err
	})
	return ledger, account, created, err
}

func grantHostedCreditsToFingerprintTx(tx *gorm.DB, fingerprint string, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.FingerprintCreditLedger, *model.FingerprintCreditAccount, bool, error) {
	var existing model.FingerprintCreditLedger
	if err := tx.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err == nil {
		account, loadErr := lockedFingerprintCreditAccountTx(tx, fingerprint)
		if loadErr != nil {
			return nil, nil, false, loadErr
		}
		return &existing, account, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, err
	}
	account, err := lockedFingerprintCreditAccountTx(tx, fingerprint)
	if err != nil {
		return nil, nil, false, err
	}
	account.CreditBalance += creditAmount
	if source == model.HostedCreditLedgerSourceAnonymousSignupBonus {
		account.BonusGranted = true
	}
	if err := tx.Save(account).Error; err != nil {
		return nil, nil, false, err
	}
	entry := &model.FingerprintCreditLedger{
		FingerprintHash: fingerprint,
		SourceType:      source,
		IdempotencyKey:  idempotencyKey,
		CreditDelta:     creditAmount,
		Reason:          strings.TrimSpace(reason),
		MetadataJSON:    metadataJSON,
	}
	if entry.Reason == "" {
		entry.Reason = string(source)
	}
	if strings.TrimSpace(entry.MetadataJSON) == "" {
		entry.MetadataJSON = "{}"
	}
	if err := tx.Create(entry).Error; err != nil {
		if isDuplicateConstraintError(err) {
			var duplicated model.FingerprintCreditLedger
			if loadErr := tx.Where("idempotency_key = ?", idempotencyKey).First(&duplicated).Error; loadErr == nil {
				return &duplicated, account, false, nil
			}
		}
		return nil, nil, false, err
	}
	return entry, account, true, nil
}

func lockedFingerprintCreditAccountTx(tx *gorm.DB, fingerprint string) (*model.FingerprintCreditAccount, error) {
	var account model.FingerprintCreditAccount
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("fingerprint_hash = ?", fingerprint).First(&account).Error
	switch {
	case err == nil:
		return &account, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}
	account = model.FingerprintCreditAccount{FingerprintHash: fingerprint}
	if err := tx.Create(&account).Error; err != nil {
		if isDuplicateConstraintError(err) {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("fingerprint_hash = ?", fingerprint).First(&account).Error; err != nil {
				return nil, err
			}
			return &account, nil
		}
		return nil, err
	}
	return &account, nil
}

// TransferAnonymousCreditsToUser moves the available (non-reserved) balance of
// a fingerprint credit account into the supplied user hosted credit account.
// The transfer is idempotent: invoking it twice with the same fingerprint and
// user combination yields zero additional movement. Reserved credits remain on
// the fingerprint account so in-flight settle/release calls can still complete
// against it; the account itself is preserved (balance may drop to zero) with
// migrated_to_user_id set for audit.
func (s *Store) TransferAnonymousCreditsToUser(ctx context.Context, fingerprint string, userID uint64) (int, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return 0, fmt.Errorf("fingerprint is required")
	}
	if userID == 0 {
		return 0, fmt.Errorf("user_id is required")
	}
	idemKey := fmt.Sprintf("anonymous-transfer:%s:%d", fingerprint, userID)
	transferred := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Idempotency: if an outbound ledger entry already exists, treat as no-op.
		var existing model.FingerprintCreditLedger
		if err := tx.Where("idempotency_key = ?", idemKey).First(&existing).Error; err == nil {
			return associateAnonymousUsageEventsTx(tx, fingerprint, userID)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		fpAccount, err := lockedFingerprintCreditAccountTx(tx, fingerprint)
		if err != nil {
			return err
		}
		amount := fpAccount.CreditBalance
		if amount < 0 {
			amount = 0
		}
		userAccount, err := lockedHostedCreditAccountTx(tx, userID)
		if err != nil {
			return err
		}

		uid := userID
		fpAccount.MigratedToUserID = &uid

		if amount > 0 {
			fpAccount.CreditBalance -= amount
			userAccount.CreditBalance += amount

			outEntry := &model.FingerprintCreditLedger{
				FingerprintHash: fingerprint,
				SourceType:      model.HostedCreditLedgerSourceAnonymousTransferOut,
				IdempotencyKey:  idemKey,
				CreditDelta:     -amount,
				Reason:          "anonymous credits transferred to user account",
				MetadataJSON:    fmt.Sprintf(`{"user_id":%d}`, userID),
			}
			if err := tx.Create(outEntry).Error; err != nil {
				return err
			}
			inEntry := &model.UserHostedCreditLedger{
				UserID:         userID,
				SourceType:     model.HostedCreditLedgerSourceAnonymousTransferIn,
				IdempotencyKey: idemKey,
				CreditDelta:    amount,
				Reason:         "anonymous credits merged from device",
				MetadataJSON:   fmt.Sprintf(`{"fingerprint_hash":%q}`, fingerprint),
			}
			if err := tx.Create(inEntry).Error; err != nil {
				return err
			}
			if err := tx.Save(userAccount).Error; err != nil {
				return err
			}
		}
		if err := associateAnonymousUsageEventsTx(tx, fingerprint, userID); err != nil {
			return err
		}
		if err := tx.Save(fpAccount).Error; err != nil {
			return err
		}
		transferred = amount
		return nil
	})
	return transferred, err
}

func associateAnonymousUsageEventsTx(tx *gorm.DB, fingerprint string, userID uint64) error {
	return tx.Model(&model.UsageEvent{}).
		Where("fingerprint_hash = ? AND (user_id IS NULL OR user_id = 0)", fingerprint).
		Update("user_id", userID).Error
}

// ErrHostedCreditsExhausted is returned by Precheck/Charge functions when an
// account does not have sufficient available balance to satisfy the requested
// amount.
var ErrHostedCreditsExhausted = errors.New("hosted credits exhausted")

// ChargeHostedCreditsByUser deducts credits from a user hosted credit account
// in a single committed transaction. The idempotency key is "charge:{requestID}"
// so concurrent or retried calls with the same requestID result in exactly one
// ledger row. Balance is clamped at zero (overdraft is recorded as a partial
// deduction). The usageEventID, when non-nil, is persisted on the ledger entry
// so each charge keeps an audit link back to the originating request.
func (s *Store) ChargeHostedCreditsByUser(ctx context.Context, userID uint64, requestID string, credits int, usageEventID *uint64) (*model.UserHostedCreditAccount, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	if credits < 1 {
		return nil, fmt.Errorf("credits must be positive")
	}
	idempotencyKey := "charge:" + requestID
	var account *model.UserHostedCreditAccount
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.UserHostedCreditLedger
		if err := tx.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err == nil {
			loaded, loadErr := lockedHostedCreditAccountTx(tx, userID)
			if loadErr != nil {
				return loadErr
			}
			account = loaded
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		loaded, err := lockedHostedCreditAccountTx(tx, userID)
		if err != nil {
			return err
		}
		loaded.CreditBalance -= credits
		if loaded.CreditBalance < 0 {
			loaded.CreditBalance = 0
		}
		if err := tx.Save(loaded).Error; err != nil {
			return err
		}
		entry := &model.UserHostedCreditLedger{
			UserID:         userID,
			SourceType:     model.HostedCreditLedgerSourceCharge,
			IdempotencyKey: idempotencyKey,
			CreditDelta:    -credits,
			UsageEventID:   usageEventID,
			Reason:         string(model.HostedCreditLedgerSourceCharge),
			MetadataJSON:   "{}",
		}
		if err := tx.Create(entry).Error; err != nil {
			if isDuplicateConstraintError(err) {
				account = loaded
				return nil
			}
			return err
		}
		account = loaded
		return nil
	})
	return account, err
}

// ChargeHostedCreditsByFingerprint mirrors ChargeHostedCreditsByUser for
// fingerprint-scoped accounts.
func (s *Store) ChargeHostedCreditsByFingerprint(ctx context.Context, fingerprint string, requestID string, credits int, usageEventID *uint64) (*model.FingerprintCreditAccount, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil, fmt.Errorf("fingerprint is required")
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	if credits < 1 {
		return nil, fmt.Errorf("credits must be positive")
	}
	idempotencyKey := "charge:" + requestID
	var account *model.FingerprintCreditAccount
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.FingerprintCreditLedger
		if err := tx.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err == nil {
			loaded, loadErr := lockedFingerprintCreditAccountTx(tx, fingerprint)
			if loadErr != nil {
				return loadErr
			}
			account = loaded
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		loaded, err := lockedFingerprintCreditAccountTx(tx, fingerprint)
		if err != nil {
			return err
		}
		loaded.CreditBalance -= credits
		if loaded.CreditBalance < 0 {
			loaded.CreditBalance = 0
		}
		if err := tx.Save(loaded).Error; err != nil {
			return err
		}
		entry := &model.FingerprintCreditLedger{
			FingerprintHash: fingerprint,
			SourceType:      model.HostedCreditLedgerSourceCharge,
			IdempotencyKey:  idempotencyKey,
			CreditDelta:     -credits,
			UsageEventID:    usageEventID,
			Reason:          string(model.HostedCreditLedgerSourceCharge),
			MetadataJSON:    "{}",
		}
		if err := tx.Create(entry).Error; err != nil {
			if isDuplicateConstraintError(err) {
				account = loaded
				return nil
			}
			return err
		}
		account = loaded
		return nil
	})
	return account, err
}

// ChargeAPIKeyCredits deducts credits from the hosted account owned by the
// supplied API key. Uses the "charge:{requestID}" idempotency key.
func (s *Store) ChargeAPIKeyCredits(ctx context.Context, apiKeyID uint64, requestID string, credits int, usageEventID *uint64) (*model.APIKey, error) {
	if apiKeyID == 0 {
		return nil, fmt.Errorf("api_key_id is required")
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	if credits < 1 {
		return nil, fmt.Errorf("credits must be positive")
	}
	idempotencyKey := "charge:" + requestID
	var resultKey model.APIKey
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var key model.APIKey
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, apiKeyID).Error; err != nil {
			return err
		}
		if key.OwnerUserID == nil || *key.OwnerUserID == 0 {
			return fmt.Errorf("hosted mode requires an owner user for the officecli api key")
		}
		ownerID := *key.OwnerUserID

		var existing model.UserHostedCreditLedger
		if err := tx.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err == nil {
			account, loadErr := lockedHostedCreditAccountTx(tx, ownerID)
			if loadErr != nil {
				return loadErr
			}
			key.CreditBalance = account.CreditBalance
			resultKey = key
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		account, err := lockedHostedCreditAccountTx(tx, ownerID)
		if err != nil {
			return err
		}
		account.CreditBalance -= credits
		if account.CreditBalance < 0 {
			account.CreditBalance = 0
		}
		if err := tx.Save(account).Error; err != nil {
			return err
		}
		entry := &model.UserHostedCreditLedger{
			UserID:         ownerID,
			SourceType:     model.HostedCreditLedgerSourceCharge,
			IdempotencyKey: idempotencyKey,
			CreditDelta:    -credits,
			UsageEventID:   usageEventID,
			Reason:         string(model.HostedCreditLedgerSourceCharge),
			MetadataJSON:   "{}",
		}
		if err := tx.Create(entry).Error; err != nil {
			if !isDuplicateConstraintError(err) {
				return err
			}
		}
		key.CreditBalance = account.CreditBalance
		resultKey = key
		return nil
	})
	if err != nil {
		return nil, err
	}
	return withRemaining(resultKey), nil
}

// WriteChargeFailedLedgerForUser records a compensation ledger row for a
// post-upstream-success failure path (decode error / charge transaction error).
// It does NOT modify the account balance; reconciliation jobs are responsible
// for any follow-up. Idempotency key is "charge_failed:{requestID}" so the
// same requestID cannot simultaneously appear as both "charge:" and
// "charge_failed:" in normal flow (callers must enforce mutual exclusion at
// the service layer).
func (s *Store) WriteChargeFailedLedgerForUser(ctx context.Context, userID uint64, requestID string, credits int, metadataJSON string) error {
	if userID == 0 {
		return fmt.Errorf("user_id is required")
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if strings.TrimSpace(metadataJSON) == "" {
		metadataJSON = "{}"
	}
	idempotencyKey := "charge_failed:" + requestID
	entry := &model.UserHostedCreditLedger{
		UserID:         userID,
		SourceType:     model.HostedCreditLedgerSourceChargeFailedPostUpstream,
		IdempotencyKey: idempotencyKey,
		CreditDelta:    -credits,
		Reason:         string(model.HostedCreditLedgerSourceChargeFailedPostUpstream),
		MetadataJSON:   metadataJSON,
	}
	if err := s.db.WithContext(ctx).Create(entry).Error; err != nil {
		if isDuplicateConstraintError(err) {
			return nil
		}
		return err
	}
	return nil
}

// WriteChargeFailedLedgerForFingerprint mirrors WriteChargeFailedLedgerForUser
// for fingerprint-scoped accounts.
func (s *Store) WriteChargeFailedLedgerForFingerprint(ctx context.Context, fingerprint string, requestID string, credits int, metadataJSON string) error {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return fmt.Errorf("fingerprint is required")
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if strings.TrimSpace(metadataJSON) == "" {
		metadataJSON = "{}"
	}
	idempotencyKey := "charge_failed:" + requestID
	entry := &model.FingerprintCreditLedger{
		FingerprintHash: fingerprint,
		SourceType:      model.HostedCreditLedgerSourceChargeFailedPostUpstream,
		IdempotencyKey:  idempotencyKey,
		CreditDelta:     -credits,
		Reason:          string(model.HostedCreditLedgerSourceChargeFailedPostUpstream),
		MetadataJSON:    metadataJSON,
	}
	if err := s.db.WithContext(ctx).Create(entry).Error; err != nil {
		if isDuplicateConstraintError(err) {
			return nil
		}
		return err
	}
	return nil
}

// WriteChargeFailedLedgerForAPIKey looks up the owner user of the api key and
// writes the compensation ledger under that user account.
func (s *Store) WriteChargeFailedLedgerForAPIKey(ctx context.Context, apiKeyID uint64, requestID string, credits int, metadataJSON string) error {
	if apiKeyID == 0 {
		return fmt.Errorf("api_key_id is required")
	}
	var key model.APIKey
	if err := s.db.WithContext(ctx).First(&key, apiKeyID).Error; err != nil {
		return err
	}
	if key.OwnerUserID == nil || *key.OwnerUserID == 0 {
		return fmt.Errorf("hosted mode requires an owner user for the officecli api key")
	}
	return s.WriteChargeFailedLedgerForUser(ctx, *key.OwnerUserID, requestID, credits, metadataJSON)
}

// PrecheckHostedCreditsByUser performs a read-only balance check. It returns
// ErrHostedCreditsExhausted when CreditBalance < minCredits. It does NOT lock
// the row — admission control is enforced by the Charge transaction.
func (s *Store) PrecheckHostedCreditsByUser(ctx context.Context, userID uint64, minCredits int) error {
	if userID == 0 {
		return fmt.Errorf("user_id is required")
	}
	if minCredits <= 0 {
		return nil
	}
	var account model.UserHostedCreditAccount
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrHostedCreditsExhausted
		}
		return err
	}
	if account.CreditBalance < minCredits {
		return ErrHostedCreditsExhausted
	}
	return nil
}

// PrecheckHostedCreditsByFingerprint mirrors PrecheckHostedCreditsByUser for
// fingerprint accounts.
func (s *Store) PrecheckHostedCreditsByFingerprint(ctx context.Context, fingerprint string, minCredits int) error {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return fmt.Errorf("fingerprint is required")
	}
	if minCredits <= 0 {
		return nil
	}
	var account model.FingerprintCreditAccount
	err := s.db.WithContext(ctx).Where("fingerprint_hash = ?", fingerprint).First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrHostedCreditsExhausted
		}
		return err
	}
	if account.CreditBalance < minCredits {
		return ErrHostedCreditsExhausted
	}
	return nil
}

// PrecheckAPIKeyCredits checks the owner account's balance for an api key.
func (s *Store) PrecheckAPIKeyCredits(ctx context.Context, apiKeyID uint64, minCredits int) error {
	if apiKeyID == 0 {
		return fmt.Errorf("api_key_id is required")
	}
	if minCredits <= 0 {
		return nil
	}
	var key model.APIKey
	if err := s.db.WithContext(ctx).First(&key, apiKeyID).Error; err != nil {
		return err
	}
	if key.OwnerUserID == nil || *key.OwnerUserID == 0 {
		return fmt.Errorf("hosted mode requires an owner user for the officecli api key")
	}
	return s.PrecheckHostedCreditsByUser(ctx, *key.OwnerUserID, minCredits)
}

func (s *Store) CreateUsageEvent(ctx context.Context, event *model.UsageEvent) error {
	return s.db.WithContext(ctx).Create(event).Error
}

func (s *Store) CreateOperationalEvent(ctx context.Context, event *model.OperationalEvent) error {
	return s.db.WithContext(ctx).Create(event).Error
}

func (s *Store) OperationsFunnel(ctx context.Context, windowStart, now time.Time) (*model.OperationsFunnel, error) {
	windowStart = windowStart.UTC()
	now = now.UTC()
	funnel := &model.OperationsFunnel{WindowStart: windowStart, WindowEnd: now}

	if err := s.countOperationalVisitors(ctx, windowStart, now, &funnel.Visitors); err != nil {
		return nil, err
	}
	if err := s.countOperationalEvents(ctx, windowStart, now, []string{"pricing_view"}, &funnel.PricingViews); err != nil {
		return nil, err
	}
	if err := s.countOperationalEvents(ctx, windowStart, now, []string{"cta_click", "download_click", "console_open"}, &funnel.CTAClicks); err != nil {
		return nil, err
	}
	if err := s.countOperationalEvents(ctx, windowStart, now, []string{"login_start"}, &funnel.LoginStarts); err != nil {
		return nil, err
	}
	if err := s.countOperationalEvents(ctx, windowStart, now, []string{"checkout_start"}, &funnel.CheckoutStarts); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.User{}).Where("created_at >= ? AND created_at < ?", windowStart, now).Count(&funnel.RegisteredUsers).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.Order{}).Where("created_at >= ? AND created_at < ?", windowStart, now).Count(&funnel.OrdersCreated).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.CLILoginChallenge{}).Where("created_at >= ? AND created_at < ?", windowStart, now).Count(&funnel.CLILoginStarted).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.CLILoginChallenge{}).Where("completed_at IS NOT NULL AND completed_at >= ? AND completed_at < ?", windowStart, now).Count(&funnel.CLILoginCompleted).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.CLISession{}).Where("created_at >= ? AND created_at < ?", windowStart, now).Count(&funnel.CLISessionsCreated).Error; err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.ActivatedUsers, `
		SELECT COUNT(*) FROM (
			SELECT user_id FROM cli_sessions WHERE created_at >= ? AND created_at < ? AND user_id IS NOT NULL AND user_id <> 0
			UNION
			SELECT user_id FROM usage_events WHERE created_at >= ? AND created_at < ? AND user_id IS NOT NULL AND user_id <> 0
		) activated_users
	`, windowStart, now, windowStart, now); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.ActivatedRegisteredUsers, `
		SELECT COUNT(DISTINCT users.id)
		FROM users
		WHERE users.created_at >= ? AND users.created_at < ?
			AND EXISTS (
				SELECT 1 FROM cli_sessions
				WHERE cli_sessions.user_id = users.id AND cli_sessions.created_at >= ? AND cli_sessions.created_at < ?
				UNION
				SELECT 1 FROM usage_events
				WHERE usage_events.user_id = users.id AND usage_events.created_at >= ? AND usage_events.created_at < ?
			)
	`, windowStart, now, windowStart, now, windowStart, now); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.FirstUsageUsers, `
		SELECT COUNT(*) FROM (
			SELECT identity, MIN(created_at) AS first_at
			FROM (
				SELECT CASE
					WHEN user_id IS NOT NULL AND user_id <> 0 THEN 'u:' || CAST(user_id AS TEXT)
					WHEN COALESCE(fingerprint_hash, '') <> '' AND COALESCE(mode, '') <> ? THEN 'f:' || fingerprint_hash
					ELSE NULL
				END AS identity, created_at
				FROM usage_events
			) identities
			WHERE identity IS NOT NULL
			GROUP BY identity
			HAVING MIN(created_at) >= ? AND MIN(created_at) < ?
		) first_usage
	`, string(model.UsageModeHosted), windowStart, now); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.RepeatUsageUsers, `
		SELECT COUNT(*) FROM (
			SELECT identity
			FROM (
				SELECT CASE
					WHEN user_id IS NOT NULL AND user_id <> 0 THEN 'u:' || CAST(user_id AS TEXT)
					WHEN COALESCE(fingerprint_hash, '') <> '' AND COALESCE(mode, '') <> ? THEN 'f:' || fingerprint_hash
					ELSE NULL
				END AS identity
				FROM usage_events
				WHERE created_at >= ? AND created_at < ?
			) identities
			WHERE identity IS NOT NULL
			GROUP BY identity
			HAVING COUNT(*) >= 2
		) repeat_usage
	`, string(model.UsageModeHosted), windowStart, now); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.MachineQuality.TotalMachines, `
		SELECT COUNT(DISTINCT fingerprint_hash) FROM usage_events
		WHERE COALESCE(fingerprint_hash, '') <> '' AND COALESCE(mode, '') <> ?
	`, string(model.UsageModeHosted)); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.MachineQuality.ActiveMachines24h, `
		SELECT COUNT(DISTINCT fingerprint_hash) FROM usage_events
		WHERE created_at >= ? AND created_at < ? AND COALESCE(fingerprint_hash, '') <> '' AND COALESCE(mode, '') <> ?
	`, now.Add(-24*time.Hour), now, string(model.UsageModeHosted)); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.MachineQuality.ActiveMachines7d, `
		SELECT COUNT(DISTINCT fingerprint_hash) FROM usage_events
		WHERE created_at >= ? AND created_at < ? AND COALESCE(fingerprint_hash, '') <> '' AND COALESCE(mode, '') <> ?
	`, now.AddDate(0, 0, -7), now, string(model.UsageModeHosted)); err != nil {
		return nil, err
	}

	var usageTotal int64
	var blockedTotal int64
	if err := s.db.WithContext(ctx).Model(&model.UsageEvent{}).Where("created_at >= ? AND created_at < ?", windowStart, now).Count(&usageTotal).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.UsageEvent{}).Where("created_at >= ? AND created_at < ? AND result = ?", windowStart, now, model.UsageResultBlocked).Count(&blockedTotal).Error; err != nil {
		return nil, err
	}
	funnel.BlockedRate = model.Rate(blockedTotal, usageTotal)

	if err := s.scanInt64(ctx, &funnel.PaidOrders, `
		SELECT COUNT(*) FROM orders WHERE updated_at >= ? AND updated_at < ? AND status = ?
	`, windowStart, now, string(model.OrderStatusPaid)); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.PaidUsers, `
		SELECT COUNT(DISTINCT user_id) FROM orders WHERE updated_at >= ? AND updated_at < ? AND status = ? AND user_id IS NOT NULL AND user_id <> 0
	`, windowStart, now, string(model.OrderStatusPaid)); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.PaidRegisteredUsers, `
		SELECT COUNT(DISTINCT orders.user_id)
		FROM orders
		INNER JOIN users ON users.id = orders.user_id
		WHERE orders.updated_at >= ? AND orders.updated_at < ?
			AND orders.status = ?
			AND users.created_at >= ? AND users.created_at < ?
			AND orders.user_id IS NOT NULL AND orders.user_id <> 0
	`, windowStart, now, string(model.OrderStatusPaid), windowStart, now); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.Revenue, `
		SELECT COALESCE(SUM(amount_total), 0) FROM orders WHERE updated_at >= ? AND updated_at < ? AND status = ?
	`, windowStart, now, string(model.OrderStatusPaid)); err != nil {
		return nil, err
	}
	if err := s.scanInt64(ctx, &funnel.RepeatPaidUsers, `
		SELECT COUNT(*) FROM (
			SELECT user_id
			FROM orders
			WHERE status = ? AND user_id IS NOT NULL AND user_id <> 0
			GROUP BY user_id
			HAVING COUNT(*) >= 2
				AND SUM(CASE WHEN updated_at >= ? AND updated_at < ? THEN 1 ELSE 0 END) > 0
		) repeat_paid
	`, string(model.OrderStatusPaid), windowStart, now); err != nil {
		return nil, err
	}

	funnel.ActivationRate = model.Rate(funnel.ActivatedRegisteredUsers, funnel.RegisteredUsers)
	funnel.PaidConversionRate = model.Rate(funnel.PaidRegisteredUsers, funnel.RegisteredUsers)
	funnel.CheckoutToPaidRate = model.Rate(funnel.PaidOrders, funnel.CheckoutStarts)
	funnel.UsageQuality = model.OperationsUsageQuality{
		FirstUsageUsers:  funnel.FirstUsageUsers,
		RepeatUsageUsers: funnel.RepeatUsageUsers,
		BlockedRate:      funnel.BlockedRate,
	}
	funnel.RevenueQuality = model.OperationsRevenueQuality{
		PaidOrders:          funnel.PaidOrders,
		PaidUsers:           funnel.PaidUsers,
		PaidRegisteredUsers: funnel.PaidRegisteredUsers,
		Revenue:             funnel.Revenue,
		RepeatPaidUsers:     funnel.RepeatPaidUsers,
		PaidConversionRate:  funnel.PaidConversionRate,
		CheckoutToPaidRate:  funnel.CheckoutToPaidRate,
	}
	return funnel, nil
}

func (s *Store) countOperationalVisitors(ctx context.Context, windowStart, now time.Time, dest *int64) error {
	return s.db.WithContext(ctx).Model(&model.OperationalEvent{}).
		Where("created_at >= ? AND created_at < ? AND visitor_id IS NOT NULL AND visitor_id <> ''", windowStart, now).
		Distinct("visitor_id").
		Count(dest).Error
}

func (s *Store) countOperationalEvents(ctx context.Context, windowStart, now time.Time, eventNames []string, dest *int64) error {
	return s.db.WithContext(ctx).Model(&model.OperationalEvent{}).
		Where("created_at >= ? AND created_at < ? AND event_name IN ?", windowStart, now, eventNames).
		Count(dest).Error
}

func (s *Store) scanInt64(ctx context.Context, dest *int64, query string, args ...any) error {
	return s.db.WithContext(ctx).Raw(query, args...).Scan(dest).Error
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

func (s *Store) ListImagePromptTemplates(ctx context.Context, enabledOnly bool) ([]model.ImagePromptTemplate, error) {
	query := s.db.WithContext(ctx).Model(&model.ImagePromptTemplate{})
	query = query.Where("visibility = ? OR visibility = ''", "platform_public")
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	var items []model.ImagePromptTemplate
	err := query.Order("sort_order asc, id asc").Find(&items).Error
	return items, err
}

func (s *Store) ListUserPrivateImagePromptTemplates(ctx context.Context, userID uint64) ([]model.ImagePromptTemplate, error) {
	var items []model.ImagePromptTemplate
	err := s.db.WithContext(ctx).
		Model(&model.ImagePromptTemplate{}).
		Where("visibility = ? AND owner_user_id = ?", "user_private", userID).
		Order("sort_order asc, id asc").
		Find(&items).Error
	return items, err
}

func (s *Store) GetImagePromptTemplate(ctx context.Context, id uint64) (*model.ImagePromptTemplate, error) {
	var item model.ImagePromptTemplate
	if err := s.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) CreateImagePromptTemplate(ctx context.Context, item *model.ImagePromptTemplate) error {
	return s.db.WithContext(ctx).Create(item).Error
}

func (s *Store) UpdateImagePromptTemplate(ctx context.Context, id uint64, values map[string]any) (*model.ImagePromptTemplate, error) {
	if err := s.db.WithContext(ctx).Model(&model.ImagePromptTemplate{}).Where("id = ?", id).Updates(values).Error; err != nil {
		return nil, err
	}
	return s.GetImagePromptTemplate(ctx, id)
}

func (s *Store) DeleteImagePromptTemplate(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).Delete(&model.ImagePromptTemplate{}, id).Error
}

func (s *Store) CreateImageGenerationProvenance(ctx context.Context, item *model.ImageGenerationProvenance) error {
	return s.db.WithContext(ctx).Create(item).Error
}

func (s *Store) GetImageGenerationProvenance(ctx context.Context, id uint64) (*model.ImageGenerationProvenance, error) {
	var item model.ImageGenerationProvenance
	if err := s.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) GetImageGenerationProvenanceByRequestID(ctx context.Context, requestID string) (*model.ImageGenerationProvenance, error) {
	var item model.ImageGenerationProvenance
	if err := s.db.WithContext(ctx).Where("request_id = ?", strings.TrimSpace(requestID)).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) CreateImageTemplatePublishRequest(ctx context.Context, item *model.ImageTemplatePublishRequest) error {
	return s.db.WithContext(ctx).Create(item).Error
}

func (s *Store) GetImageTemplatePublishRequest(ctx context.Context, id uint64) (*model.ImageTemplatePublishRequest, error) {
	var item model.ImageTemplatePublishRequest
	if err := s.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) UpdateImageTemplatePublishRequest(ctx context.Context, id uint64, values map[string]any) (*model.ImageTemplatePublishRequest, error) {
	if err := s.db.WithContext(ctx).Model(&model.ImageTemplatePublishRequest{}).Where("id = ?", id).Updates(values).Error; err != nil {
		return nil, err
	}
	return s.GetImageTemplatePublishRequest(ctx, id)
}

func (s *Store) ListImageTemplatePublishRequests(ctx context.Context, status string) ([]model.ImageTemplatePublishRequest, error) {
	query := s.db.WithContext(ctx).Model(&model.ImageTemplatePublishRequest{})
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(status))
	}
	var items []model.ImageTemplatePublishRequest
	err := query.Order("created_at desc, id desc").Find(&items).Error
	return items, err
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
	if values := compactStrings(filter.Modes); len(values) > 0 {
		query = query.Where("mode IN ?", values)
	} else if filter.Mode != "" {
		query = query.Where("mode = ?", filter.Mode)
	}
	if values := compactStrings(filter.Results); len(values) > 0 {
		query = query.Where("result IN ?", values)
	} else if filter.Result != "" {
		query = query.Where("result = ?", filter.Result)
	}
	if values := compactStrings(filter.Actions); len(values) > 0 {
		query = query.Where("action IN ?", values)
	} else if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.ReasonCode != "" {
		query = query.Where("reason_code = ?", filter.ReasonCode)
	}
	if filter.Fingerprint != "" {
		query = query.Where("fingerprint_hash LIKE ?", "%"+filter.Fingerprint+"%")
	}
	if filter.ClientIP != "" {
		query = query.Where("client_ip LIKE ?", "%"+filter.ClientIP+"%")
	}
	if filter.RequestID != "" {
		query = query.Where("request_id LIKE ?", "%"+filter.RequestID+"%")
	}
	if filter.APIKeyID != nil {
		query = query.Where("api_key_id = ?", *filter.APIKeyID)
	}
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
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

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

type fingerprintQualityAggregate struct {
	FingerprintHash string
	FirstAt         fingerprintQualityTime
	LastAt          fingerprintQualityTime
	Events          int64
	GenerateEvents  int64
	StatusEvents    int64
	BlockedEvents   int64
	UserBoundEvents int64
	IPCount         int64
}

type fingerprintQualityDistinctValue struct {
	FingerprintHash string
	Value           string
}

type fingerprintQualityTime struct {
	time.Time
}

func (t *fingerprintQualityTime) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		t.Time = time.Time{}
		return nil
	case time.Time:
		t.Time = v.UTC()
		return nil
	case string:
		return t.scanString(v)
	case []byte:
		return t.scanString(string(v))
	default:
		return fmt.Errorf("unsupported fingerprint quality time value %T", value)
	}
}

func (t fingerprintQualityTime) Value() (driver.Value, error) {
	return t.Time, nil
}

func (t *fingerprintQualityTime) scanString(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		t.Time = time.Time{}
		return nil
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	return fmt.Errorf("parse fingerprint quality time %q", value)
}

func (s *Store) FingerprintQuality(ctx context.Context) (*model.FingerprintQuality, error) {
	var aggregates []fingerprintQualityAggregate
	if err := s.db.WithContext(ctx).
		Model(&model.UsageEvent{}).
		Select(`
			fingerprint_hash,
			MIN(created_at) AS first_at,
			MAX(created_at) AS last_at,
			COUNT(*) AS events,
			SUM(CASE WHEN action = ? THEN 1 ELSE 0 END) AS generate_events,
			SUM(CASE WHEN action = ? THEN 1 ELSE 0 END) AS status_events,
			SUM(CASE WHEN result = ? THEN 1 ELSE 0 END) AS blocked_events,
			SUM(CASE WHEN user_id IS NOT NULL AND user_id <> 0 THEN 1 ELSE 0 END) AS user_bound_events,
			COUNT(DISTINCT NULLIF(TRIM(COALESCE(client_ip, '')), '')) AS ip_count
		`, model.UsageActionGenerate, model.UsageActionStatus, model.UsageResultBlocked).
		Group("fingerprint_hash").
		Find(&aggregates).Error; err != nil {
		return nil, err
	}

	distinctValues := map[string]map[string][]string{}
	for _, column := range []string{"client_ip", "cli_version", "runtime_mode", "document_type", "user_agent"} {
		values, err := s.fingerprintQualityDistinctValues(ctx, column)
		if err != nil {
			return nil, err
		}
		distinctValues[column] = values
	}

	rows := make([]model.FingerprintQualityRow, 0, len(aggregates))
	summary := map[string]*model.FingerprintQualityBucketSummary{}
	for _, bucket := range fingerprintQualityBuckets() {
		summary[bucket] = &model.FingerprintQualityBucketSummary{
			Bucket:          bucket,
			Reason:          fingerprintQualityBucketReason(bucket),
			DefaultFiltered: defaultFilterFingerprintQualityBucket(bucket),
		}
	}
	for _, aggregate := range aggregates {
		row := model.FingerprintQualityRow{
			FingerprintHash:   aggregate.FingerprintHash,
			FingerprintPrefix: fingerprintPrefix(aggregate.FingerprintHash),
			FirstAt:           aggregate.FirstAt.Time,
			LastAt:            aggregate.LastAt.Time,
			Events:            aggregate.Events,
			GenerateEvents:    aggregate.GenerateEvents,
			StatusEvents:      aggregate.StatusEvents,
			BlockedEvents:     aggregate.BlockedEvents,
			UserBoundEvents:   aggregate.UserBoundEvents,
			IPCount:           aggregate.IPCount,
			IPs:               distinctValues["client_ip"][aggregate.FingerprintHash],
			CLIVersions:       distinctValues["cli_version"][aggregate.FingerprintHash],
			RuntimeModes:      distinctValues["runtime_mode"][aggregate.FingerprintHash],
			DocumentTypes:     distinctValues["document_type"][aggregate.FingerprintHash],
			UserAgents:        distinctValues["user_agent"][aggregate.FingerprintHash],
		}
		row.Bucket, row.Reason = classifyFingerprintQuality(row)
		rows = append(rows, row)

		item := summary[row.Bucket]
		if item == nil {
			item = &model.FingerprintQualityBucketSummary{
				Bucket:          row.Bucket,
				Reason:          fingerprintQualityBucketReason(row.Bucket),
				DefaultFiltered: defaultFilterFingerprintQualityBucket(row.Bucket),
			}
			summary[row.Bucket] = item
		}
		item.Fingerprints++
		item.Events += row.Events
	}

	sort.Slice(rows, func(i, j int) bool {
		leftRank := fingerprintQualityBucketRank(rows[i].Bucket)
		rightRank := fingerprintQualityBucketRank(rows[j].Bucket)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return rows[i].FingerprintHash < rows[j].FingerprintHash
	})

	summaryRows := make([]model.FingerprintQualityBucketSummary, 0, len(summary))
	for _, bucket := range fingerprintQualityBuckets() {
		if item := summary[bucket]; item != nil {
			summaryRows = append(summaryRows, *item)
		}
	}

	return &model.FingerprintQuality{Summary: summaryRows, Rows: rows}, nil
}

func (s *Store) fingerprintQualityDistinctValues(ctx context.Context, column string) (map[string][]string, error) {
	if !isFingerprintQualityDistinctColumn(column) {
		return nil, fmt.Errorf("unsupported fingerprint quality distinct column %q", column)
	}

	query := fmt.Sprintf(`
		SELECT fingerprint_hash, value
		FROM (
			SELECT fingerprint_hash, value, ROW_NUMBER() OVER (PARTITION BY fingerprint_hash ORDER BY value) AS rn
			FROM (
				SELECT DISTINCT fingerprint_hash, TRIM(%s) AS value
				FROM usage_events
				WHERE TRIM(COALESCE(%s, '')) <> ''
			) distinct_values
		) ranked_values
		WHERE rn <= ?
		ORDER BY fingerprint_hash, value
	`, column, column)

	var pairs []fingerprintQualityDistinctValue
	if err := s.db.WithContext(ctx).Raw(query, fingerprintQualityDistinctValueLimit).Scan(&pairs).Error; err != nil {
		return nil, err
	}
	valuesByFingerprint := map[string][]string{}
	for _, pair := range pairs {
		valuesByFingerprint[pair.FingerprintHash] = append(valuesByFingerprint[pair.FingerprintHash], pair.Value)
	}
	return valuesByFingerprint, nil
}

func isFingerprintQualityDistinctColumn(column string) bool {
	switch column {
	case "client_ip", "cli_version", "runtime_mode", "document_type", "user_agent":
		return true
	default:
		return false
	}
}

func classifyFingerprintQuality(row model.FingerprintQualityRow) (string, string) {
	if anyNormalizedContains(row.UserAgents, "officecli-production-inspection/1.0") {
		return "internal_inspection", "production inspection agent"
	}
	if !fingerprintSHA256Pattern.MatchString(strings.ToLower(strings.TrimSpace(row.FingerprintHash))) {
		return "test_fake_fingerprint", "empty or non sha256 fingerprint hash"
	}
	if anyNormalizedHasPrefix(row.CLIVersions, "current-") {
		return "likely_docker_ci_current_build", "cli version uses current-* build marker"
	}
	if hasExactNormalized(row.CLIVersions, "dev") && hasExactNormalized(row.RuntimeModes, "hosted") && hasAllDocumentTypes(row.DocumentTypes, "docx", "xlsx") && len(normalizedSet(row.DocumentTypes)) >= 2 {
		reason := "dev cli with hosted runtime and docx/xlsx matrix"
		extras := []string{}
		if hasAnyDocumentType(row.DocumentTypes, "pptx", "report", "img") {
			extras = append(extras, "extra document profiles")
		}
		if row.GenerateEvents > 0 && row.StatusEvents > 0 {
			extras = append(extras, "generate/status mix")
		}
		if len(extras) > 0 {
			reason += "; " + strings.Join(extras, "; ")
		}
		return "likely_docker_dev_hosted_matrix", reason
	}
	if anyInternalTestVersion(row.CLIVersions) {
		return "likely_internal_test_dev_latest_local", "dev/latest/local cli version marker"
	}
	return "candidate_real_or_unknown", "no machine/test signals matched"
}

func defaultFilterFingerprintQualityBucket(bucket string) bool {
	switch bucket {
	case "internal_inspection", "test_fake_fingerprint", "likely_docker_ci_current_build", "likely_docker_dev_hosted_matrix", "likely_internal_test_dev_latest_local":
		return true
	default:
		return false
	}
}

func fingerprintQualityBuckets() []string {
	return []string{
		"internal_inspection",
		"test_fake_fingerprint",
		"likely_docker_ci_current_build",
		"likely_docker_dev_hosted_matrix",
		"likely_internal_test_dev_latest_local",
		"candidate_real_or_unknown",
	}
}

func fingerprintQualityBucketReason(bucket string) string {
	switch bucket {
	case "internal_inspection":
		return "production inspection agent"
	case "test_fake_fingerprint":
		return "empty or non sha256 fingerprint hash"
	case "likely_docker_ci_current_build":
		return "cli version uses current-* build marker"
	case "likely_docker_dev_hosted_matrix":
		return "dev cli with hosted runtime and docx/xlsx matrix"
	case "likely_internal_test_dev_latest_local":
		return "dev/latest/local cli version marker"
	default:
		return "no machine/test signals matched"
	}
}

func fingerprintQualityBucketRank(bucket string) int {
	switch bucket {
	case "internal_inspection":
		return 0
	case "test_fake_fingerprint":
		return 1
	case "likely_docker_ci_current_build":
		return 2
	case "likely_docker_dev_hosted_matrix":
		return 3
	case "likely_internal_test_dev_latest_local":
		return 4
	case "candidate_real_or_unknown":
		return 5
	default:
		return 99
	}
}

func fingerprintPrefix(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func normalized(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		if item := normalized(value); item != "" {
			out[item] = struct{}{}
		}
	}
	return out
}

func hasExactNormalized(values []string, target string) bool {
	target = normalized(target)
	for _, value := range values {
		if normalized(value) == target {
			return true
		}
	}
	return false
}

func anyNormalizedContains(values []string, needle string) bool {
	needle = normalized(needle)
	for _, value := range values {
		if strings.Contains(normalized(value), needle) {
			return true
		}
	}
	return false
}

func anyNormalizedHasPrefix(values []string, prefix string) bool {
	prefix = normalized(prefix)
	for _, value := range values {
		if strings.HasPrefix(normalized(value), prefix) {
			return true
		}
	}
	return false
}

func anyInternalTestVersion(values []string) bool {
	for _, value := range values {
		item := normalized(value)
		if item == "dev" || strings.HasPrefix(item, "latest-") || strings.HasPrefix(item, "local-") || strings.HasSuffix(item, "-local-smoke") {
			return true
		}
	}
	return false
}

func hasAllDocumentTypes(values []string, required ...string) bool {
	set := normalizedSet(values)
	for _, value := range required {
		if _, ok := set[normalized(value)]; !ok {
			return false
		}
	}
	return true
}

func hasAnyDocumentType(values []string, candidates ...string) bool {
	set := normalizedSet(values)
	for _, value := range candidates {
		if _, ok := set[normalized(value)]; ok {
			return true
		}
	}
	return false
}

func (s *Store) GetAdminUserPreference(ctx context.Context, adminEmail, pageKey string) (*model.AdminUserPreference, error) {
	adminEmail = strings.ToLower(strings.TrimSpace(adminEmail))
	pageKey = strings.TrimSpace(pageKey)
	if adminEmail == "" || pageKey == "" {
		return nil, nil
	}
	var preference model.AdminUserPreference
	err := s.db.WithContext(ctx).
		Where("admin_email = ? AND page_key = ?", adminEmail, pageKey).
		First(&preference).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &preference, nil
}

func (s *Store) UpsertAdminUserPreference(ctx context.Context, adminEmail, pageKey, preferencesJSON string) (*model.AdminUserPreference, error) {
	preference := model.AdminUserPreference{
		AdminEmail:      strings.ToLower(strings.TrimSpace(adminEmail)),
		PageKey:         strings.TrimSpace(pageKey),
		PreferencesJSON: preferencesJSON,
	}
	if preference.AdminEmail == "" || preference.PageKey == "" {
		return nil, fmt.Errorf("admin email and page key are required")
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "admin_email"}, {Name: "page_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"preferences_json", "updated_at"}),
	}).Create(&preference).Error
	if err != nil {
		return nil, err
	}
	return s.GetAdminUserPreference(ctx, preference.AdminEmail, preference.PageKey)
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

func (s *Store) SaveGoogleUser(ctx context.Context, googleSub, email, name string, avatarURL *string) (*model.User, error) {
	if strings.TrimSpace(googleSub) == "" {
		return nil, fmt.Errorf("google subject is required")
	}
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

	if existing, err := s.findUserByEmailLowered(ctx, email); err != nil {
		return nil, err
	} else if existing != nil {
		sub := googleSub
		existing.GoogleSub = &sub
		existing.Email = email
		existing.Name = name
		existing.AvatarURL = avatarURL
		if existing.InviteCode == "" {
			existing.InviteCode = buildInviteCode(existing.ID)
		}
		if err := s.db.WithContext(ctx).Save(existing).Error; err != nil {
			return nil, err
		}
		return existing, nil
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	sub := googleSub
	user = model.User{
		GoogleSub: &sub,
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

func (s *Store) SaveGitHubUser(ctx context.Context, githubSub, email, name string, avatarURL *string) (*model.User, error) {
	if strings.TrimSpace(githubSub) == "" {
		return nil, fmt.Errorf("github subject is required")
	}
	var user model.User
	err := s.db.WithContext(ctx).Where("github_sub = ?", githubSub).First(&user).Error
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

	if existing, err := s.findUserByEmailLowered(ctx, email); err != nil {
		return nil, err
	} else if existing != nil {
		sub := githubSub
		existing.GitHubSub = &sub
		existing.Email = email
		existing.Name = name
		existing.AvatarURL = avatarURL
		if existing.InviteCode == "" {
			existing.InviteCode = buildInviteCode(existing.ID)
		}
		if err := s.db.WithContext(ctx).Save(existing).Error; err != nil {
			return nil, err
		}
		return existing, nil
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer rollbackOnPanic(tx)

	sub := githubSub
	user = model.User{
		GitHubSub:  &sub,
		Email:      email,
		Name:       name,
		AvatarURL:  avatarURL,
		InviteCode: buildPendingInviteCode("github:" + githubSub),
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

func (s *Store) findUserByEmailLowered(ctx context.Context, email string) (*model.User, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return nil, nil
	}
	var user model.User
	err := s.db.WithContext(ctx).Where("LOWER(email) = ?", normalized).First(&user).Error
	if err == nil {
		return &user, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
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
	db := s.db.WithContext(ctx).
		Model(&model.User{}).
		Select("users.*, COALESCE(user_hosted_credit_accounts.credit_balance, 0) AS credit_balance").
		Joins("LEFT JOIN user_hosted_credit_accounts ON user_hosted_credit_accounts.user_id = users.id")
	normalized := strings.TrimSpace(query)
	if normalized != "" {
		if id, err := strconv.ParseUint(normalized, 10, 64); err == nil {
			db = db.Where("users.id = ?", id)
		} else {
			db = db.Where("users.email LIKE ?", "%"+normalized+"%")
		}
	}
	if err := db.Order("users.created_at desc").Find(&users).Error; err != nil {
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

	switch order.PackKind {
	case model.PackKindHostedCredits:
		if order.CreditAmount > 0 {
			if _, _, _, err := grantHostedCreditsToUserTx(tx, order.UserID, model.HostedCreditLedgerSourcePurchase, fmt.Sprintf("order:%d", order.ID), order.CreditAmount, "hosted credit purchase", JSONString(map[string]any{
				"order_id":  order.ID,
				"pack_code": order.PackCode,
			})); err != nil {
				tx.Rollback()
				return nil, false, err
			}
		}
	default:
		if order.TargetAPIKeyID != nil {
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

func (s *Store) ListUserHostedCreditLedger(ctx context.Context, userID uint64, includeZeroDelta bool) ([]model.UserHostedCreditLedger, error) {
	q := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if !includeZeroDelta {
		q = q.Where("credit_delta != 0 OR source_type IN ?", []string{
			string(model.HostedCreditLedgerSourceCharge),
			string(model.HostedCreditLedgerSourceChargeFailedPostUpstream),
		})
	}
	var entries []model.UserHostedCreditLedger
	if err := q.Order("created_at desc").Limit(500).Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) ListAllUserHostedCreditLedger(ctx context.Context, includeZeroDelta bool) ([]model.UserHostedCreditLedger, error) {
	q := s.db.WithContext(ctx)
	if !includeZeroDelta {
		q = q.Where("credit_delta != 0 OR source_type IN ?", []string{
			string(model.HostedCreditLedgerSourceCharge),
			string(model.HostedCreditLedgerSourceChargeFailedPostUpstream),
		})
	}
	var entries []model.UserHostedCreditLedger
	if err := q.Order("created_at desc").Limit(1000).Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) Overview(ctx context.Context) (*model.OverviewStats, error) {
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)
	trendStart := utcDayStart(now).AddDate(0, 0, -6)
	trendEnd := trendStart.AddDate(0, 0, 7)
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
	if err := s.db.WithContext(ctx).Model(&model.FingerprintCreditAccount{}).Distinct("fingerprint_hash").Count(&stats.FreeMachines).Error; err != nil {
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

	var usageEvents []model.UsageEvent
	if err := s.db.WithContext(ctx).
		Where("created_at >= ? AND created_at < ?", trendStart, trendEnd).
		Order("created_at asc").
		Find(&usageEvents).Error; err != nil {
		return nil, err
	}
	stats.UsageTrend = buildOverviewUsageTrend(trendStart, usageEvents)
	stats.ModeBreakdown = buildOverviewModeBreakdown(usageEvents)
	stats.ResultBreakdown = buildOverviewResultBreakdown(usageEvents)
	stats.APIKeyStatusBreakdown = buildOverviewAPIKeyStatusBreakdown(now, keys)
	funnel, err := s.OperationsFunnel(ctx, now.AddDate(0, 0, -30), now)
	if err != nil {
		return nil, err
	}
	stats.OperationsFunnel30d = funnel

	return stats, nil
}

func utcDayStart(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func buildOverviewUsageTrend(start time.Time, events []model.UsageEvent) []model.OverviewUsageTrendPoint {
	points := make([]model.OverviewUsageTrendPoint, 7)
	indexByDate := make(map[string]int, len(points))
	for i := range points {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = model.OverviewUsageTrendPoint{Date: date}
		indexByDate[date] = i
	}
	for _, event := range events {
		date := utcDayStart(event.CreatedAt).Format("2006-01-02")
		index, ok := indexByDate[date]
		if !ok {
			continue
		}
		points[index].Total++
		if event.Charged {
			points[index].Consumes++
		} else {
			points[index].Checks++
		}
		switch event.Result {
		case model.UsageResultBlocked:
			points[index].Blocked++
		case model.UsageResultAllowed:
			points[index].Allowed++
		}
	}
	return points
}

func buildOverviewModeBreakdown(events []model.UsageEvent) []model.OverviewBreakdownItem {
	items := []model.OverviewBreakdownItem{
		{Key: string(model.UsageModeFree), Label: "Free"},
		{Key: string(model.UsageModeReward), Label: "Reward"},
		{Key: string(model.UsageModePaid), Label: "Paid"},
		{Key: string(model.UsageModeHosted), Label: "Hosted"},
	}
	indexByKey := map[string]int{}
	for i := range items {
		indexByKey[items[i].Key] = i
	}
	for _, event := range events {
		if index, ok := indexByKey[string(event.Mode)]; ok {
			items[index].Value++
		}
	}
	return items
}

func buildOverviewResultBreakdown(events []model.UsageEvent) []model.OverviewBreakdownItem {
	items := []model.OverviewBreakdownItem{
		{Key: string(model.UsageResultAllowed), Label: "Allowed"},
		{Key: string(model.UsageResultBlocked), Label: "Blocked"},
	}
	indexByKey := map[string]int{}
	for i := range items {
		indexByKey[items[i].Key] = i
	}
	for _, event := range events {
		if index, ok := indexByKey[string(event.Result)]; ok {
			items[index].Value++
		}
	}
	return items
}

func buildOverviewAPIKeyStatusBreakdown(now time.Time, keys []model.APIKey) []model.OverviewBreakdownItem {
	items := []model.OverviewBreakdownItem{
		{Key: "active", Label: "Active"},
		{Key: "disabled", Label: "Disabled"},
		{Key: "expired", Label: "Expired"},
		{Key: "other", Label: "Other"},
	}
	indexByKey := map[string]int{}
	for i := range items {
		indexByKey[items[i].Key] = i
	}
	for _, key := range keys {
		bucket := "other"
		if key.ExpiresAt != nil && key.ExpiresAt.Before(now) {
			bucket = "expired"
		} else if key.Status == model.APIKeyStatusActive {
			bucket = "active"
		} else if key.Status == model.APIKeyStatusDisabled {
			bucket = "disabled"
		}
		items[indexByKey[bucket]].Value++
	}
	return items
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

	demoCredits := defaultFreeLimit
	if demoCredits <= 0 {
		demoCredits = 100
	}
	demoAccount := model.FingerprintCreditAccount{
		FingerprintHash: "demo-fingerprint",
		CreditBalance:   demoCredits,
		BonusGranted:    true,
	}
	_ = s.db.WithContext(ctx).FirstOrCreate(&demoAccount, model.FingerprintCreditAccount{FingerprintHash: demoAccount.FingerprintHash}).Error
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
		Joins("LEFT JOIN api_keys ON api_keys.id = usage_events.api_key_id").
		Where("usage_events.user_id = ? OR api_keys.owner_user_id = ?", userID, userID).
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
	var account model.UserHostedCreditAccount
	if err := s.db.WithContext(ctx).Where("user_id = ?", *key.OwnerUserID).First(&account).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else {
		key.CreditBalance = account.CreditBalance
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

func (s *Store) CreateIssueReport(ctx context.Context, report *model.IssueReport, requestIDs []string) (created *model.IssueReport, isReplay bool, err error) {
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(report).Error; err != nil {
			if isDuplicateConstraintError(err) {
				var existing model.IssueReport
				if loadErr := tx.Where("client_event_id = ?", report.ClientEventID).First(&existing).Error; loadErr != nil {
					return loadErr
				}
				created = &existing
				isReplay = true
				// Replay still may carry new request_id values (hosted-mode evidence chain
				// across retries). Attempt to append; existing pairs are skipped via the
				// uq_issue_report_request_ids_report_request unique index.
				if err := appendIssueReportRequestIDs(tx, existing.ID, requestIDs); err != nil {
					return err
				}
				return nil
			}
			return err
		}
		if err := appendIssueReportRequestIDs(tx, report.ID, requestIDs); err != nil {
			return err
		}
		created = report
		return nil
	})
	return
}

func appendIssueReportRequestIDs(tx *gorm.DB, reportID uint64, requestIDs []string) error {
	if len(requestIDs) == 0 {
		return nil
	}
	rows := make([]model.IssueReportRequestID, len(requestIDs))
	for i, rid := range requestIDs {
		rows[i] = model.IssueReportRequestID{
			ReportID:  reportID,
			RequestID: rid,
		}
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (s *Store) FindIssueReportByClientEventID(ctx context.Context, clientEventID string) (*model.IssueReport, error) {
	var report model.IssueReport
	if err := s.db.WithContext(ctx).Where("client_event_id = ?", clientEventID).First(&report).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &report, nil
}

func (s *Store) FindIssueReportByTicketID(ctx context.Context, ticketID string) (*model.IssueReport, error) {
	var report model.IssueReport
	if err := s.db.WithContext(ctx).Where("ticket_id = ?", ticketID).First(&report).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &report, nil
}

func (s *Store) ListIssueReportRequestIDs(ctx context.Context, reportID uint64) ([]model.IssueReportRequestID, error) {
	var rows []model.IssueReportRequestID
	if err := s.db.WithContext(ctx).Where("report_id = ?", reportID).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) NullifyIssueReportContactEmail(ctx context.Context, ticketID string) error {
	return s.db.WithContext(ctx).Model(&model.IssueReport{}).Where("ticket_id = ?", ticketID).Update("contact_email", nil).Error
}

func (s *Store) DeleteIssueReportsOlderThan(ctx context.Context, before time.Time) (int64, error) {
	result := s.db.WithContext(ctx).Where("created_at < ?", before).Delete(&model.IssueReport{})
	return result.RowsAffected, result.Error
}
