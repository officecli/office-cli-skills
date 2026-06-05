package sqlstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/officecli/officecli/platform/internal/model"
)

// Domain-specific errors surfaced by redemption code operations. These map
// cleanly to user-facing error codes returned by the API handlers.
var (
	ErrRedemptionCodeNotFound       = errors.New("redemption_code_not_found")
	ErrRedemptionCodeDisabled       = errors.New("redemption_code_disabled")
	ErrRedemptionCodeExpired        = errors.New("redemption_code_expired")
	ErrRedemptionCodeExhausted      = errors.New("redemption_code_exhausted")
	ErrRedemptionCodeAlreadyClaimed = errors.New("redemption_code_already_claimed")
)

// RedemptionCodeFilter captures the query parameters supported by
// ListRedemptionCodes for admin search/filter use cases.
type RedemptionCodeFilter struct {
	Status       model.RedemptionCodeStatus
	CodeContains string
	Limit        int
	Offset       int
}

// RedemptionRecordFilter captures the query parameters supported by
// ListRedemptionRecords. All fields are optional.
type RedemptionRecordFilter struct {
	RedemptionCodeID uint64
	Code             string
	UserID           uint64
	Source           model.RedemptionSource
	Limit            int
	Offset           int
}

// RedeemContext is the request-side info carried into a redemption attempt.
type RedeemContext struct {
	UserID    uint64
	Code      string
	Source    model.RedemptionSource
	ClientIP  string
	UserAgent string
	Now       time.Time
}

// RedeemResult is what the store returns on a successful redemption.
type RedeemResult struct {
	Code         *model.RedemptionCode
	Redemption   *model.RedemptionCodeRedemption
	LedgerEntry  *model.UserHostedCreditLedger
	Account      *model.UserHostedCreditAccount
	CreditAmount int
}

func (s *Store) CreateRedemptionCode(ctx context.Context, code *model.RedemptionCode) error {
	if code == nil {
		return fmt.Errorf("redemption code is required")
	}
	code.Code = strings.TrimSpace(code.Code)
	if code.Code == "" {
		return fmt.Errorf("code is required")
	}
	if code.CreditAmount <= 0 {
		return fmt.Errorf("credit_amount must be positive")
	}
	if code.PerUserLimit < 1 {
		code.PerUserLimit = 1
	}
	if code.MaxRedemptions != nil && *code.MaxRedemptions <= 0 {
		return fmt.Errorf("max_redemptions must be positive when set")
	}
	if code.Status == "" {
		code.Status = model.RedemptionCodeStatusDisabled
	}
	if err := s.db.WithContext(ctx).Create(code).Error; err != nil {
		if isDuplicateConstraintError(err) {
			return fmt.Errorf("redemption code already exists")
		}
		return err
	}
	return nil
}

func (s *Store) GetRedemptionCodeByID(ctx context.Context, id uint64) (*model.RedemptionCode, error) {
	var c model.RedemptionCode
	if err := s.db.WithContext(ctx).First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRedemptionCodeNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (s *Store) GetRedemptionCodeByCode(ctx context.Context, code string) (*model.RedemptionCode, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, ErrRedemptionCodeNotFound
	}
	var c model.RedemptionCode
	if err := s.db.WithContext(ctx).Where("code = ?", code).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRedemptionCodeNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListRedemptionCodes(ctx context.Context, filter RedemptionCodeFilter) ([]model.RedemptionCode, int64, error) {
	q := s.db.WithContext(ctx).Model(&model.RedemptionCode{})
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if pattern := strings.TrimSpace(filter.CodeContains); pattern != "" {
		q = q.Where("code ILIKE ?", "%"+pattern+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var items []model.RedemptionCode
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// UpdateRedemptionCodeFields applies admin-supplied edits to an existing code.
// Nil-valued pointers indicate "leave unchanged". Returns the refreshed row.
type RedemptionCodeUpdates struct {
	CreditAmount   *int
	MaxRedemptions *int // *int with explicit nil-vs-int distinction
	ClearMaxLimit  bool // when true, sets max_redemptions to NULL
	PerUserLimit   *int
	ExpiresAt      *time.Time
	ClearExpiresAt bool
	Notes          *string
}

func (s *Store) UpdateRedemptionCode(ctx context.Context, id uint64, updates RedemptionCodeUpdates) (*model.RedemptionCode, error) {
	if id == 0 {
		return nil, fmt.Errorf("id is required")
	}
	updateMap := map[string]any{}
	if updates.CreditAmount != nil {
		if *updates.CreditAmount <= 0 {
			return nil, fmt.Errorf("credit_amount must be positive")
		}
		updateMap["credit_amount"] = *updates.CreditAmount
	}
	if updates.ClearMaxLimit {
		updateMap["max_redemptions"] = nil
	} else if updates.MaxRedemptions != nil {
		if *updates.MaxRedemptions <= 0 {
			return nil, fmt.Errorf("max_redemptions must be positive when set")
		}
		updateMap["max_redemptions"] = *updates.MaxRedemptions
	}
	if updates.PerUserLimit != nil {
		if *updates.PerUserLimit < 1 {
			return nil, fmt.Errorf("per_user_limit must be at least 1")
		}
		updateMap["per_user_limit"] = *updates.PerUserLimit
	}
	if updates.ClearExpiresAt {
		updateMap["expires_at"] = nil
	} else if updates.ExpiresAt != nil {
		updateMap["expires_at"] = *updates.ExpiresAt
	}
	if updates.Notes != nil {
		updateMap["notes"] = strings.TrimSpace(*updates.Notes)
	}
	if len(updateMap) == 0 {
		return s.GetRedemptionCodeByID(ctx, id)
	}
	updateMap["updated_at"] = time.Now()
	res := s.db.WithContext(ctx).Model(&model.RedemptionCode{}).Where("id = ?", id).Updates(updateMap)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrRedemptionCodeNotFound
	}
	return s.GetRedemptionCodeByID(ctx, id)
}

func (s *Store) SetRedemptionCodeStatus(ctx context.Context, id uint64, status model.RedemptionCodeStatus) (*model.RedemptionCode, error) {
	if status != model.RedemptionCodeStatusEnabled && status != model.RedemptionCodeStatusDisabled {
		return nil, fmt.Errorf("invalid status")
	}
	res := s.db.WithContext(ctx).Model(&model.RedemptionCode{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "updated_at": time.Now()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrRedemptionCodeNotFound
	}
	return s.GetRedemptionCodeByID(ctx, id)
}

func (s *Store) DeleteRedemptionCode(ctx context.Context, id uint64) error {
	if id == 0 {
		return fmt.Errorf("id is required")
	}
	res := s.db.WithContext(ctx).Delete(&model.RedemptionCode{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRedemptionCodeNotFound
	}
	return nil
}

func (s *Store) ListRedemptionRecords(ctx context.Context, filter RedemptionRecordFilter) ([]model.RedemptionCodeRedemption, int64, error) {
	q := s.db.WithContext(ctx).Model(&model.RedemptionCodeRedemption{})
	if filter.RedemptionCodeID != 0 {
		q = q.Where("redemption_code_id = ?", filter.RedemptionCodeID)
	}
	if code := strings.TrimSpace(filter.Code); code != "" {
		q = q.Where("code = ?", code)
	}
	if filter.UserID != 0 {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if filter.Source != "" {
		q = q.Where("source = ?", filter.Source)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var items []model.RedemptionCodeRedemption
	if err := q.Order("redeemed_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// RedeemCode applies the redemption inside a single transaction. It locks the
// redemption_codes row FOR UPDATE, validates state, grants hosted credits via
// the existing primitive, and records a redemption row keyed by an
// idempotency key derived from (code_id, user_id, redemptions_used_at_time).
//
// On already-claimed-per-user the per_user_limit branch returns
// ErrRedemptionCodeAlreadyClaimed; on duplicate idempotency-key insert the
// existing redemption row is loaded and returned as a no-op success.
func (s *Store) RedeemCode(ctx context.Context, rc RedeemContext) (*RedeemResult, error) {
	if rc.UserID == 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	rc.Code = strings.TrimSpace(rc.Code)
	if rc.Code == "" {
		return nil, ErrRedemptionCodeNotFound
	}
	if rc.Source == "" {
		rc.Source = model.RedemptionSourceApp
	}
	if !rc.Source.Valid() {
		return nil, fmt.Errorf("invalid source")
	}
	now := rc.Now
	if now.IsZero() {
		now = time.Now()
	}

	var result *RedeemResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var code model.RedemptionCode
		// Concurrency contract:
		//
		//   The SELECT ... FOR UPDATE here is the *only* serialization point for
		//   redemption attempts against this code. Once a goroutine acquires the
		//   row lock, every other attempt on the same code blocks until this
		//   transaction commits, regardless of which user is redeeming.
		//
		//   Two invariants ride on that lock:
		//
		//   1. `userClaimCount` (computed below from a COUNT of existing
		//      redemptions for this user+code) is effectively a monotonic
		//      per-user sequence: concurrent same-user attempts cannot observe
		//      the same count because the second attempt only sees the row
		//      after the first one's INSERT + commit becomes visible. So the
		//      derived idempotency key `redemption:<code_id>:<user_id>:<n>`
		//      is unique per successful attempt without needing a separate
		//      sequence generator.
		//
		//   2. `redemptions_used` only increments under the same lock, so
		//      max_redemptions checks never race with each other.
		//
		//   The UNIQUE(idempotency_key) index on user_hosted_credit_ledger and
		//   redemption_code_redemptions is a backstop -- if the FOR UPDATE
		//   contract is ever broken (e.g. by a future refactor), duplicate
		//   inserts will fail loudly rather than silently double-grant credits.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("code = ?", rc.Code).First(&code).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRedemptionCodeNotFound
			}
			return err
		}
		if !code.IsEnabled() {
			return ErrRedemptionCodeDisabled
		}
		if code.IsExpired(now) {
			return ErrRedemptionCodeExpired
		}
		if code.IsExhausted() {
			return ErrRedemptionCodeExhausted
		}

		var userClaimCount int64
		if err := tx.Model(&model.RedemptionCodeRedemption{}).
			Where("redemption_code_id = ? AND user_id = ?", code.ID, rc.UserID).
			Count(&userClaimCount).Error; err != nil {
			return err
		}
		if int(userClaimCount) >= code.PerUserLimit {
			return ErrRedemptionCodeAlreadyClaimed
		}

		idempotencyKey := fmt.Sprintf("redemption:%d:%d:%d", code.ID, rc.UserID, userClaimCount)
		ledger, account, _, err := grantHostedCreditsToUserTx(
			tx,
			rc.UserID,
			model.HostedCreditLedgerSourceRedemptionCode,
			"redemption-code:"+idempotencyKey,
			code.CreditAmount,
			"redemption_code:"+code.Code,
			fmt.Sprintf(`{"redemption_code_id":%d,"code":%q,"source":%q}`, code.ID, code.Code, string(rc.Source)),
		)
		if err != nil {
			return err
		}

		redemption := &model.RedemptionCodeRedemption{
			RedemptionCodeID:    code.ID,
			Code:                code.Code,
			UserID:              rc.UserID,
			CreditAmount:        code.CreditAmount,
			LedgerEntryID:       ledger.ID,
			Source:              rc.Source,
			ClientIP:            truncate(rc.ClientIP, 64),
			UserAgent:           truncate(rc.UserAgent, 512),
			IdempotencyKey:      idempotencyKey,
			PerUserLimitAtClaim: code.PerUserLimit,
			RedeemedAt:          now,
		}
		if err := tx.Create(redemption).Error; err != nil {
			if isDuplicateConstraintError(err) {
				var existing model.RedemptionCodeRedemption
				if loadErr := tx.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; loadErr == nil {
					result = &RedeemResult{
						Code:         &code,
						Redemption:   &existing,
						LedgerEntry:  ledger,
						Account:      account,
						CreditAmount: code.CreditAmount,
					}
					return nil
				}
			}
			return err
		}

		if err := tx.Model(&model.RedemptionCode{}).
			Where("id = ?", code.ID).
			UpdateColumn("redemptions_used", gorm.Expr("redemptions_used + 1")).Error; err != nil {
			return err
		}
		code.RedemptionsUsed++

		result = &RedeemResult{
			Code:         &code,
			Redemption:   redemption,
			LedgerEntry:  ledger,
			Account:      account,
			CreditAmount: code.CreditAmount,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
