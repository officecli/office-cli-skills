package admin

import (
	"encoding/json"
	"io"
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
}

type AdminPreferenceResponse struct {
	Preferences json.RawMessage `json:"preferences"`
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
	PromptPer1KCredits         int    `json:"prompt_per_1k_credits"`
	OutputPer1KCredits         int    `json:"output_per_1k_credits"`
	ReasoningPer1KCredits      int    `json:"reasoning_per_1k_credits"`
	ImagePerAssetCredits       int    `json:"image_per_asset_credits"`
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

type ImagePromptSlot struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Example      string `json:"example,omitempty"`
	DefaultValue string `json:"default_value,omitempty"`
	HelpText     string `json:"help_text,omitempty"`
	Required     bool   `json:"required,omitempty"`
	Multiline    bool   `json:"multiline,omitempty"`
}

type ImagePromptTemplateResponse struct {
	ID           uint64            `json:"id"`
	OwnerUserID  uint64            `json:"owner_user_id,omitempty"`
	Visibility   string            `json:"visibility,omitempty"`
	Slug         string            `json:"slug"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	PromptPreset string            `json:"prompt_preset,omitempty"`
	ThumbnailURL string            `json:"thumbnail_url,omitempty"`
	SortOrder    int               `json:"sort_order"`
	Enabled      bool              `json:"enabled"`
	Slots        []ImagePromptSlot `json:"slots,omitempty"`
	CreatedAt    string            `json:"created_at,omitempty"`
	UpdatedAt    string            `json:"updated_at,omitempty"`
}

type AdminImagePromptTemplateResponse struct {
	ImagePromptTemplateResponse
}

type UpsertImagePromptTemplateRequest struct {
	Slug         string            `json:"slug"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	PromptPreset string            `json:"prompt_preset"`
	SortOrder    int               `json:"sort_order"`
	Enabled      bool              `json:"enabled"`
	Slots        []ImagePromptSlot `json:"slots,omitempty"`
}

type UserImagePromptTemplatesResponse struct {
	Public  []ImagePromptTemplateResponse `json:"public"`
	Private []ImagePromptTemplateResponse `json:"private"`
}

type CreateUserImagePromptTemplateRequest struct {
	SourceTemplateID uint64            `json:"source_template_id,omitempty"`
	Slug             string            `json:"slug"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	PromptPreset     string            `json:"prompt_preset,omitempty"`
	Slots            []ImagePromptSlot `json:"slots,omitempty"`
	SortOrder        int               `json:"sort_order,omitempty"`
}

type RecordImageGenerationProvenanceRequest struct {
	RequestID        string    `json:"request_id"`
	UserID           uint64    `json:"user_id"`
	Prompt           string    `json:"prompt"`
	ImageName        string    `json:"image_name,omitempty"`
	ContentType      string    `json:"content_type,omitempty"`
	Image            io.Reader `json:"-"`
	ImageSize        int64     `json:"-"`
	SourceTemplateID uint64    `json:"source_template_id,omitempty"`
}

type ImageGenerationProvenanceResponse struct {
	ID               uint64 `json:"id"`
	RequestID        string `json:"request_id"`
	UserID           uint64 `json:"user_id"`
	PromptSHA256     string `json:"prompt_sha256"`
	ImageSHA256      string `json:"image_sha256"`
	ImageContentType string `json:"image_content_type,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
}

type CreateImageTemplatePublishRequest struct {
	PrivateTemplateID uint64 `json:"private_template_id"`
	ProvenanceID      uint64 `json:"provenance_id"`
	RequestID         string `json:"request_id,omitempty"`
	SubmitterNote     string `json:"submitter_note,omitempty"`
}

type ReviewImageTemplatePublishRequest struct {
	Action     string `json:"action"`
	AdminNotes string `json:"admin_notes,omitempty"`
}

type ImageTemplatePublishRequestResponse struct {
	ID                uint64 `json:"id"`
	PrivateTemplateID uint64 `json:"private_template_id"`
	RequesterUserID   uint64 `json:"requester_user_id"`
	ProvenanceID      uint64 `json:"provenance_id"`
	Status            string `json:"status"`
	SubmitterNote     string `json:"submitter_note,omitempty"`
	AdminNotes        string `json:"admin_notes,omitempty"`
	ReviewedBy        string `json:"reviewed_by,omitempty"`
	PublicTemplateID  uint64 `json:"public_template_id,omitempty"`
	ReviewedAt        string `json:"reviewed_at,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type ComposeImagePromptTemplateRequest struct {
	Prompt string `json:"prompt"`
}

type ComposeImagePromptTemplateResponse struct {
	Prompt string `json:"prompt"`
}

type CreateAPIKeyResponse struct {
	PlaintextKey string `json:"plaintext_key"`
	KeyPrefix    string `json:"key_prefix"`
}

type APIKeyPlaintextResponse struct {
	PlaintextKey string `json:"plaintext_key"`
}

type CreateRedemptionCodeRequest struct {
	Code           string     `json:"code,omitempty"`
	CreditAmount   int        `json:"credit_amount"`
	MaxRedemptions *int       `json:"max_redemptions,omitempty"`
	PerUserLimit   *int       `json:"per_user_limit,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Notes          string     `json:"notes,omitempty"`
}

type UpdateRedemptionCodeRequest struct {
	CreditAmount   *int       `json:"credit_amount,omitempty"`
	MaxRedemptions *int       `json:"max_redemptions,omitempty"`
	ClearMaxLimit  bool       `json:"clear_max_limit,omitempty"`
	PerUserLimit   *int       `json:"per_user_limit,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	ClearExpiresAt bool       `json:"clear_expires_at,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
}

type ListRedemptionCodesRequest struct {
	Status       string `form:"status" json:"status,omitempty"`
	CodeContains string `form:"q" json:"q,omitempty"`
	Limit        int    `form:"limit" json:"limit,omitempty"`
	Offset       int    `form:"offset" json:"offset,omitempty"`
}

type ListRedemptionCodesResponse struct {
	Items []model.RedemptionCode `json:"items"`
	Total int64                  `json:"total"`
}

type ListRedemptionRecordsRequest struct {
	RedemptionCodeID uint64 `form:"redemption_code_id" json:"redemption_code_id,omitempty"`
	Code             string `form:"code" json:"code,omitempty"`
	UserID           uint64 `form:"user_id" json:"user_id,omitempty"`
	Source           string `form:"source" json:"source,omitempty"`
	Limit            int    `form:"limit" json:"limit,omitempty"`
	Offset           int    `form:"offset" json:"offset,omitempty"`
}

type ListRedemptionRecordsResponse struct {
	Items []model.RedemptionCodeRedemption `json:"items"`
	Total int64                            `json:"total"`
}
