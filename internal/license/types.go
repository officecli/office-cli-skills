package license

import "time"

type Config struct {
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	UserID     uint64 `json:"user_id,omitempty"`
	Enabled    bool   `json:"enabled"`
	TimeoutSec int    `json:"timeout_sec"`
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

type CheckResult struct {
	Allowed             bool        `json:"allowed"`
	AccessMode          AccessMode  `json:"access_mode"`
	ReasonCode          string      `json:"reason_code,omitempty"`
	Message             string      `json:"message,omitempty"`
	AllowedModes        []string    `json:"allowed_modes,omitempty"`
	DefaultRuntimeMode  string      `json:"default_runtime_mode,omitempty"`
	SelectedRuntimeMode string      `json:"selected_runtime_mode,omitempty"`
	HostedEnabled       bool        `json:"hosted_enabled,omitempty"`
	CreditBalance       int         `json:"credit_balance,omitempty"`
	FreeLimit           int         `json:"free_limit,omitempty"`
	FreeUsed            int         `json:"free_used,omitempty"`
	FreeRemaining       int         `json:"free_remaining,omitempty"`
	PlanName            string      `json:"plan_name,omitempty"`
	ExpiresAt           *time.Time  `json:"expires_at,omitempty"`
	RewardRemaining     int         `json:"reward_remaining,omitempty"`
	PaidQuotaTotal      int         `json:"paid_quota_total,omitempty"`
	PaidQuotaUsed       int         `json:"paid_quota_used,omitempty"`
	PaidQuotaRemaining  int         `json:"paid_quota_remaining,omitempty"`
	CommitToken         CommitToken `json:"commit_token,omitempty"`
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
