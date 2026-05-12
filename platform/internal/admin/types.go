package admin

import (
	"time"

	"github.com/officecli/officecli/platform/internal/model"
)

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

type UpdateHostedPricingSettingsRequest struct {
	MarkupBPS     int    `json:"markup_bps"`
	Currency      string `json:"currency,omitempty"`
	CreditsPerUSD int    `json:"credits_per_usd,omitempty"`
}

type UpsertHostedModelPricingConfigRequest struct {
	Key                        string `json:"key"`
	Kind                       string `json:"kind"`
	Provider                   string `json:"provider"`
	Model                      string `json:"model"`
	PromptPer1MCostMicrousd    int64  `json:"prompt_per_1m_cost_microusd"`
	OutputPer1MCostMicrousd    int64  `json:"output_per_1m_cost_microusd"`
	ReasoningPer1MCostMicrousd int64  `json:"reasoning_per_1m_cost_microusd"`
	PromptPer1MCostCredits     *int64 `json:"prompt_per_1m_cost_credits,omitempty"`
	OutputPer1MCostCredits     *int64 `json:"output_per_1m_cost_credits,omitempty"`
	ReasoningPer1MCostCredits  *int64 `json:"reasoning_per_1m_cost_credits,omitempty"`
	Enabled                    bool   `json:"enabled"`
}

type UpsertHostedPricingRuleRequest struct {
	DocumentProfile            string `json:"document_profile"`
	Provider                   string `json:"provider"`
	Model                      string `json:"model"`
	TextModelKey               string `json:"text_model_key"`
	ImageModelKey              string `json:"image_model_key"`
	PromptPer1KCostMicrousd    int64  `json:"prompt_per_1k_cost_microusd"`
	OutputPer1KCostMicrousd    int64  `json:"output_per_1k_cost_microusd"`
	ReasoningPer1KCostMicrousd int64  `json:"reasoning_per_1k_cost_microusd"`
	ImagePerAssetCostMicrousd  int64  `json:"image_per_asset_cost_microusd"`
	ReservationCredits         int    `json:"reservation_credits"`
	MinimumChargeCredits       int    `json:"minimum_charge_credits"`
	MarkupBPS                  *int   `json:"markup_bps,omitempty"`
	Enabled                    bool   `json:"enabled"`
}

type UpsertHostedCreditPackRequest struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Currency     string `json:"currency"`
	AmountTotal  int64  `json:"amount_total"`
	CreditAmount int    `json:"credit_amount"`
	Enabled      bool   `json:"enabled"`
}

type HostedBillingConfig struct {
	Settings     model.HostedPricingSetting       `json:"settings"`
	ModelConfigs []model.HostedModelPricingConfig `json:"model_configs"`
	Rules        []model.HostedPricingRule        `json:"rules"`
	Packs        []model.HostedCreditPack         `json:"packs"`
}

type CreateAPIKeyResponse struct {
	PlaintextKey string `json:"plaintext_key"`
	KeyPrefix    string `json:"key_prefix"`
}

type APIKeyPlaintextResponse struct {
	PlaintextKey string `json:"plaintext_key"`
}
