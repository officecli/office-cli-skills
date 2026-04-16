package admin

import "time"

type LoginRequest struct {
	Password string `json:"password"`
}

type AdminIdentity struct {
	Email      string `json:"email,omitempty"`
	Name       string `json:"name,omitempty"`
	AuthMethod string `json:"auth_method,omitempty"`
}

type AccessDeniedError struct {
	Email string
}

func (e *AccessDeniedError) Error() string {
	return "admin account is not authorized"
}

type LoginResponse struct {
	Success bool `json:"success"`
}

type CreateAPIKeyRequest struct {
	OwnerUserID        *uint64    `json:"owner_user_id,omitempty"`
	PlanName           string     `json:"plan_name"`
	AllowedModes       *string    `json:"allowed_modes,omitempty"`
	HostedEnabled      *bool      `json:"hosted_enabled,omitempty"`
	DefaultRuntimeMode *string    `json:"default_runtime_mode,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	Note               *string    `json:"note,omitempty"`
	PlanCode           *string    `json:"plan_code,omitempty"`
	QuotaTotal         *int       `json:"quota_total,omitempty"`
	CreditBalance      *int       `json:"credit_balance,omitempty"`
}

type UpdateAPIKeyRequest struct {
	Status             *string    `json:"status,omitempty"`
	OwnerUserID        *uint64    `json:"owner_user_id,omitempty"`
	PlanName           *string    `json:"plan_name,omitempty"`
	AllowedModes       *string    `json:"allowed_modes,omitempty"`
	HostedEnabled      *bool      `json:"hosted_enabled,omitempty"`
	DefaultRuntimeMode *string    `json:"default_runtime_mode,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	Note               *string    `json:"note,omitempty"`
	PlanCode           *string    `json:"plan_code,omitempty"`
	QuotaTotal         *int       `json:"quota_total,omitempty"`
	QuotaUsed          *int       `json:"quota_used,omitempty"`
	CreditBalance      *int       `json:"credit_balance,omitempty"`
	CreditReserved     *int       `json:"credit_reserved,omitempty"`
}

type UpdateFreeQuotaRequest struct {
	FreeLimit int `json:"free_limit"`
}

type QuotaSourcesFilter struct {
	Fingerprint string `json:"fingerprint,omitempty"`
	UsageDate   string `json:"usage_date,omitempty"`
	KeyPrefix   string `json:"key_prefix,omitempty"`
	UserID      uint64 `json:"user_id,omitempty"`
}

type UpdateOrderRequest struct {
	Status *string `json:"status,omitempty"`
	Note   *string `json:"note,omitempty"`
}

type UpdateUserRequest struct {
	Status *string `json:"status,omitempty"`
}

type CreateAPIKeyResponse struct {
	PlaintextKey string `json:"plaintext_key"`
	KeyPrefix    string `json:"key_prefix"`
}
