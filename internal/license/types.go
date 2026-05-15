package license

import "time"

type Config struct {
	BaseURL            string     `json:"base_url"`
	APIKey             string     `json:"api_key"`
	UserID             uint64     `json:"user_id,omitempty"`
	SessionToken       string     `json:"session_token,omitempty"`
	SessionTokenPrefix string     `json:"session_token_prefix,omitempty"`
	SessionExpiresAt   *time.Time `json:"session_expires_at,omitempty"`
	Enabled            bool       `json:"enabled"`
	TimeoutSec         int        `json:"timeout_sec"`
}

type AccessMode string

const (
	AccessModeDisabled AccessMode = "disabled"
	AccessModeFree     AccessMode = "free"
	AccessModeReward   AccessMode = "reward"
	AccessModePaid     AccessMode = "paid"
	AccessModeHosted   AccessMode = "hosted"
	AccessModeBlocked  AccessMode = "blocked"
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
	FingerprintHash string     `json:"fingerprint_hash"`
	UserID          uint64     `json:"user_id,omitempty"`
	RequestID       string     `json:"request_id"`
	AccessMode      AccessMode `json:"access_mode,omitempty"`
	APIKeyHint      string     `json:"api_key_hint,omitempty"`
	Action          string     `json:"action,omitempty"`
	DocumentType    string     `json:"document_type,omitempty"`
	RuntimeMode     string     `json:"runtime_mode,omitempty"`
	RequestNonce    string     `json:"request_nonce,omitempty"`
	ProofVersion    string     `json:"proof_version,omitempty"`
	IssuedAt        time.Time  `json:"issued_at,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at,omitempty"`
	Signature       string     `json:"signature,omitempty"`
}

type FreeTrialDailySnapshot struct {
	UsageDate               string `json:"usage_date,omitempty"`
	Limit                   int    `json:"limit,omitempty"`
	Used                    int    `json:"used,omitempty"`
	Remaining               int    `json:"remaining,omitempty"`
	BinaryOnly              bool   `json:"binary_only,omitempty"`
	IncludedInAccountTotals bool   `json:"included_in_account_totals,omitempty"`
}

type FreeTrialSnapshot struct {
	Scope      string `json:"scope,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Used       int    `json:"used,omitempty"`
	Remaining  int    `json:"remaining,omitempty"`
	BinaryOnly bool   `json:"binary_only,omitempty"`
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
	FreeTrial         FreeTrialSnapshot         `json:"free_trial,omitempty"`
	FreeTrialDaily    FreeTrialDailySnapshot    `json:"free_trial_daily,omitempty"`
	RewardQuota       RewardQuotaSnapshot       `json:"reward_quota,omitempty"`
	PaidExternalQuota PaidExternalQuotaSnapshot `json:"paid_external_quota,omitempty"`
}

type CheckResult struct {
	Allowed             bool           `json:"allowed"`
	AccessMode          AccessMode     `json:"access_mode"`
	ReasonCode          string         `json:"reason_code,omitempty"`
	Message             string         `json:"message,omitempty"`
	AllowedModes        []string       `json:"allowed_modes,omitempty"`
	DefaultRuntimeMode  string         `json:"default_runtime_mode,omitempty"`
	SelectedRuntimeMode string         `json:"selected_runtime_mode,omitempty"`
	HostedEnabled       bool           `json:"hosted_enabled,omitempty"`
	CreditBalance       int            `json:"credit_balance,omitempty"`
	FreeLimit           int            `json:"free_limit,omitempty"`
	FreeUsed            int            `json:"free_used,omitempty"`
	FreeRemaining       int            `json:"free_remaining,omitempty"`
	PlanName            string         `json:"plan_name,omitempty"`
	ExpiresAt           *time.Time     `json:"expires_at,omitempty"`
	RewardRemaining     int            `json:"reward_remaining,omitempty"`
	PaidQuotaTotal      int            `json:"paid_quota_total,omitempty"`
	PaidQuotaUsed       int            `json:"paid_quota_used,omitempty"`
	PaidQuotaRemaining  int            `json:"paid_quota_remaining,omitempty"`
	QuotaSnapshot       *QuotaSnapshot `json:"quota_snapshot,omitempty"`
	CommitToken         CommitToken    `json:"commit_token,omitempty"`
}

type ConsumeRequest struct {
	FingerprintHash string      `json:"fingerprint_hash"`
	UserID          uint64      `json:"user_id,omitempty"`
	RequestID       string      `json:"request_id"`
	UsageType       string      `json:"usage_type"`
	AccessMode      AccessMode  `json:"access_mode,omitempty"`
	APIKey          string      `json:"api_key,omitempty"`
	CommitToken     CommitToken `json:"commit_token,omitempty"`
}

type ConsumeResult struct {
	AccessMode         AccessMode `json:"access_mode,omitempty"`
	CreditBalance      int        `json:"credit_balance,omitempty"`
	FreeUsed           int        `json:"free_used,omitempty"`
	FreeRemaining      int        `json:"free_remaining,omitempty"`
	RewardRemaining    int        `json:"reward_remaining,omitempty"`
	PaidQuotaTotal     int        `json:"paid_quota_total,omitempty"`
	PaidQuotaUsed      int        `json:"paid_quota_used,omitempty"`
	PaidQuotaRemaining int        `json:"paid_quota_remaining,omitempty"`
	Remaining          int        `json:"remaining,omitempty"`
}
