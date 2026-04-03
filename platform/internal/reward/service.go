package reward

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/officecli/officecli/platform/internal/model"
)

var (
	ErrUserRequired           = errors.New("user_id is required")
	ErrSourceTypeRequired     = errors.New("source_type is required")
	ErrIdempotencyKeyRequired = errors.New("idempotency_key is required")
	ErrAmountRequired         = errors.New("amount_total must be greater than 0")
	ErrReasonRequired         = errors.New("reason is required")
	ErrQuotaExhausted         = errors.New("reward quota exhausted")
)

type Store interface {
	FindRewardGrantByIdempotencyKey(ctx context.Context, key string) (*model.RewardGrant, error)
	CreateRewardGrant(ctx context.Context, grant *model.RewardGrant) error
	ListRewardGrantsByUser(ctx context.Context, userID uint64) ([]model.RewardGrant, error)
	ConsumeRewardGrant(ctx context.Context, userID uint64) (*model.RewardGrant, int, error)
}

type Service struct {
	store Store
}

type GrantRequest struct {
	UserID         uint64
	SourceType     model.RewardSourceType
	IdempotencyKey string
	AmountTotal    int
	Reason         string
	Metadata       map[string]any
}

type Balance struct {
	UserID    uint64              `json:"user_id"`
	Remaining int                 `json:"remaining"`
	Grants    []model.RewardGrant `json:"grants"`
}

type ConsumeResult struct {
	Grant     *model.RewardGrant `json:"grant,omitempty"`
	Remaining int                `json:"remaining"`
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Grant(ctx context.Context, req GrantRequest) (*model.RewardGrant, bool, error) {
	if s == nil || s.store == nil {
		return nil, false, fmt.Errorf("reward store is unavailable")
	}
	if req.UserID == 0 {
		return nil, false, ErrUserRequired
	}
	if strings.TrimSpace(string(req.SourceType)) == "" {
		return nil, false, ErrSourceTypeRequired
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return nil, false, ErrIdempotencyKeyRequired
	}
	if req.AmountTotal <= 0 {
		return nil, false, ErrAmountRequired
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, false, ErrReasonRequired
	}

	existing, err := s.store.FindRewardGrantByIdempotencyKey(ctx, strings.TrimSpace(req.IdempotencyKey))
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}

	metadataJSON := "{}"
	if len(req.Metadata) > 0 {
		raw, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, false, err
		}
		metadataJSON = string(raw)
	}

	grant := &model.RewardGrant{
		UserID:         req.UserID,
		SourceType:     req.SourceType,
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		AmountTotal:    req.AmountTotal,
		Reason:         strings.TrimSpace(req.Reason),
		MetadataJSON:   metadataJSON,
	}
	if err := s.store.CreateRewardGrant(ctx, grant); err != nil {
		return nil, false, err
	}
	return grant, true, nil
}

func (s *Service) Balance(ctx context.Context, userID uint64) (*Balance, error) {
	if s == nil || s.store == nil {
		return &Balance{UserID: userID}, nil
	}
	if userID == 0 {
		return nil, ErrUserRequired
	}
	grants, err := s.store.ListRewardGrantsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	balance := &Balance{UserID: userID, Grants: grants}
	for _, grant := range grants {
		balance.Remaining += grant.Remaining()
	}
	return balance, nil
}

func (s *Service) Consume(ctx context.Context, userID uint64) (*ConsumeResult, error) {
	if s == nil || s.store == nil {
		return nil, ErrQuotaExhausted
	}
	if userID == 0 {
		return nil, ErrUserRequired
	}
	grant, remaining, err := s.store.ConsumeRewardGrant(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &ConsumeResult{Grant: grant, Remaining: remaining}, nil
}
