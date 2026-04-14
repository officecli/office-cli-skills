package license

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
	rewardsvc "github.com/officecli/officecli/platform/internal/reward"
)

type APIKeyStore interface {
	FindByHash(ctx context.Context, hash string) (*model.APIKey, error)
	TouchLastUsedAt(ctx context.Context, id uint64, usedAt time.Time) error
	ConsumePaidByHash(ctx context.Context, hash string) (*model.APIKey, error)
}

type FreeQuotaStore interface {
	GetOrCreateByFingerprint(ctx context.Context, fingerprint string, usageDate string, defaultLimit int) (*model.DailyFreeQuota, bool, error)
	GetByFingerprint(ctx context.Context, fingerprint string, usageDate string) (*model.DailyFreeQuota, error)
	Consume(ctx context.Context, fingerprint string, usageDate string, defaultLimit int) (*model.DailyFreeQuota, error)
}

type UsageEventStore interface {
	Create(ctx context.Context, event *model.UsageEvent) error
	FindByRequestID(ctx context.Context, requestID string) (*model.UsageEvent, error)
}

type IdempotencyStore interface {
	GetConsumeResult(ctx context.Context, requestID string) (*ConsumeResponse, error)
	SaveConsumeResult(ctx context.Context, requestID string, resp *ConsumeResponse, ttl time.Duration) error
	AcquireConsumeLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	ReleaseConsumeLock(ctx context.Context, key string) error
}

type RewardManager interface {
	Balance(ctx context.Context, userID uint64) (*rewardsvc.Balance, error)
	Consume(ctx context.Context, userID uint64) (*rewardsvc.ConsumeResult, error)
}

type ReferralActivator interface {
	ActivateReferral(ctx context.Context, invitedUserID uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error)
}

type Service struct {
	apiKeys          APIKeyStore
	freeQuotas       FreeQuotaStore
	usage            UsageEventStore
	idem             IdempotencyStore
	rewards          RewardManager
	referrals        ReferralActivator
	salt             string
	defaultFreeLimit int
	idemTTL          time.Duration
	clock            func() time.Time
	proof            *proofSigner
}

func NewService(apiKeys APIKeyStore, freeQuotas FreeQuotaStore, usage UsageEventStore, idem IdempotencyStore, rewards RewardManager, referrals ReferralActivator, salt string, defaultFreeLimit int, idemTTL time.Duration, proofConfigs ...ProofConfig) *Service {
	proofCfg := ProofConfig{}
	if len(proofConfigs) > 0 {
		proofCfg = proofConfigs[0]
	}
	signer, err := newProofSigner(proofCfg)
	if err != nil {
		panic(err)
	}
	service := &Service{
		apiKeys:          apiKeys,
		freeQuotas:       freeQuotas,
		usage:            usage,
		idem:             idem,
		rewards:          rewards,
		referrals:        referrals,
		salt:             salt,
		defaultFreeLimit: defaultFreeLimit,
		idemTTL:          idemTTL,
		clock:            time.Now,
		proof:            signer,
	}
	service.proof.clock = func() time.Time {
		if service.clock != nil {
			return service.clock()
		}
		return time.Now()
	}
	return service
}

func (s *Service) Check(ctx context.Context, req CheckRequest) (*CheckResponse, error) {
	if strings.TrimSpace(req.FingerprintHash) == "" {
		return nil, ErrFingerprintRequired
	}
	if strings.TrimSpace(req.RequestNonce) == "" {
		return nil, ErrRequestNonceRequired
	}
	if req.Action != string(model.UsageActionGenerate) && req.Action != string(model.UsageActionStatus) {
		return nil, ErrInvalidAction
	}
	if strings.TrimSpace(req.APIKey) == "" && strings.TrimSpace(req.RuntimeMode) == "hosted" {
		return &CheckResponse{
			Allowed:             false,
			AccessMode:          model.AccessModeBlocked,
			ReasonCode:          "hosted_requires_api_key",
			Message:             "Hosted mode requires a valid officecli API key to be configured first.",
			AllowedModes:        []string{"external"},
			SelectedRuntimeMode: "hosted",
		}, nil
	}
	if strings.TrimSpace(req.APIKey) != "" {
		return s.checkPaid(ctx, req)
	}
	if rewardResp, handled, err := s.checkReward(ctx, req); err != nil {
		return nil, err
	} else if handled {
		return rewardResp, nil
	}
	return s.checkFree(ctx, req)
}

func (s *Service) checkPaid(ctx context.Context, req CheckRequest) (*CheckResponse, error) {
	keyHash := hashAPIKey(req.APIKey, s.salt)
	key, err := s.apiKeys.FindByHash(ctx, keyHash)
	if err != nil {
		return nil, err
	}

	response := &CheckResponse{
		Allowed:             false,
		AccessMode:          model.AccessModeBlocked,
		AllowedModes:        allowedRuntimeModes(key),
		DefaultRuntimeMode:  defaultRuntimeMode(key),
		SelectedRuntimeMode: selectedRuntimeMode(req, key),
	}
	now := time.Now().UTC()
	switch {
	case key == nil:
		response.ReasonCode = "invalid_api_key"
		response.Message = "api key is invalid"
	case key.Status == model.APIKeyStatusDisabled:
		response.ReasonCode = "disabled_api_key"
		response.Message = "api key is disabled"
	case key.ExpiresAt != nil && key.ExpiresAt.Before(now):
		response.ReasonCode = "expired_api_key"
		response.Message = "api key has expired"
	case requestedRuntimeMode(req, key) == string(model.AccessModeHosted) && !key.SupportsHosted():
		response.ReasonCode = "hosted_not_enabled"
		response.Message = "the current key does not allow hosted mode"
	case requestedRuntimeMode(req, key) == "external" && !key.SupportsExternal():
		response.ReasonCode = "external_not_enabled"
		response.Message = "the current key does not allow external mode"
	case requestedRuntimeMode(req, key) == string(model.AccessModeHosted) && key.AvailableCredits() <= 0:
		response.ReasonCode = "hosted_credit_exhausted"
		response.Message = "hosted credits are exhausted; add more credits first"
	case requestedRuntimeMode(req, key) != string(model.AccessModeHosted) && key.QuotaTotal != nil && key.PaidQuotaRemaining() <= 0:
		response.ReasonCode = "paid_quota_exhausted"
		response.Message = "the current key has no remaining generations; replace it or top up the quota pack"
	default:
		response.Allowed = true
		if requestedRuntimeMode(req, key) == string(model.AccessModeHosted) {
			response.AccessMode = model.AccessModeHosted
			response.HostedEnabled = key.SupportsHosted()
			response.CreditBalance = key.AvailableCredits()
		} else {
			response.AccessMode = model.AccessModePaid
		}
		response.PlanName = key.PlanName
		response.ExpiresAt = key.ExpiresAt
		if key.QuotaTotal != nil {
			response.PaidQuotaTotal = *key.QuotaTotal
			response.PaidQuotaUsed = key.QuotaUsed
			response.PaidQuotaRemaining = key.PaidQuotaRemaining()
		}
		response.CommitToken, err = s.issueCommitToken(req, response.AccessMode, key.KeyPrefix)
		if err != nil {
			return nil, err
		}
		_ = s.apiKeys.TouchLastUsedAt(ctx, key.ID, now)
	}

	event := buildUsageEvent(req, model.UsageModePaid, response, key)
	if err := s.usage.Create(ctx, event); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) checkReward(ctx context.Context, req CheckRequest) (*CheckResponse, bool, error) {
	if s.rewards == nil || req.UserID == 0 {
		return nil, false, nil
	}
	balance, err := s.rewards.Balance(ctx, req.UserID)
	if err != nil {
		return nil, false, err
	}
	if balance == nil || balance.Remaining <= 0 {
		return nil, false, nil
	}

	response := &CheckResponse{
		Allowed:             true,
		AccessMode:          model.AccessModeReward,
		AllowedModes:        []string{"external"},
		SelectedRuntimeMode: "external",
		RewardRemaining:     balance.Remaining,
		Message:             fmt.Sprintf("reward mode is active with %d generations remaining.", balance.Remaining),
	}
	response.CommitToken, err = s.issueCommitToken(req, model.AccessModeReward, "")
	if err != nil {
		return nil, false, err
	}
	if err := s.usage.Create(ctx, buildUsageEvent(req, model.UsageModeReward, response, nil)); err != nil {
		return nil, false, err
	}
	return response, true, nil
}

func (s *Service) checkFree(ctx context.Context, req CheckRequest) (*CheckResponse, error) {
	usageDate := s.currentUsageDate()
	quota, _, err := s.freeQuotas.GetOrCreateByFingerprint(ctx, req.FingerprintHash, usageDate, s.defaultFreeLimit)
	if err != nil {
		return nil, err
	}

	response := &CheckResponse{FreeLimit: quota.DailyLimit, FreeUsed: quota.DailyUsed, FreeRemaining: quota.Remaining()}
	response.AllowedModes = []string{"external"}
	response.SelectedRuntimeMode = "external"
	if quota.DailyUsed < quota.DailyLimit {
		response.Allowed = true
		response.AccessMode = model.AccessModeFree
		response.CommitToken, err = s.issueCommitToken(req, model.AccessModeFree, "")
		if err != nil {
			return nil, err
		}
	} else {
		response.Allowed = false
		response.AccessMode = model.AccessModeBlocked
		response.ReasonCode = "free_quota_exhausted"
		response.Message = "free quota is exhausted; configure an API key to continue"
	}

	event := buildUsageEvent(req, model.UsageModeFree, response, nil)
	if err := s.usage.Create(ctx, event); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) Consume(ctx context.Context, req ConsumeRequest) (*ConsumeResponse, error) {
	if strings.TrimSpace(req.FingerprintHash) == "" {
		return nil, ErrFingerprintRequired
	}
	if strings.TrimSpace(req.RequestID) == "" {
		return nil, ErrRequestIDRequired
	}
	if req.UsageType != string(model.UsageActionGenerate) {
		return nil, ErrInvalidUsageType
	}
	if req.AccessMode == "" {
		req.AccessMode = model.AccessModeFree
	}
	if req.AccessMode != model.AccessModeHosted {
		if req.CommitToken == nil {
			return nil, fmt.Errorf("commit_token is required")
		}
		if err := s.proof.verifyCommitToken(*req.CommitToken, req, req.AccessMode); err != nil {
			return nil, fmt.Errorf("invalid commit_token: %w", err)
		}
	}

	if cached, err := s.idem.GetConsumeResult(ctx, req.RequestID); err != nil {
		return nil, err
	} else if cached != nil {
		return cached, nil
	}

	if existing, err := s.usage.FindByRequestID(ctx, req.RequestID); err != nil {
		return nil, err
	} else if existing != nil {
		return s.restoreConsumeResult(ctx, req, existing)
	}

	lockKey := fmt.Sprintf("consume:%s:%s", req.FingerprintHash, req.RequestID)
	locked, err := s.idem.AcquireConsumeLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("consume request is already processing")
	}
	defer func() { _ = s.idem.ReleaseConsumeLock(ctx, lockKey) }()

	if cached, err := s.idem.GetConsumeResult(ctx, req.RequestID); err != nil {
		return nil, err
	} else if cached != nil {
		return cached, nil
	}

	switch req.AccessMode {
	case model.AccessModePaid:
		return s.consumePaid(ctx, req)
	case model.AccessModeReward:
		return s.consumeReward(ctx, req)
	case model.AccessModeFree:
		fallthrough
	default:
		return s.consumeFree(ctx, req)
	}
}

func (s *Service) restoreConsumeResult(ctx context.Context, req ConsumeRequest, existing *model.UsageEvent) (*ConsumeResponse, error) {
	switch existing.Mode {
	case model.UsageModePaid:
		if strings.TrimSpace(req.APIKey) == "" {
			resp := &ConsumeResponse{AccessMode: model.AccessModePaid}
			_ = s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL)
			return resp, nil
		}
		key, err := s.apiKeys.FindByHash(ctx, hashAPIKey(req.APIKey, s.salt))
		if err != nil {
			return nil, err
		}
		resp := paidConsumeResponse(key)
		if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
			return nil, err
		}
		return resp, nil
	case model.UsageModeReward:
		resp, err := s.rewardBalanceResponse(ctx, rewardUserID(req, existing))
		if err != nil {
			return nil, err
		}
		if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
			return nil, err
		}
		return resp, nil
	default:
		usageDate := s.currentUsageDate()
		if !existing.CreatedAt.IsZero() {
			usageDate = existing.CreatedAt.UTC().Format("2006-01-02")
		}
		quota, err := s.freeQuotas.GetByFingerprint(ctx, req.FingerprintHash, usageDate)
		if err != nil {
			return nil, err
		}
		resp := &ConsumeResponse{
			AccessMode:    model.AccessModeFree,
			FreeUsed:      quota.DailyUsed,
			FreeRemaining: quota.Remaining(),
			Remaining:     quota.Remaining(),
		}
		if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func (s *Service) consumeFree(ctx context.Context, req ConsumeRequest) (*ConsumeResponse, error) {
	quota, err := s.freeQuotas.Consume(ctx, req.FingerprintHash, s.currentUsageDate(), s.defaultFreeLimit)
	if err != nil {
		return nil, err
	}
	requestID := req.RequestID
	event := &model.UsageEvent{
		RequestID:       &requestID,
		FingerprintHash: req.FingerprintHash,
		Mode:            model.UsageModeFree,
		Action:          model.UsageActionGenerate,
		Result:          model.UsageResultAllowed,
		Charged:         true,
		BilledUnits:     1,
		UnitType:        "document",
		UserID:          optionalUserID(req.UserID),
	}
	if err := s.usage.Create(ctx, event); err != nil {
		return nil, err
	}
	if err := s.activateReferralOnSuccess(ctx, req.UserID); err != nil {
		return nil, err
	}
	resp := &ConsumeResponse{
		AccessMode:    model.AccessModeFree,
		FreeUsed:      quota.DailyUsed,
		FreeRemaining: quota.Remaining(),
		Remaining:     quota.Remaining(),
	}
	if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *Service) consumeReward(ctx context.Context, req ConsumeRequest) (*ConsumeResponse, error) {
	if s.rewards == nil {
		return nil, ErrRewardQuotaExhausted
	}
	result, err := s.rewards.Consume(ctx, req.UserID)
	if err != nil {
		if errors.Is(err, rewardsvc.ErrQuotaExhausted) {
			return nil, ErrRewardQuotaExhausted
		}
		return nil, err
	}
	requestID := req.RequestID
	event := &model.UsageEvent{
		RequestID:       &requestID,
		FingerprintHash: req.FingerprintHash,
		Mode:            model.UsageModeReward,
		Action:          model.UsageActionGenerate,
		Result:          model.UsageResultAllowed,
		Charged:         true,
		BilledUnits:     1,
		UnitType:        "document",
		UserID:          optionalUserID(req.UserID),
	}
	if err := s.usage.Create(ctx, event); err != nil {
		return nil, err
	}
	if err := s.activateReferralOnSuccess(ctx, req.UserID); err != nil {
		return nil, err
	}
	resp := &ConsumeResponse{
		AccessMode:      model.AccessModeReward,
		RewardRemaining: result.Remaining,
		Remaining:       result.Remaining,
	}
	if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *Service) consumePaid(ctx context.Context, req ConsumeRequest) (*ConsumeResponse, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return nil, fmt.Errorf("api_key is required for paid consume")
	}
	keyHash := hashAPIKey(req.APIKey, s.salt)
	key, err := s.apiKeys.ConsumePaidByHash(ctx, keyHash)
	if err != nil {
		if strings.Contains(err.Error(), "paid quota exhausted") {
			return nil, ErrPaidQuotaExhausted
		}
		return nil, err
	}
	requestID := req.RequestID
	event := &model.UsageEvent{
		RequestID:       &requestID,
		FingerprintHash: req.FingerprintHash,
		Mode:            model.UsageModePaid,
		Action:          model.UsageActionGenerate,
		APIKeyID:        &key.ID,
		Result:          model.UsageResultAllowed,
		Charged:         true,
		BilledUnits:     1,
		UnitType:        "document",
		UserID:          optionalUserID(req.UserID),
	}
	if err := s.usage.Create(ctx, event); err != nil {
		return nil, err
	}
	if err := s.activateReferralOnSuccess(ctx, req.UserID); err != nil {
		return nil, err
	}
	resp := paidConsumeResponse(key)
	if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
		return nil, err
	}
	return resp, nil
}

func paidConsumeResponse(key *model.APIKey) *ConsumeResponse {
	resp := &ConsumeResponse{AccessMode: model.AccessModePaid}
	if key == nil {
		return resp
	}
	if key.QuotaTotal != nil {
		resp.PaidQuotaTotal = *key.QuotaTotal
		resp.PaidQuotaUsed = key.QuotaUsed
		resp.PaidQuotaRemaining = key.PaidQuotaRemaining()
		resp.Remaining = key.PaidQuotaRemaining()
	}
	return resp
}

func (s *Service) rewardBalanceResponse(ctx context.Context, userID uint64) (*ConsumeResponse, error) {
	resp := &ConsumeResponse{AccessMode: model.AccessModeReward}
	if s.rewards == nil || userID == 0 {
		return resp, nil
	}
	balance, err := s.rewards.Balance(ctx, userID)
	if err != nil {
		return nil, err
	}
	if balance != nil {
		resp.RewardRemaining = balance.Remaining
		resp.Remaining = balance.Remaining
	}
	return resp, nil
}

func (s *Service) currentUsageDate() string {
	now := time.Now().UTC()
	if s.clock != nil {
		now = s.clock().UTC()
	}
	return now.Format("2006-01-02")
}

func (s *Service) issueCommitToken(req CheckRequest, accessMode model.AccessMode, apiKeyHint string) (*CommitToken, error) {
	if s.proof == nil {
		return nil, fmt.Errorf("license proof signer is unavailable")
	}
	return s.proof.issueCommitToken(req, accessMode, apiKeyHint)
}

func buildUsageEvent(req CheckRequest, mode model.UsageMode, resp *CheckResponse, key *model.APIKey) *model.UsageEvent {
	action := model.UsageAction(req.Action)
	result := model.UsageResultBlocked
	if resp.Allowed {
		result = model.UsageResultAllowed
	}
	event := &model.UsageEvent{
		FingerprintHash: req.FingerprintHash,
		Mode:            mode,
		Action:          action,
		Result:          result,
		BilledUnits:     0,
		UnitType:        "document",
		Charged:         false,
		UserID:          optionalUserID(req.UserID),
	}
	if key != nil {
		event.APIKeyID = &key.ID
	}
	if req.CLIVersion != "" {
		event.CLIVersion = &req.CLIVersion
	}
	if req.DocumentType != "" {
		event.DocumentType = &req.DocumentType
	}
	if req.RuntimeMode != "" {
		event.RuntimeMode = &req.RuntimeMode
	}
	if resp.ReasonCode != "" {
		event.ReasonCode = &resp.ReasonCode
	}
	return event
}

func allowedRuntimeModes(key *model.APIKey) []string {
	if key == nil {
		return nil
	}
	modes := make([]string, 0, 2)
	if key.SupportsExternal() {
		modes = append(modes, "external")
	}
	if key.SupportsHosted() {
		modes = append(modes, "hosted")
	}
	return modes
}

func defaultRuntimeMode(key *model.APIKey) string {
	if key == nil || key.DefaultRuntimeMode == nil {
		return ""
	}
	return strings.TrimSpace(*key.DefaultRuntimeMode)
}

func requestedRuntimeMode(req CheckRequest, key *model.APIKey) string {
	runtimeMode := strings.TrimSpace(req.RuntimeMode)
	if runtimeMode != "" {
		return runtimeMode
	}
	if key != nil && key.DefaultRuntimeMode != nil && strings.TrimSpace(*key.DefaultRuntimeMode) != "" {
		return strings.TrimSpace(*key.DefaultRuntimeMode)
	}
	return "external"
}

func selectedRuntimeMode(req CheckRequest, key *model.APIKey) string {
	return requestedRuntimeMode(req, key)
}

func hashAPIKey(apiKey, salt string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + strings.TrimSpace(apiKey)))
	return hex.EncodeToString(sum[:])
}

func optionalUserID(userID uint64) *uint64 {
	if userID == 0 {
		return nil
	}
	return &userID
}

func rewardUserID(req ConsumeRequest, existing *model.UsageEvent) uint64 {
	if req.UserID != 0 {
		return req.UserID
	}
	if existing != nil && existing.UserID != nil {
		return *existing.UserID
	}
	return 0
}

func (s *Service) activateReferralOnSuccess(ctx context.Context, userID uint64) error {
	if s.referrals == nil || userID == 0 {
		return nil
	}
	_, err := s.referrals.ActivateReferral(ctx, userID, growthsvc.InviteActivationRewardAmount)
	if err == nil || errors.Is(err, growthsvc.ErrReferralNotFound) {
		return nil
	}
	return err
}
