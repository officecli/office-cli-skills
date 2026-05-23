package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/officecli/officecli-internal/platform/internal/model"
	"github.com/officecli/officecli-internal/platform/internal/redemption"
	sqlstore "github.com/officecli/officecli-internal/platform/internal/store/sqlstore"
)

// SetRedemptionService wires in the redemption-code business service. It is
// optional in NewService so existing test harnesses that construct the admin
// service without redemption support still work; callers that need
// redemption code management must invoke this after NewService.
func (s *Service) SetRedemptionService(svc *redemption.Service) {
	if s == nil {
		return
	}
	s.redemption = svc
}

func (s *Service) redemptionService() (*redemption.Service, error) {
	if s == nil || s.redemption == nil {
		return nil, errors.New("redemption service is not configured")
	}
	return s.redemption, nil
}

func (s *Service) CreateRedemptionCode(ctx context.Context, actorEmail string, req CreateRedemptionCodeRequest) (*model.RedemptionCode, error) {
	svc, err := s.redemptionService()
	if err != nil {
		return nil, err
	}
	perUser := 1
	if req.PerUserLimit != nil {
		perUser = *req.PerUserLimit
	}
	code, err := svc.CreateCode(ctx, redemption.CreateCodeRequest{
		Code:           req.Code,
		CreditAmount:   req.CreditAmount,
		MaxRedemptions: req.MaxRedemptions,
		PerUserLimit:   perUser,
		ExpiresAt:      req.ExpiresAt,
		Notes:          req.Notes,
		CreatedBy:      actorEmail,
	})
	if err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "redemption_code.create", "redemption_code", fmt.Sprintf("%d", code.ID), sqlstore.JSONString(code))
	return code, nil
}

func (s *Service) ListRedemptionCodes(ctx context.Context, req ListRedemptionCodesRequest) (*ListRedemptionCodesResponse, error) {
	svc, err := s.redemptionService()
	if err != nil {
		return nil, err
	}
	items, total, err := svc.ListCodes(ctx, redemption.ListCodesRequest{
		Status:       req.Status,
		CodeContains: req.CodeContains,
		Limit:        req.Limit,
		Offset:       req.Offset,
	})
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.RedemptionCode{}
	}
	return &ListRedemptionCodesResponse{Items: items, Total: total}, nil
}

func (s *Service) GetRedemptionCode(ctx context.Context, id uint64) (*model.RedemptionCode, error) {
	svc, err := s.redemptionService()
	if err != nil {
		return nil, err
	}
	return svc.GetCode(ctx, id)
}

func (s *Service) UpdateRedemptionCode(ctx context.Context, actorEmail string, id uint64, req UpdateRedemptionCodeRequest) (*model.RedemptionCode, error) {
	svc, err := s.redemptionService()
	if err != nil {
		return nil, err
	}
	updated, err := svc.UpdateCode(ctx, id, redemption.UpdateCodeRequest{
		CreditAmount:   req.CreditAmount,
		MaxRedemptions: req.MaxRedemptions,
		ClearMaxLimit:  req.ClearMaxLimit,
		PerUserLimit:   req.PerUserLimit,
		ExpiresAt:      req.ExpiresAt,
		ClearExpiresAt: req.ClearExpiresAt,
		Notes:          req.Notes,
	})
	if err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "redemption_code.update", "redemption_code", fmt.Sprintf("%d", id), sqlstore.JSONString(map[string]any{
		"actor":   actorEmail,
		"updated": updated,
		"request": req,
	}))
	return updated, nil
}

func (s *Service) EnableRedemptionCode(ctx context.Context, actorEmail string, id uint64) (*model.RedemptionCode, error) {
	svc, err := s.redemptionService()
	if err != nil {
		return nil, err
	}
	code, err := svc.EnableCode(ctx, id)
	if err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "redemption_code.enable", "redemption_code", fmt.Sprintf("%d", id), sqlstore.JSONString(map[string]any{"actor": actorEmail}))
	return code, nil
}

func (s *Service) DisableRedemptionCode(ctx context.Context, actorEmail string, id uint64) (*model.RedemptionCode, error) {
	svc, err := s.redemptionService()
	if err != nil {
		return nil, err
	}
	code, err := svc.DisableCode(ctx, id)
	if err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "redemption_code.disable", "redemption_code", fmt.Sprintf("%d", id), sqlstore.JSONString(map[string]any{"actor": actorEmail}))
	return code, nil
}

func (s *Service) ListRedemptionRecords(ctx context.Context, req ListRedemptionRecordsRequest) (*ListRedemptionRecordsResponse, error) {
	svc, err := s.redemptionService()
	if err != nil {
		return nil, err
	}
	items, total, err := svc.ListRedemptions(ctx, redemption.ListRecordsRequest{
		RedemptionCodeID: req.RedemptionCodeID,
		Code:             req.Code,
		UserID:           req.UserID,
		Source:           req.Source,
		Limit:            req.Limit,
		Offset:           req.Offset,
	})
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.RedemptionCodeRedemption{}
	}
	return &ListRedemptionRecordsResponse{Items: items, Total: total}, nil
}
