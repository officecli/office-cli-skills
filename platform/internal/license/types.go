package license

import (
	"errors"
	"time"

	"github.com/officecli/officecli/platform/internal/model"
)

var (
	ErrFingerprintRequired  = errors.New("fingerprint_hash is required")
	ErrRequestNonceRequired = errors.New("request_nonce is required")
	ErrInvalidAction        = errors.New("action must be generate or status")
	ErrInvalidUsageType     = errors.New("usage_type must be generate")
	ErrQuotaExhausted       = errors.New("free quota exhausted")
	ErrPaidQuotaExhausted   = errors.New("paid quota exhausted")
	ErrRewardQuotaExhausted = errors.New("reward quota exhausted")
	ErrRequestIDRequired    = errors.New("request_id is required")
)

type CheckRequest struct {
	FingerprintHash string `json:"fingerprint_hash"`
	UserID          uint64 `json:"user_id,omitempty"`
	APIKey          string `json:"api_key,omitempty"`
	CLIVersion      string `json:"cli_version,omitempty"`
	DocumentType    string `json:"document_type,omitempty"`
	RuntimeMode     string `json:"runtime_mode,omitempty"`
	RequestNonce    string `json:"request_nonce,omitempty"`
	Action          string `json:"action"`
}

type CommitToken struct {
	FingerprintHash string           `json:"fingerprint_hash"`
	UserID          uint64           `json:"user_id,omitempty"`
	RequestID       string           `json:"request_id"`
	AccessMode      model.AccessMode `json:"access_mode,omitempty"`
	APIKeyHint      string           `json:"api_key_hint,omitempty"`
	Action          string           `json:"action,omitempty"`
	DocumentType    string           `json:"document_type,omitempty"`
	RuntimeMode     string           `json:"runtime_mode,omitempty"`
	RequestNonce    string           `json:"request_nonce,omitempty"`
	ProofVersion    string           `json:"proof_version,omitempty"`
	IssuedAt        time.Time        `json:"issued_at,omitempty"`
	ExpiresAt       time.Time        `json:"expires_at,omitempty"`
	Signature       string           `json:"signature,omitempty"`
}

type FreeTrialDailySnapshot struct {
	UsageDate               string `json:"usage_date,omitempty"`
	Limit                   int    `json:"limit,omitempty"`
	Used                    int    `json:"used,omitempty"`
	Remaining               int    `json:"remaining,omitempty"`
	BinaryOnly              bool   `json:"binary_only,omitempty"`
	IncludedInAccountTotals bool   `json:"included_in_account_totals,omitempty"`
}

type RewardQuotaSnapshot struct {
	Remaining int `json:"remaining,omitempty"`
}

type PaidExternalQuotaSnapshot struct {
	CurrentKeyPrefix    string `json:"current_key_prefix,omitempty"`
	CurrentKeyTotal     int    `json:"current_key_total,omitempty"`
	CurrentKeyUsed      int    `json:"current_key_used,omitempty"`
	CurrentKeyRemaining int    `json:"current_key_remaining,omitempty"`
}

type QuotaSnapshot struct {
	FreeTrialDaily    FreeTrialDailySnapshot    `json:"free_trial_daily,omitempty"`
	RewardQuota       RewardQuotaSnapshot       `json:"reward_quota,omitempty"`
	PaidExternalQuota PaidExternalQuotaSnapshot `json:"paid_external_quota,omitempty"`
}

type CheckResponse struct {
	Allowed             bool             `json:"allowed"`
	AccessMode          model.AccessMode `json:"access_mode"`
	ReasonCode          string           `json:"reason_code,omitempty"`
	Message             string           `json:"message,omitempty"`
	AllowedModes        []string         `json:"allowed_modes,omitempty"`
	DefaultRuntimeMode  string           `json:"default_runtime_mode,omitempty"`
	SelectedRuntimeMode string           `json:"selected_runtime_mode,omitempty"`
	HostedEnabled       bool             `json:"hosted_enabled,omitempty"`
	CreditBalance       int              `json:"credit_balance,omitempty"`
	FreeLimit           int              `json:"free_limit,omitempty"`
	FreeUsed            int              `json:"free_used,omitempty"`
	FreeRemaining       int              `json:"free_remaining,omitempty"`
	RewardRemaining     int              `json:"reward_remaining,omitempty"`
	PlanName            string           `json:"plan_name,omitempty"`
	ExpiresAt           *time.Time       `json:"expires_at,omitempty"`
	PaidQuotaTotal      int              `json:"paid_quota_total,omitempty"`
	PaidQuotaUsed       int              `json:"paid_quota_used,omitempty"`
	PaidQuotaRemaining  int              `json:"paid_quota_remaining,omitempty"`
	QuotaSnapshot       *QuotaSnapshot   `json:"quota_snapshot,omitempty"`
	CommitToken         *CommitToken     `json:"commit_token,omitempty"`
}

type ConsumeRequest struct {
	FingerprintHash string           `json:"fingerprint_hash"`
	UserID          uint64           `json:"user_id,omitempty"`
	RequestID       string           `json:"request_id"`
	UsageType       string           `json:"usage_type"`
	AccessMode      model.AccessMode `json:"access_mode,omitempty"`
	APIKey          string           `json:"api_key,omitempty"`
	CommitToken     *CommitToken     `json:"commit_token,omitempty"`
}

type ConsumeResponse struct {
	AccessMode         model.AccessMode `json:"access_mode,omitempty"`
	CreditBalance      int              `json:"credit_balance,omitempty"`
	FreeUsed           int              `json:"free_used,omitempty"`
	FreeRemaining      int              `json:"free_remaining,omitempty"`
	RewardRemaining    int              `json:"reward_remaining,omitempty"`
	PaidQuotaTotal     int              `json:"paid_quota_total,omitempty"`
	PaidQuotaUsed      int              `json:"paid_quota_used,omitempty"`
	PaidQuotaRemaining int              `json:"paid_quota_remaining,omitempty"`
	Remaining          int              `json:"remaining,omitempty"`
}
