package license

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	growthsvc "github.com/officecli/officecli-internal/platform/internal/growth"
	"github.com/officecli/officecli-internal/platform/internal/model"
	rewardsvc "github.com/officecli/officecli-internal/platform/internal/reward"
)

type APIKeyStore interface {
	FindByHash(ctx context.Context, hash string) (*model.APIKey, error)
	TouchLastUsedAt(ctx context.Context, id uint64, usedAt time.Time) error
	ConsumePaidByHash(ctx context.Context, hash string) (*model.APIKey, error)
}

type FreeQuotaStore interface {
	GetOrCreateByFingerprint(ctx context.Context, fingerprint string, defaultLimit int) (*model.FreeQuota, bool, error)
	GetByFingerprint(ctx context.Context, fingerprint string) (*model.FreeQuota, error)
	Consume(ctx context.Context, fingerprint string, defaultLimit int) (*model.FreeQuota, error)
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
	if isExternalRuntime(req.RuntimeMode) {
		return s.checkExternalFree(ctx, req)
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
		response.Message = "the current key has no remaining document generations; replace it or buy more quota"
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
	response.QuotaSnapshot, err = s.buildQuotaSnapshot(ctx, req, key, nil, response.RewardRemaining)
	if err != nil {
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
		Message:             fmt.Sprintf("reward mode is active with %d document generations remaining.", balance.Remaining),
	}
	response.CommitToken, err = s.issueCommitToken(req, model.AccessModeReward, "")
	if err != nil {
		return nil, false, err
	}
	if err := s.usage.Create(ctx, buildUsageEvent(req, model.UsageModeReward, response, nil)); err != nil {
		return nil, false, err
	}
	response.QuotaSnapshot, err = s.buildQuotaSnapshot(ctx, req, nil, nil, balance.Remaining)
	if err != nil {
		return nil, false, err
	}
	return response, true, nil
}

func (s *Service) checkFree(ctx context.Context, req CheckRequest) (*CheckResponse, error) {
	freeLimit := s.freeLimitForDocumentType(req.DocumentType)
	quota, _, err := s.freeQuotas.GetOrCreateByFingerprint(ctx, req.FingerprintHash, freeLimit)
	if err != nil {
		return nil, err
	}

	response := &CheckResponse{FreeLimit: quota.FreeLimit, FreeUsed: quota.FreeUsed, FreeRemaining: quota.FreeRemaining()}
	response.AllowedModes = []string{"external"}
	response.SelectedRuntimeMode = selectedFreeRuntimeMode(req)
	if quota.FreeUsed < quota.FreeLimit {
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
		response.Message = "Free trial quota is used up. Continue at https://officecli.io/pricing, then run officecli auth set-key <api-key>."
	}

	event := buildUsageEvent(req, model.UsageModeFree, response, nil)
	if err := s.usage.Create(ctx, event); err != nil {
		return nil, err
	}
	response.QuotaSnapshot, err = s.buildQuotaSnapshot(ctx, req, nil, quota, response.RewardRemaining)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) checkExternalFree(ctx context.Context, req CheckRequest) (*CheckResponse, error) {
	response := &CheckResponse{
		Allowed:             true,
		AccessMode:          model.AccessModeFree,
		AllowedModes:        []string{"external"},
		SelectedRuntimeMode: "external",
		Message:             "External mode is free with unlimited generations.",
	}
	var err error
	response.CommitToken, err = s.issueCommitToken(req, model.AccessModeFree, "")
	if err != nil {
		return nil, err
	}
	if err := s.usage.Create(ctx, buildUsageEvent(req, model.UsageModeFree, response, nil)); err != nil {
		return nil, err
	}
	response.QuotaSnapshot, err = s.buildQuotaSnapshot(ctx, req, nil, nil, response.RewardRemaining)
	if err != nil {
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
		if err := s.ValidateCommitToken(req); err != nil {
			return nil, err
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
		if isExternalRuntime(commitTokenRuntimeMode(req.CommitToken)) {
			return s.consumeExternalFree(ctx, req)
		}
		return s.consumeFree(ctx, req)
	}
}

func (s *Service) ValidateCommitToken(req ConsumeRequest) error {
	if req.AccessMode == "" {
		req.AccessMode = model.AccessModeFree
	}
	if req.CommitToken == nil {
		return fmt.Errorf("commit_token is required")
	}
	if err := s.proof.verifyCommitToken(*req.CommitToken, req, req.AccessMode); err != nil {
		return fmt.Errorf("invalid commit_token: %w", err)
	}
	return nil
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
		if existing.RuntimeMode != nil && isExternalRuntime(*existing.RuntimeMode) {
			resp := &ConsumeResponse{AccessMode: model.AccessModeFree}
			if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
				return nil, err
			}
			return resp, nil
		}
		quota, err := s.freeQuotas.GetByFingerprint(ctx, req.FingerprintHash)
		if err != nil {
			return nil, err
		}
		if quota == nil {
			quota = &model.FreeQuota{FingerprintHash: req.FingerprintHash, FreeLimit: s.defaultFreeLimit}
		}
		resp := &ConsumeResponse{
			AccessMode:    model.AccessModeFree,
			FreeUsed:      quota.FreeUsed,
			FreeRemaining: quota.FreeRemaining(),
			Remaining:     quota.FreeRemaining(),
		}
		if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func (s *Service) consumeExternalFree(ctx context.Context, req ConsumeRequest) (*ConsumeResponse, error) {
	documentType := consumeDocumentType(req)
	requestID := req.RequestID
	runtimeMode := commitTokenRuntimeMode(req.CommitToken)
	event := &model.UsageEvent{
		RequestID:       &requestID,
		FingerprintHash: req.FingerprintHash,
		Mode:            model.UsageModeFree,
		Action:          model.UsageActionGenerate,
		Result:          model.UsageResultAllowed,
		Charged:         true,
		BilledUnits:     0,
		UnitType:        unitTypeForDocumentType(documentType),
		DocumentType:    optionalString(documentType),
		RuntimeMode:     optionalString(runtimeMode),
		UserID:          optionalUserID(req.UserID),
	}
	if err := s.usage.Create(ctx, event); err != nil {
		return nil, err
	}
	if err := s.activateReferralOnSuccess(ctx, req.UserID); err != nil {
		return nil, err
	}
	resp := &ConsumeResponse{AccessMode: model.AccessModeFree}
	if err := s.idem.SaveConsumeResult(ctx, req.RequestID, resp, s.idemTTL); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *Service) consumeFree(ctx context.Context, req ConsumeRequest) (*ConsumeResponse, error) {
	documentType := consumeDocumentType(req)
	quota, err := s.freeQuotas.Consume(ctx, req.FingerprintHash, s.freeLimitForDocumentType(documentType))
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
		UnitType:        unitTypeForDocumentType(documentType),
		DocumentType:    optionalString(documentType),
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
		FreeUsed:      quota.FreeUsed,
		FreeRemaining: quota.FreeRemaining(),
		Remaining:     quota.FreeRemaining(),
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
		UnitType:        unitTypeForDocumentType(consumeDocumentType(req)),
		DocumentType:    optionalString(consumeDocumentType(req)),
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
		UnitType:        unitTypeForDocumentType(consumeDocumentType(req)),
		DocumentType:    optionalString(consumeDocumentType(req)),
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

func (s *Service) buildQuotaSnapshot(ctx context.Context, req CheckRequest, key *model.APIKey, freeQuota *model.FreeQuota, rewardRemaining int) (*QuotaSnapshot, error) {
	freeLimit := s.freeLimitForDocumentType(req.DocumentType)
	freeTrial := FreeTrialSnapshot{
		Scope:      "lifetime",
		Limit:      freeLimit,
		Used:       0,
		Remaining:  freeLimit,
		BinaryOnly: true,
	}
	snapshot := &QuotaSnapshot{
		FreeTrial: freeTrial,
		FreeTrialDaily: FreeTrialDailySnapshot{
			UsageDate:               s.currentUsageDate(),
			Limit:                   freeLimit,
			Used:                    0,
			Remaining:               freeLimit,
			BinaryOnly:              true,
			IncludedInAccountTotals: false,
		},
		RewardQuota: RewardQuotaSnapshot{
			Remaining: rewardRemaining,
		},
	}

	if freeQuota == nil && s.freeQuotas != nil {
		var err error
		freeQuota, err = s.freeQuotas.GetByFingerprint(ctx, req.FingerprintHash)
		if err != nil {
			return nil, err
		}
	}
	if freeQuota != nil {
		snapshot.FreeTrial = FreeTrialSnapshot{
			Scope:      "lifetime",
			Limit:      freeQuota.FreeLimit,
			Used:       freeQuota.FreeUsed,
			Remaining:  freeQuota.FreeRemaining(),
			BinaryOnly: true,
		}
		snapshot.FreeTrialDaily.Limit = freeQuota.FreeLimit
		snapshot.FreeTrialDaily.Used = freeQuota.FreeUsed
		snapshot.FreeTrialDaily.Remaining = freeQuota.FreeRemaining()
	}

	if snapshot.RewardQuota.Remaining == 0 && s.rewards != nil && req.UserID != 0 {
		balance, err := s.rewards.Balance(ctx, req.UserID)
		if err != nil {
			return nil, err
		}
		if balance != nil {
			snapshot.RewardQuota.Remaining = balance.Remaining
		}
	}

	if key != nil {
		snapshot.PaidExternalQuota.CurrentKeyPrefix = key.KeyPrefix
		if key.QuotaTotal != nil {
			snapshot.PaidExternalQuota.CurrentKeyTotal = *key.QuotaTotal
			snapshot.PaidExternalQuota.CurrentKeyUsed = key.QuotaUsed
			snapshot.PaidExternalQuota.CurrentKeyRemaining = key.PaidQuotaRemaining()
		}
	}

	return snapshot, nil
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
		UnitType:        unitTypeForDocumentType(req.DocumentType),
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

func (s *Service) freeLimitForDocumentType(documentType string) int {
	return s.defaultFreeLimit
}

func selectedFreeRuntimeMode(req CheckRequest) string {
	runtimeMode := strings.TrimSpace(req.RuntimeMode)
	if runtimeMode != "" {
		return runtimeMode
	}
	return "hosted"
}

func quotaDocumentType(documentType string) string {
	switch strings.ToLower(strings.TrimSpace(documentType)) {
	case "img", "image":
		return "img"
	default:
		return "document"
	}
}

func unitTypeForDocumentType(documentType string) string {
	if quotaDocumentType(documentType) == "img" {
		return "image"
	}
	return "document"
}

func consumeDocumentType(req ConsumeRequest) string {
	if req.CommitToken != nil && strings.TrimSpace(req.CommitToken.DocumentType) != "" {
		return req.CommitToken.DocumentType
	}
	return ""
}

func commitTokenRuntimeMode(token *CommitToken) string {
	if token == nil {
		return ""
	}
	return strings.TrimSpace(token.RuntimeMode)
}

func isExternalRuntime(runtimeMode string) bool {
	return strings.EqualFold(strings.TrimSpace(runtimeMode), "external")
}

func hashAPIKey(apiKey, salt string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + strings.TrimSpace(apiKey)))
	return hex.EncodeToString(sum[:])
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
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
