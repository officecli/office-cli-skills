// Package redemption implements the business logic for redemption codes that
// grant hosted credits to platform users. Admin operations (create / list /
// update / enable / disable) and the user redemption flow are both routed
// through the same Service so audit trails, validation, and idempotency
// guarantees stay in one place.
package redemption

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/officecli/officecli-internal/platform/internal/model"
	"github.com/officecli/officecli-internal/platform/internal/store/sqlstore"
)

// Store is the storage surface this service depends on. It is satisfied by
// *sqlstore.Store but kept as an interface so unit tests can substitute a
// fake.
type Store interface {
	CreateRedemptionCode(ctx context.Context, code *model.RedemptionCode) error
	GetRedemptionCodeByID(ctx context.Context, id uint64) (*model.RedemptionCode, error)
	GetRedemptionCodeByCode(ctx context.Context, code string) (*model.RedemptionCode, error)
	ListRedemptionCodes(ctx context.Context, filter sqlstore.RedemptionCodeFilter) ([]model.RedemptionCode, int64, error)
	UpdateRedemptionCode(ctx context.Context, id uint64, updates sqlstore.RedemptionCodeUpdates) (*model.RedemptionCode, error)
	SetRedemptionCodeStatus(ctx context.Context, id uint64, status model.RedemptionCodeStatus) (*model.RedemptionCode, error)
	ListRedemptionRecords(ctx context.Context, filter sqlstore.RedemptionRecordFilter) ([]model.RedemptionCodeRedemption, int64, error)
	RedeemCode(ctx context.Context, rc sqlstore.RedeemContext) (*sqlstore.RedeemResult, error)
}

// Service is the redemption-code orchestrator. It validates input, normalises
// codes, generates codes when not supplied, and surfaces typed errors that
// HTTP handlers can map to user-facing codes.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// Error variants exposed to handlers.
var (
	ErrCodeRequired         = errors.New("code is required")
	ErrCreditAmountRequired = errors.New("credit_amount must be greater than zero")
	ErrInvalidStatus        = errors.New("invalid status")
	ErrCodeNotFound         = sqlstore.ErrRedemptionCodeNotFound
	ErrCodeDisabled         = sqlstore.ErrRedemptionCodeDisabled
	ErrCodeExpired          = sqlstore.ErrRedemptionCodeExpired
	ErrCodeExhausted        = sqlstore.ErrRedemptionCodeExhausted
	ErrCodeAlreadyClaimed   = sqlstore.ErrRedemptionCodeAlreadyClaimed
)

const (
	defaultGeneratedCodeLength = 16
	maxCodeLength              = 64
	minCodeLength              = 4
)

// CreateCodeRequest captures the inputs admin endpoints supply when creating
// a new redemption code. Code may be empty -- a random 16-char code is
// generated when so.
type CreateCodeRequest struct {
	Code           string
	CreditAmount   int
	MaxRedemptions *int
	PerUserLimit   int
	ExpiresAt      *time.Time
	Notes          string
	CreatedBy      string
}

// UpdateCodeRequest captures the fields an admin may edit. Pointers indicate
// "leave unchanged" vs "set to value". ClearMaxLimit / ClearExpiresAt switch
// from "set" semantics to "make NULL" (long-term / unlimited).
type UpdateCodeRequest struct {
	CreditAmount   *int
	MaxRedemptions *int
	ClearMaxLimit  bool
	PerUserLimit   *int
	ExpiresAt      *time.Time
	ClearExpiresAt bool
	Notes          *string
}

// ListCodesRequest is the filter shape for admin listing.
type ListCodesRequest struct {
	Status       string
	CodeContains string
	Limit        int
	Offset       int
}

// ListRecordsRequest is the filter shape for admin redemption record listing.
type ListRecordsRequest struct {
	RedemptionCodeID uint64
	Code             string
	UserID           uint64
	Source           string
	Limit            int
	Offset           int
}

// RedeemRequest is what handlers pass when a user submits a code.
type RedeemRequest struct {
	UserID    uint64
	Code      string
	Source    string
	ClientIP  string
	UserAgent string
}

// RedeemResponse is the public-facing result of a successful redemption.
type RedeemResponse struct {
	Code         string     `json:"code"`
	CreditAmount int        `json:"credit_amount"`
	NewBalance   int        `json:"new_balance"`
	RedeemedAt   time.Time  `json:"redeemed_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// CreateCode validates the request and inserts a new code. New codes default
// to status=disabled so admins must explicitly enable them.
func (s *Service) CreateCode(ctx context.Context, req CreateCodeRequest) (*model.RedemptionCode, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("redemption service is unavailable")
	}
	if req.CreditAmount <= 0 {
		return nil, ErrCreditAmountRequired
	}
	if req.PerUserLimit < 1 {
		req.PerUserLimit = 1
	}
	if req.MaxRedemptions != nil && *req.MaxRedemptions <= 0 {
		return nil, fmt.Errorf("max_redemptions must be positive when set")
	}
	code := normalizeCode(req.Code)
	if code == "" {
		generated, err := generateCode(defaultGeneratedCodeLength)
		if err != nil {
			return nil, err
		}
		code = generated
	}
	if !isValidCodeShape(code) {
		return nil, fmt.Errorf("code must be %d-%d chars, alphanumeric or dash/underscore", minCodeLength, maxCodeLength)
	}
	row := &model.RedemptionCode{
		Code:           code,
		CreditAmount:   req.CreditAmount,
		MaxRedemptions: req.MaxRedemptions,
		PerUserLimit:   req.PerUserLimit,
		Status:         model.RedemptionCodeStatusDisabled,
		ExpiresAt:      req.ExpiresAt,
		Notes:          strings.TrimSpace(req.Notes),
		CreatedBy:      strings.TrimSpace(req.CreatedBy),
	}
	if err := s.store.CreateRedemptionCode(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) GetCode(ctx context.Context, id uint64) (*model.RedemptionCode, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("redemption service is unavailable")
	}
	return s.store.GetRedemptionCodeByID(ctx, id)
}

func (s *Service) ListCodes(ctx context.Context, req ListCodesRequest) ([]model.RedemptionCode, int64, error) {
	if s == nil || s.store == nil {
		return nil, 0, fmt.Errorf("redemption service is unavailable")
	}
	filter := sqlstore.RedemptionCodeFilter{
		CodeContains: req.CodeContains,
		Limit:        req.Limit,
		Offset:       req.Offset,
	}
	if req.Status != "" {
		status := model.RedemptionCodeStatus(req.Status)
		if status != model.RedemptionCodeStatusEnabled && status != model.RedemptionCodeStatusDisabled {
			return nil, 0, ErrInvalidStatus
		}
		filter.Status = status
	}
	return s.store.ListRedemptionCodes(ctx, filter)
}

func (s *Service) UpdateCode(ctx context.Context, id uint64, req UpdateCodeRequest) (*model.RedemptionCode, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("redemption service is unavailable")
	}
	updates := sqlstore.RedemptionCodeUpdates{
		CreditAmount:   req.CreditAmount,
		MaxRedemptions: req.MaxRedemptions,
		ClearMaxLimit:  req.ClearMaxLimit,
		PerUserLimit:   req.PerUserLimit,
		ExpiresAt:      req.ExpiresAt,
		ClearExpiresAt: req.ClearExpiresAt,
		Notes:          req.Notes,
	}
	return s.store.UpdateRedemptionCode(ctx, id, updates)
}

func (s *Service) EnableCode(ctx context.Context, id uint64) (*model.RedemptionCode, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("redemption service is unavailable")
	}
	return s.store.SetRedemptionCodeStatus(ctx, id, model.RedemptionCodeStatusEnabled)
}

func (s *Service) DisableCode(ctx context.Context, id uint64) (*model.RedemptionCode, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("redemption service is unavailable")
	}
	return s.store.SetRedemptionCodeStatus(ctx, id, model.RedemptionCodeStatusDisabled)
}

func (s *Service) ListRedemptions(ctx context.Context, req ListRecordsRequest) ([]model.RedemptionCodeRedemption, int64, error) {
	if s == nil || s.store == nil {
		return nil, 0, fmt.Errorf("redemption service is unavailable")
	}
	filter := sqlstore.RedemptionRecordFilter{
		RedemptionCodeID: req.RedemptionCodeID,
		Code:             strings.TrimSpace(req.Code),
		UserID:           req.UserID,
		Limit:            req.Limit,
		Offset:           req.Offset,
	}
	if src := strings.TrimSpace(req.Source); src != "" {
		source := model.RedemptionSource(src)
		if !source.Valid() {
			return nil, 0, fmt.Errorf("invalid source")
		}
		filter.Source = source
	}
	return s.store.ListRedemptionRecords(ctx, filter)
}

// Redeem executes the user-side claim. Returns typed errors mapped 1:1 to
// API error codes (code_not_found / code_disabled / code_expired /
// code_exhausted / code_already_claimed).
func (s *Service) Redeem(ctx context.Context, req RedeemRequest) (*RedeemResponse, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("redemption service is unavailable")
	}
	if req.UserID == 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	code := normalizeCode(req.Code)
	if code == "" {
		return nil, ErrCodeRequired
	}
	source := model.RedemptionSource(strings.TrimSpace(req.Source))
	if source == "" {
		source = model.RedemptionSourceApp
	}
	if !source.Valid() {
		return nil, fmt.Errorf("invalid source")
	}
	res, err := s.store.RedeemCode(ctx, sqlstore.RedeemContext{
		UserID:    req.UserID,
		Code:      code,
		Source:    source,
		ClientIP:  req.ClientIP,
		UserAgent: req.UserAgent,
		Now:       time.Now(),
	})
	if err != nil {
		return nil, err
	}
	resp := &RedeemResponse{
		Code:         res.Code.Code,
		CreditAmount: res.CreditAmount,
		RedeemedAt:   res.Redemption.RedeemedAt,
		ExpiresAt:    res.Code.ExpiresAt,
	}
	if res.Account != nil {
		resp.NewBalance = res.Account.AvailableCredits()
	}
	return resp, nil
}

// normalizeCode standardises codes for storage and lookup. Codes are case-
// insensitive (stored uppercase) and surrounding whitespace is stripped.
func normalizeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// isValidCodeShape rejects codes outside the [4..64] char range or containing
// disallowed characters. Allowed: A-Z, 0-9, '-', '_'.
func isValidCodeShape(s string) bool {
	if len(s) < minCodeLength || len(s) > maxCodeLength {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// generateCode returns a random code drawn from a confusion-resistant
// alphabet (excludes I, O, 0, 1). It uses crypto/rand for unbiased output.
func generateCode(n int) (string, error) {
	if n <= 0 {
		n = defaultGeneratedCodeLength
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return string(out), nil
}
