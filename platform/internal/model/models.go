package model

import "time"

type APIKeyStatus string
type UserAIGatewayAPIKeyStatus string
type UserStatus string
type OrderStatus string
type BillingEventStatus string
type AccessMode string
type UsageMode string
type UsageAction string
type UsageResult string
type RewardSourceType string
type PackKind string

const (
	APIKeyStatusActive   APIKeyStatus = "active"
	APIKeyStatusDisabled APIKeyStatus = "disabled"

	UserAIGatewayAPIKeyStatusCreating UserAIGatewayAPIKeyStatus = "creating"
	UserAIGatewayAPIKeyStatusActive   UserAIGatewayAPIKeyStatus = "active"
	UserAIGatewayAPIKeyStatusError    UserAIGatewayAPIKeyStatus = "error"

	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"

	OrderStatusPending  OrderStatus = "pending"
	OrderStatusPaid     OrderStatus = "paid"
	OrderStatusFailed   OrderStatus = "failed"
	OrderStatusRefunded OrderStatus = "refunded"

	BillingEventStatusHandled BillingEventStatus = "handled"
	BillingEventStatusIgnored BillingEventStatus = "ignored"
	BillingEventStatusFailed  BillingEventStatus = "failed"

	AccessModeFree    AccessMode = "free"
	AccessModeReward  AccessMode = "reward"
	AccessModePaid    AccessMode = "paid"
	AccessModeHosted  AccessMode = "hosted"
	AccessModeBlocked AccessMode = "blocked"

	UsageModeFree   UsageMode = "free"
	UsageModeReward UsageMode = "reward"
	UsageModePaid   UsageMode = "paid"
	UsageModeHosted UsageMode = "hosted"

	UsageActionGenerate UsageAction = "generate"
	UsageActionStatus   UsageAction = "status"

	UsageResultAllowed UsageResult = "allowed"
	UsageResultBlocked UsageResult = "blocked"

	RewardSourceInviteActivation RewardSourceType = "invite_activation_reward"
	RewardSourceDiscordJoin      RewardSourceType = "discord_join_reward"
	RewardSourceAdminManual      RewardSourceType = "admin_manual_grant"

	PackKindExternalGeneration PackKind = "external_generation"
	PackKindHostedCredits      PackKind = "hosted_credits"
)

type User struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	GoogleSub  string     `gorm:"column:google_sub;size:191;uniqueIndex;not null" json:"google_sub"`
	Email      string     `gorm:"column:email;size:191;uniqueIndex;not null" json:"email"`
	Name       string     `gorm:"column:name;size:191;not null" json:"name"`
	InviteCode string     `gorm:"column:invite_code;size:64;uniqueIndex;not null" json:"invite_code"`
	AvatarURL  *string    `gorm:"column:avatar_url;size:512" json:"avatar_url,omitempty"`
	Status     UserStatus `gorm:"column:status;size:16;index;not null" json:"status"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string { return "users" }

type APIKey struct {
	ID                 uint64       `gorm:"primaryKey" json:"id"`
	OwnerUserID        *uint64      `gorm:"column:owner_user_id;index" json:"owner_user_id,omitempty"`
	KeyHash            string       `gorm:"column:key_hash;size:128;uniqueIndex;not null" json:"-"`
	KeyPrefix          string       `gorm:"column:key_prefix;size:32;index;not null" json:"key_prefix"`
	KeyCiphertext      *string      `gorm:"column:key_ciphertext;type:text" json:"-"`
	Status             APIKeyStatus `gorm:"column:status;size:16;index;not null" json:"status"`
	PlanName           string       `gorm:"column:plan_name;size:128;not null" json:"plan_name"`
	PlanCode           *string      `gorm:"column:plan_code;size:64" json:"plan_code,omitempty"`
	AllowedModes       string       `gorm:"column:allowed_modes;size:32;not null;default:external_only" json:"allowed_modes"`
	HostedEnabled      bool         `gorm:"column:hosted_enabled;not null;default:false" json:"hosted_enabled"`
	DefaultRuntimeMode *string      `gorm:"column:default_runtime_mode;size:16" json:"default_runtime_mode,omitempty"`
	CreditBalance      int          `gorm:"column:credit_balance;not null;default:0" json:"credit_balance"`
	CreditReserved     int          `gorm:"column:credit_reserved;not null;default:0" json:"credit_reserved"`
	ExpiresAt          *time.Time   `gorm:"column:expires_at" json:"expires_at,omitempty"`
	Note               *string      `gorm:"column:note;type:text" json:"note,omitempty"`
	LastUsedAt         *time.Time   `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	QuotaTotal         *int         `gorm:"column:quota_total" json:"quota_total,omitempty"`
	QuotaUsed          int          `gorm:"column:quota_used;not null;default:0" json:"quota_used"`
	QuotaRemaining     *int         `gorm:"-" json:"quota_remaining,omitempty"`
	PlaintextAvailable bool         `gorm:"-" json:"plaintext_available"`
	CreatedAt          time.Time    `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time    `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (APIKey) TableName() string { return "api_keys" }

func (k APIKey) PaidQuotaRemaining() int {
	if k.QuotaTotal == nil {
		return 0
	}
	remaining := *k.QuotaTotal - k.QuotaUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (k APIKey) SupportsExternal() bool {
	switch k.AllowedModes {
	case "hybrid", "external_only", "":
		return true
	default:
		return false
	}
}

func (k APIKey) SupportsHosted() bool {
	if !k.HostedEnabled {
		return false
	}
	switch k.AllowedModes {
	case "hybrid", "hosted_only":
		return true
	default:
		return false
	}
}

func (k APIKey) AvailableCredits() int {
	remaining := k.CreditBalance - k.CreditReserved
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (k APIKey) HasStoredPlaintext() bool {
	return k.KeyCiphertext != nil && *k.KeyCiphertext != ""
}

type UserAIGatewayAPIKey struct {
	ID              uint64                    `gorm:"primaryKey" json:"id"`
	UserID          uint64                    `gorm:"column:user_id;uniqueIndex;not null" json:"user_id"`
	KeyCiphertext   string                    `gorm:"column:key_ciphertext;type:text;not null" json:"-"`
	KeyPrefix       string                    `gorm:"column:key_prefix;size:32;not null" json:"key_prefix"`
	Status          UserAIGatewayAPIKeyStatus `gorm:"column:status;size:16;index;not null" json:"status"`
	UpstreamID      string                    `gorm:"column:upstream_id;size:128;not null" json:"upstream_id,omitempty"`
	UpstreamName    string                    `gorm:"column:upstream_name;size:191;not null" json:"upstream_name"`
	LastError       string                    `gorm:"column:last_error;type:text;not null" json:"last_error,omitempty"`
	CreationClaimed bool                      `gorm:"-" json:"-"`
	CreatedAt       time.Time                 `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time                 `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (UserAIGatewayAPIKey) TableName() string { return "user_aigateway_api_keys" }

type FreeQuota struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	FingerprintHash string    `gorm:"column:fingerprint_hash;size:128;uniqueIndex;not null" json:"fingerprint_hash"`
	FreeLimit       int       `gorm:"column:free_limit;not null" json:"free_limit"`
	FreeUsed        int       `gorm:"column:free_used;not null" json:"free_used"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (FreeQuota) TableName() string { return "free_quotas" }

func (f FreeQuota) FreeRemaining() int {
	remaining := f.FreeLimit - f.FreeUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

type DailyFreeQuota struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	FingerprintHash string    `gorm:"column:fingerprint_hash;size:128;index;not null" json:"fingerprint_hash"`
	UsageDate       string    `gorm:"column:usage_date;size:10;not null" json:"usage_date"`
	DocumentType    string    `gorm:"column:document_type;size:32;not null;default:document;index" json:"document_type"`
	DailyLimit      int       `gorm:"column:daily_limit;not null" json:"daily_limit"`
	DailyUsed       int       `gorm:"column:daily_used;not null" json:"daily_used"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (DailyFreeQuota) TableName() string { return "daily_free_quotas" }

func (d DailyFreeQuota) Remaining() int {
	remaining := d.DailyLimit - d.DailyUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

type UsageEvent struct {
	ID                    uint64      `gorm:"primaryKey" json:"id"`
	RequestID             *string     `gorm:"column:request_id;size:128;uniqueIndex" json:"request_id,omitempty"`
	FingerprintHash       string      `gorm:"column:fingerprint_hash;size:128;index;not null" json:"fingerprint_hash"`
	Mode                  UsageMode   `gorm:"column:mode;size:16;index;not null" json:"mode"`
	Action                UsageAction `gorm:"column:action;size:16;index;not null" json:"action"`
	APIKeyID              *uint64     `gorm:"column:api_key_id;index" json:"api_key_id,omitempty"`
	Result                UsageResult `gorm:"column:result;size:16;index;not null" json:"result"`
	ReasonCode            *string     `gorm:"column:reason_code;size:64;index" json:"reason_code,omitempty"`
	CLIVersion            *string     `gorm:"column:cli_version;size:64" json:"cli_version,omitempty"`
	DocumentType          *string     `gorm:"column:document_type;size:32" json:"document_type,omitempty"`
	RuntimeMode           *string     `gorm:"column:runtime_mode;size:16;index" json:"runtime_mode,omitempty"`
	UserID                *uint64     `gorm:"column:user_id;index" json:"user_id,omitempty"`
	BilledUnits           int         `gorm:"column:billed_units;not null;default:0" json:"billed_units"`
	UnitType              string      `gorm:"column:unit_type;size:32;not null;default:document" json:"unit_type"`
	Charged               bool        `gorm:"column:charged;not null;default:false" json:"charged"`
	Provider              *string     `gorm:"column:provider;size:64" json:"provider,omitempty"`
	ModelName             *string     `gorm:"column:model_name;size:128" json:"model_name,omitempty"`
	PromptTokens          int         `gorm:"column:prompt_tokens;not null;default:0" json:"prompt_tokens"`
	CompletionTokens      int         `gorm:"column:completion_tokens;not null;default:0" json:"completion_tokens"`
	ReasoningTokens       int         `gorm:"column:reasoning_tokens;not null;default:0" json:"reasoning_tokens"`
	ImageCount            int         `gorm:"column:image_count;not null;default:0" json:"image_count"`
	ReservedCredits       int         `gorm:"column:reserved_credits;not null;default:0" json:"reserved_credits"`
	SettledCredits        int         `gorm:"column:settled_credits;not null;default:0" json:"settled_credits"`
	RefundCredits         int         `gorm:"column:refund_credits;not null;default:0" json:"refund_credits"`
	HostedPricingRuleID   uint64      `gorm:"column:hosted_pricing_rule_id;not null;default:0" json:"hosted_pricing_rule_id,omitempty"`
	MarkupBPS             int         `gorm:"column:markup_bps;not null;default:0" json:"markup_bps"`
	UpstreamCostMicrousd  int64       `gorm:"column:upstream_cost_microusd;not null;default:0" json:"upstream_cost_microusd"`
	UncappedChargeCredits int         `gorm:"column:uncapped_charge_credits;not null;default:0" json:"uncapped_charge_credits"`
	ProfitMicrousd        int64       `gorm:"column:profit_microusd;not null;default:0" json:"profit_microusd"`
	CapApplied            bool        `gorm:"column:cap_applied;not null;default:false" json:"cap_applied"`
	CreatedAt             time.Time   `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (UsageEvent) TableName() string { return "usage_events" }

type RewardGrant struct {
	ID             uint64           `gorm:"primaryKey" json:"id"`
	UserID         uint64           `gorm:"column:user_id;index;not null" json:"user_id"`
	SourceType     RewardSourceType `gorm:"column:source_type;size:64;index;not null" json:"source_type"`
	IdempotencyKey string           `gorm:"column:idempotency_key;size:191;uniqueIndex;not null" json:"idempotency_key"`
	AmountTotal    int              `gorm:"column:amount_total;not null" json:"amount_total"`
	AmountUsed     int              `gorm:"column:amount_used;not null;default:0" json:"amount_used"`
	Reason         string           `gorm:"column:reason;size:191;not null" json:"reason"`
	MetadataJSON   string           `gorm:"column:metadata_json;type:json;not null" json:"metadata_json"`
	CreatedAt      time.Time        `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time        `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (RewardGrant) TableName() string { return "reward_grants" }

func (g RewardGrant) Remaining() int {
	remaining := g.AmountTotal - g.AmountUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

type UserReferral struct {
	ID              uint64     `gorm:"primaryKey" json:"id"`
	InviterUserID   uint64     `gorm:"column:inviter_user_id;index;not null" json:"inviter_user_id"`
	InvitedUserID   uint64     `gorm:"column:invited_user_id;uniqueIndex;not null" json:"invited_user_id"`
	InviteCode      string     `gorm:"column:invite_code;size:64;index;not null" json:"invite_code"`
	RegisteredAt    time.Time  `gorm:"column:registered_at;not null" json:"registered_at"`
	ActivatedAt     *time.Time `gorm:"column:activated_at" json:"activated_at,omitempty"`
	RewardGrantedAt *time.Time `gorm:"column:reward_granted_at" json:"reward_granted_at,omitempty"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (UserReferral) TableName() string { return "user_referrals" }

type DiscordConnection struct {
	ID              uint64     `gorm:"primaryKey" json:"id"`
	UserID          uint64     `gorm:"column:user_id;uniqueIndex;not null" json:"user_id"`
	DiscordUserID   string     `gorm:"column:discord_user_id;size:191;uniqueIndex;not null" json:"discord_user_id"`
	Username        string     `gorm:"column:username;size:191;not null" json:"username"`
	GuildMember     bool       `gorm:"column:guild_member;not null;default:false" json:"guild_member"`
	ConnectedAt     time.Time  `gorm:"column:connected_at;not null" json:"connected_at"`
	RewardGrantedAt *time.Time `gorm:"column:reward_granted_at" json:"reward_granted_at,omitempty"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (DiscordConnection) TableName() string { return "discord_connections" }

type StripeCustomer struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	UserID           uint64    `gorm:"column:user_id;uniqueIndex;not null" json:"user_id"`
	StripeCustomerID string    `gorm:"column:stripe_customer_id;size:191;uniqueIndex;not null" json:"stripe_customer_id"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (StripeCustomer) TableName() string { return "stripe_customers" }

type Order struct {
	ID                      uint64      `gorm:"primaryKey" json:"id"`
	UserID                  uint64      `gorm:"column:user_id;index;not null" json:"user_id"`
	StripeCheckoutSessionID *string     `gorm:"column:stripe_checkout_session_id;size:191;uniqueIndex" json:"stripe_checkout_session_id,omitempty"`
	StripePaymentIntentID   *string     `gorm:"column:stripe_payment_intent_id;size:191;index" json:"stripe_payment_intent_id,omitempty"`
	StripeCustomerID        *string     `gorm:"column:stripe_customer_id;size:191;index" json:"stripe_customer_id,omitempty"`
	Status                  OrderStatus `gorm:"column:status;size:16;index;not null" json:"status"`
	Currency                string      `gorm:"column:currency;size:16;not null" json:"currency"`
	AmountTotal             int64       `gorm:"column:amount_total;not null" json:"amount_total"`
	PackCode                string      `gorm:"column:pack_code;size:64;index;not null" json:"pack_code"`
	PackName                string      `gorm:"column:pack_name;size:128;not null" json:"pack_name"`
	PackKind                PackKind    `gorm:"column:pack_kind;size:32;not null;default:external_generation" json:"pack_kind"`
	QuotaAmount             int         `gorm:"column:quota_amount;not null" json:"quota_amount"`
	CreditAmount            int         `gorm:"column:credit_amount;not null;default:0" json:"credit_amount"`
	TargetAPIKeyID          *uint64     `gorm:"column:target_api_key_id;index" json:"target_api_key_id,omitempty"`
	Note                    *string     `gorm:"column:note;type:text" json:"note,omitempty"`
	CreatedAt               time.Time   `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt               time.Time   `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Order) TableName() string { return "orders" }

type BillingEvent struct {
	ID           uint64             `gorm:"primaryKey" json:"id"`
	OrderID      *uint64            `gorm:"column:order_id;index" json:"order_id,omitempty"`
	EventID      string             `gorm:"column:event_id;size:191;uniqueIndex;not null" json:"event_id"`
	EventType    string             `gorm:"column:event_type;size:128;index;not null" json:"event_type"`
	Status       BillingEventStatus `gorm:"column:status;size:16;index;not null" json:"status"`
	PayloadJSON  string             `gorm:"column:payload_json;type:json;not null" json:"payload_json"`
	ErrorMessage *string            `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	ProcessedAt  *time.Time         `gorm:"column:processed_at" json:"processed_at,omitempty"`
	CreatedAt    time.Time          `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (BillingEvent) TableName() string { return "billing_events" }

type AdminAuditLog struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	Action      string    `gorm:"column:action;size:64;index;not null" json:"action"`
	TargetType  string    `gorm:"column:target_type;size:64;index;not null" json:"target_type"`
	TargetID    string    `gorm:"column:target_id;size:128;index;not null" json:"target_id"`
	PayloadJSON string    `gorm:"column:payload_json;type:json;not null" json:"payload_json"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (AdminAuditLog) TableName() string { return "admin_audit_logs" }

type PricingPack struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Currency     string `json:"currency"`
	AmountTotal  int64  `json:"amount_total"`
	QuotaAmount  int    `json:"quota_amount"`
	CreditAmount int    `json:"credit_amount,omitempty"`
	PackKind     string `json:"pack_kind,omitempty"`
}

type HostedModelPricingKind string

const (
	HostedModelPricingKindText  HostedModelPricingKind = "text"
	HostedModelPricingKindImage HostedModelPricingKind = "image"
)

type HostedModelPricingConfig struct {
	ID                         uint64                 `gorm:"primaryKey" json:"id,omitempty"`
	Key                        string                 `gorm:"column:key;size:64;uniqueIndex;not null" json:"key"`
	Kind                       HostedModelPricingKind `gorm:"column:kind;size:32;index;not null" json:"kind"`
	Provider                   string                 `gorm:"column:provider;size:64;not null" json:"provider"`
	Model                      string                 `gorm:"column:model;size:128;not null" json:"model"`
	PromptPer1MCostMicrousd    int64                  `gorm:"column:prompt_per_1m_cost_microusd;not null;default:0" json:"prompt_per_1m_cost_microusd"`
	OutputPer1MCostMicrousd    int64                  `gorm:"column:output_per_1m_cost_microusd;not null;default:0" json:"output_per_1m_cost_microusd"`
	ReasoningPer1MCostMicrousd int64                  `gorm:"column:reasoning_per_1m_cost_microusd;not null;default:0" json:"reasoning_per_1m_cost_microusd"`
	Enabled                    bool                   `gorm:"column:enabled;not null;default:true" json:"enabled"`
	CreatedAt                  time.Time              `gorm:"column:created_at;autoCreateTime" json:"created_at,omitempty"`
	UpdatedAt                  time.Time              `gorm:"column:updated_at;autoUpdateTime" json:"updated_at,omitempty"`
}

func (HostedModelPricingConfig) TableName() string { return "hosted_model_pricing_configs" }

type HostedPricingRule struct {
	ID                         uint64    `gorm:"primaryKey" json:"id,omitempty"`
	DocumentProfile            string    `gorm:"column:document_profile;size:64;index;not null" json:"document_profile"`
	Provider                   string    `gorm:"column:provider;size:64;not null" json:"provider"`
	Model                      string    `gorm:"column:model;size:128;not null" json:"model"`
	TextModelKey               string    `gorm:"column:text_model_key;size:64;index;not null;default:''" json:"text_model_key"`
	ImageModelKey              string    `gorm:"column:image_model_key;size:64;index;not null;default:''" json:"image_model_key"`
	PromptPer1KCredits         int       `gorm:"-" json:"prompt_per_1k_credits,omitempty"`
	OutputPer1KCredits         int       `gorm:"-" json:"output_per_1k_credits,omitempty"`
	ReasoningPer1KCredits      int       `gorm:"-" json:"reasoning_per_1k_credits,omitempty"`
	ImagePerAssetCredits       int       `gorm:"-" json:"image_per_asset_credits,omitempty"`
	PromptPer1KCostMicrousd    int64     `gorm:"column:prompt_per_1k_cost_microusd;not null;default:0" json:"prompt_per_1k_cost_microusd"`
	OutputPer1KCostMicrousd    int64     `gorm:"column:output_per_1k_cost_microusd;not null;default:0" json:"output_per_1k_cost_microusd"`
	ReasoningPer1KCostMicrousd int64     `gorm:"column:reasoning_per_1k_cost_microusd;not null;default:0" json:"reasoning_per_1k_cost_microusd"`
	ImagePerAssetCostMicrousd  int64     `gorm:"column:image_per_asset_cost_microusd;not null;default:0" json:"image_per_asset_cost_microusd"`
	ReservationCredits         int       `gorm:"column:reservation_credits;not null;default:0" json:"reservation_credits"`
	MinimumChargeCredits       int       `gorm:"column:minimum_charge_credits;not null;default:0" json:"minimum_charge_credits"`
	MarkupBPS                  *int      `gorm:"column:markup_bps" json:"markup_bps,omitempty"`
	Enabled                    bool      `gorm:"column:enabled;not null;default:true" json:"enabled"`
	CreatedAt                  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at,omitempty"`
	UpdatedAt                  time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at,omitempty"`
}

func (HostedPricingRule) TableName() string { return "hosted_pricing_rules" }

type HostedPricingSetting struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	MarkupBPS int       `gorm:"column:markup_bps;not null;default:3000" json:"markup_bps"`
	Currency  string    `gorm:"column:currency;size:8;not null;default:usd" json:"currency"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at,omitempty"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at,omitempty"`
}

func (HostedPricingSetting) TableName() string { return "hosted_pricing_settings" }

type HostedCreditPack struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	Code         string    `gorm:"column:code;size:64;uniqueIndex;not null" json:"code"`
	Name         string    `gorm:"column:name;size:128;not null" json:"name"`
	Description  string    `gorm:"column:description;size:512;not null" json:"description"`
	Currency     string    `gorm:"column:currency;size:8;not null;default:usd" json:"currency"`
	AmountTotal  int64     `gorm:"column:amount_total;not null" json:"amount_total"`
	CreditAmount int       `gorm:"column:credit_amount;not null" json:"credit_amount"`
	Enabled      bool      `gorm:"column:enabled;not null;default:true" json:"enabled"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at,omitempty"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at,omitempty"`
}

func (HostedCreditPack) TableName() string { return "hosted_credit_packs" }

func (p HostedCreditPack) PricingPack() PricingPack {
	return PricingPack{
		Code:         p.Code,
		Name:         p.Name,
		Description:  p.Description,
		Currency:     p.Currency,
		AmountTotal:  p.AmountTotal,
		CreditAmount: p.CreditAmount,
		PackKind:     string(PackKindHostedCredits),
	}
}

type OverviewStats struct {
	TotalAPIKeys          int64 `json:"total_api_keys"`
	ActiveAPIKeys         int64 `json:"active_api_keys"`
	DisabledAPIKeys       int64 `json:"disabled_api_keys"`
	ExpiredAPIKeys        int64 `json:"expired_api_keys"`
	FreeMachines          int64 `json:"free_machines"`
	ChecksLast24h         int64 `json:"checks_last_24h"`
	ConsumesLast24h       int64 `json:"consumes_last_24h"`
	BlockedLast24h        int64 `json:"blocked_last_24h"`
	TotalUsers            int64 `json:"total_users"`
	PaidOrdersLast24h     int64 `json:"paid_orders_last_24h"`
	PaidQuotaAddedLast24h int64 `json:"paid_quota_added_last_24h"`
	RemainingPaidQuota    int64 `json:"remaining_paid_quota"`
}
