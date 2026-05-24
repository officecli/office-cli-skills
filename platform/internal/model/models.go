package model

import (
	"strings"
	"time"
)

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
type HostedCreditGrantSource string
type HostedCreditLedgerSource string
type PackKind string
type CLILoginChallengeStatus string
type CLILoginChallengeFlow string

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

	HostedCreditGrantSourceSignup           HostedCreditGrantSource = "signup_bonus"
	HostedCreditGrantSourceInviteActivation HostedCreditGrantSource = "invite_activation_bonus"

	HostedCreditLedgerSourcePurchase                HostedCreditLedgerSource = "purchase"
	HostedCreditLedgerSourceSignupBonus             HostedCreditLedgerSource = "signup_bonus"
	HostedCreditLedgerSourceInviteActivationBonus   HostedCreditLedgerSource = "invite_activation_bonus"
	HostedCreditLedgerSourceDiscordJoinBonus        HostedCreditLedgerSource = "discord_join_bonus"
	HostedCreditLedgerSourceReserve                 HostedCreditLedgerSource = "reserve"
	HostedCreditLedgerSourceSettle                  HostedCreditLedgerSource = "settle"
	HostedCreditLedgerSourceRelease                 HostedCreditLedgerSource = "release"
	HostedCreditLedgerSourceCharge                  HostedCreditLedgerSource = "charge"
	HostedCreditLedgerSourceChargeFailedPostUpstream HostedCreditLedgerSource = "charge_failed_post_upstream"
	HostedCreditLedgerSourceReservedCutover         HostedCreditLedgerSource = "reserved_cutover"
	HostedCreditLedgerSourceMigration               HostedCreditLedgerSource = "migration"
	HostedCreditLedgerSourceAnonymousSignupBonus    HostedCreditLedgerSource = "anonymous_signup_bonus"
	HostedCreditLedgerSourceAnonymousTransferOut    HostedCreditLedgerSource = "anonymous_transfer_out"
	HostedCreditLedgerSourceAnonymousTransferIn     HostedCreditLedgerSource = "anonymous_transfer_in"
	HostedCreditLedgerSourceRedemptionCode          HostedCreditLedgerSource = "redemption_code"

	PackKindExternalGeneration PackKind = "external_generation"
	PackKindHostedCredits      PackKind = "hosted_credits"

	CLILoginChallengeStatusPending   CLILoginChallengeStatus = "pending"
	CLILoginChallengeStatusCompleted CLILoginChallengeStatus = "completed"
	CLILoginChallengeStatusConsumed  CLILoginChallengeStatus = "consumed"

	CLILoginChallengeFlowCallback CLILoginChallengeFlow = "callback"
	CLILoginChallengeFlowDevice   CLILoginChallengeFlow = "device"
)

type User struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	GoogleSub  *string    `gorm:"column:google_sub;size:191;uniqueIndex" json:"google_sub,omitempty"`
	GitHubSub  *string    `gorm:"column:github_sub;size:191;uniqueIndex" json:"github_sub,omitempty"`
	Email      string     `gorm:"column:email;size:191;uniqueIndex;not null" json:"email"`
	Name       string     `gorm:"column:name;size:191;not null" json:"name"`
	InviteCode string     `gorm:"column:invite_code;size:64;uniqueIndex;not null" json:"invite_code"`
	AvatarURL  *string    `gorm:"column:avatar_url;size:512" json:"avatar_url,omitempty"`
	Status     UserStatus `gorm:"column:status;size:16;index;not null" json:"status"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string { return "users" }

type AdminUserPreference struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	AdminEmail      string    `gorm:"column:admin_email;size:191;uniqueIndex:idx_admin_user_preferences_email_page;not null" json:"admin_email"`
	PageKey         string    `gorm:"column:page_key;size:128;uniqueIndex:idx_admin_user_preferences_email_page;not null" json:"page_key"`
	PreferencesJSON string    `gorm:"column:preferences_json;type:json;not null" json:"preferences_json"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (AdminUserPreference) TableName() string { return "admin_user_preferences" }

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

type CLILoginChallenge struct {
	ID                  uint64                  `gorm:"primaryKey" json:"id"`
	ChallengeID         string                  `gorm:"column:challenge_id;size:128;uniqueIndex;not null" json:"challenge_id"`
	Flow                CLILoginChallengeFlow   `gorm:"column:flow;size:16;index;not null" json:"flow"`
	CodeChallenge       string                  `gorm:"column:code_challenge;size:191;not null" json:"-"`
	CodeChallengeMethod string                  `gorm:"column:code_challenge_method;size:16;not null" json:"code_challenge_method"`
	RedirectURI         string                  `gorm:"column:redirect_uri;size:512;not null" json:"redirect_uri"`
	State               string                  `gorm:"column:state;size:191;not null" json:"state"`
	Status              CLILoginChallengeStatus `gorm:"column:status;size:16;index;not null" json:"status"`
	UserID              *uint64                 `gorm:"column:user_id;index" json:"user_id,omitempty"`
	UserCodeHash        *string                 `gorm:"column:user_code_hash;size:128;uniqueIndex" json:"-"`
	ExchangeCodeHash    *string                 `gorm:"column:exchange_code_hash;size:128;uniqueIndex" json:"-"`
	ExpiresAt           time.Time               `gorm:"column:expires_at;index;not null" json:"expires_at"`
	CompletedAt         *time.Time              `gorm:"column:completed_at" json:"completed_at,omitempty"`
	ConsumedAt          *time.Time              `gorm:"column:consumed_at" json:"consumed_at,omitempty"`
	CreatedAt           time.Time               `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time               `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (CLILoginChallenge) TableName() string { return "cli_login_challenges" }

type CLISession struct {
	ID          uint64     `gorm:"primaryKey" json:"id"`
	UserID      uint64     `gorm:"column:user_id;index;not null" json:"user_id"`
	TokenHash   string     `gorm:"column:token_hash;size:128;uniqueIndex;not null" json:"-"`
	TokenPrefix string     `gorm:"column:token_prefix;size:32;not null" json:"token_prefix"`
	Name        string     `gorm:"column:name;size:191;not null" json:"name"`
	RevokedAt   *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	ExpiresAt   time.Time  `gorm:"column:expires_at;index;not null" json:"expires_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (CLISession) TableName() string { return "cli_sessions" }

type FingerprintCreditAccount struct {
	FingerprintHash  string    `gorm:"column:fingerprint_hash;size:128;primaryKey" json:"fingerprint_hash"`
	CreditBalance    int       `gorm:"column:credit_balance;not null;default:0" json:"credit_balance"`
	CreditReserved   int       `gorm:"column:credit_reserved;not null;default:0" json:"credit_reserved"`
	BonusGranted     bool      `gorm:"column:bonus_granted;not null;default:false" json:"bonus_granted"`
	MigratedToUserID *uint64   `gorm:"column:migrated_to_user_id;index" json:"migrated_to_user_id,omitempty"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (FingerprintCreditAccount) TableName() string { return "fingerprint_credit_accounts" }

func (a FingerprintCreditAccount) AvailableCredits() int {
	remaining := a.CreditBalance - a.CreditReserved
	if remaining < 0 {
		return 0
	}
	return remaining
}

type FingerprintCreditLedger struct {
	ID              uint64                   `gorm:"primaryKey" json:"id"`
	FingerprintHash string                   `gorm:"column:fingerprint_hash;size:128;index;not null" json:"fingerprint_hash"`
	SourceType      HostedCreditLedgerSource `gorm:"column:source_type;size:64;index;not null" json:"source_type"`
	IdempotencyKey  string                   `gorm:"column:idempotency_key;size:191;uniqueIndex;not null" json:"idempotency_key"`
	CreditDelta     int                      `gorm:"column:credit_delta;not null;default:0" json:"credit_delta"`
	ReservedDelta   int                      `gorm:"column:reserved_delta;not null;default:0" json:"reserved_delta"`
	UsageEventID    *uint64                  `gorm:"column:usage_event_id;index" json:"usage_event_id,omitempty"`
	OrderID         *uint64                  `gorm:"column:order_id;index" json:"order_id,omitempty"`
	Reason          string                   `gorm:"column:reason;size:191;not null" json:"reason"`
	MetadataJSON    string                   `gorm:"column:metadata_json;type:json;not null" json:"metadata_json"`
	CreatedAt       time.Time                `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (FingerprintCreditLedger) TableName() string { return "fingerprint_credit_ledger" }

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
	ClientIP              *string     `gorm:"column:client_ip;size:64;index" json:"client_ip,omitempty"`
	ForwardedFor          *string     `gorm:"column:forwarded_for;size:512" json:"forwarded_for,omitempty"`
	UserAgent             *string     `gorm:"column:user_agent;size:512" json:"user_agent,omitempty"`
	RequestHost           *string     `gorm:"column:request_host;size:191" json:"request_host,omitempty"`
	RequestPath           *string     `gorm:"column:request_path;size:191" json:"request_path,omitempty"`
	RequestMethod         *string     `gorm:"column:request_method;size:16" json:"request_method,omitempty"`
	CreatedAt             time.Time   `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (UsageEvent) TableName() string { return "usage_events" }

type OperationalEvent struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	EventName       string    `gorm:"column:event_name;size:64;index:idx_operational_events_event_name_created_at,priority:1;not null" json:"event_name"`
	Surface         string    `gorm:"column:surface;size:16;index:idx_operational_events_surface_created_at,priority:1;not null" json:"surface"`
	VisitorID       *string   `gorm:"column:visitor_id;size:96;index:idx_operational_events_visitor_id_created_at,priority:1" json:"visitor_id,omitempty"`
	FingerprintHash *string   `gorm:"column:fingerprint_hash;size:128;index:idx_operational_events_fingerprint_hash_created_at,priority:1" json:"fingerprint_hash,omitempty"`
	UserID          *uint64   `gorm:"column:user_id;index:idx_operational_events_user_id_created_at,priority:1" json:"user_id,omitempty"`
	APIKeyID        *uint64   `gorm:"column:api_key_id" json:"api_key_id,omitempty"`
	OrderID         *uint64   `gorm:"column:order_id" json:"order_id,omitempty"`
	Invite          *string   `gorm:"column:invite;size:191" json:"invite,omitempty"`
	UTMSource       *string   `gorm:"column:utm_source;size:191" json:"utm_source,omitempty"`
	UTMMedium       *string   `gorm:"column:utm_medium;size:191" json:"utm_medium,omitempty"`
	UTMCampaign     *string   `gorm:"column:utm_campaign;size:191" json:"utm_campaign,omitempty"`
	UTMTerm         *string   `gorm:"column:utm_term;size:191" json:"utm_term,omitempty"`
	UTMContent      *string   `gorm:"column:utm_content;size:191" json:"utm_content,omitempty"`
	UTMID           *string   `gorm:"column:utm_id;size:191" json:"utm_id,omitempty"`
	PagePath        *string   `gorm:"column:page_path;size:512" json:"page_path,omitempty"`
	Referrer        *string   `gorm:"column:referrer;size:512" json:"referrer,omitempty"`
	UserAgent       *string   `gorm:"column:user_agent;size:512" json:"user_agent,omitempty"`
	MetadataJSON    string    `gorm:"column:metadata_json;type:json;not null" json:"metadata_json"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime;index:idx_operational_events_event_name_created_at,priority:2;index:idx_operational_events_surface_created_at,priority:2;index:idx_operational_events_visitor_id_created_at,priority:2;index:idx_operational_events_fingerprint_hash_created_at,priority:2;index:idx_operational_events_user_id_created_at,priority:2" json:"created_at"`
}

func (OperationalEvent) TableName() string { return "operational_events" }

type UsageAuditContext struct {
	ClientIP      string
	ForwardedFor  string
	UserAgent     string
	RequestHost   string
	RequestPath   string
	RequestMethod string
}

func (e *UsageEvent) ApplyAuditContext(ctx UsageAuditContext) {
	if e == nil {
		return
	}
	e.ClientIP = trimmedStringPtr(ctx.ClientIP)
	e.ForwardedFor = trimmedStringPtr(ctx.ForwardedFor)
	e.UserAgent = trimmedStringPtr(ctx.UserAgent)
	e.RequestHost = trimmedStringPtr(ctx.RequestHost)
	e.RequestPath = trimmedStringPtr(ctx.RequestPath)
	e.RequestMethod = trimmedStringPtr(ctx.RequestMethod)
}

func trimmedStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

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

type HostedCreditGrant struct {
	ID             uint64                  `gorm:"primaryKey" json:"id"`
	UserID         uint64                  `gorm:"column:user_id;index;not null" json:"user_id"`
	APIKeyID       uint64                  `gorm:"column:api_key_id;index;not null" json:"api_key_id"`
	SourceType     HostedCreditGrantSource `gorm:"column:source_type;size:64;index;not null" json:"source_type"`
	IdempotencyKey string                  `gorm:"column:idempotency_key;size:191;uniqueIndex;not null" json:"idempotency_key"`
	CreditAmount   int                     `gorm:"column:credit_amount;not null" json:"credit_amount"`
	Reason         string                  `gorm:"column:reason;size:191;not null" json:"reason"`
	MetadataJSON   string                  `gorm:"column:metadata_json;type:json;not null" json:"metadata_json"`
	CreatedAt      time.Time               `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (HostedCreditGrant) TableName() string { return "hosted_credit_grants" }

type UserHostedCreditAccount struct {
	UserID         uint64    `gorm:"column:user_id;primaryKey" json:"user_id"`
	CreditBalance  int       `gorm:"column:credit_balance;not null;default:0" json:"credit_balance"`
	CreditReserved int       `gorm:"column:credit_reserved;not null;default:0" json:"credit_reserved"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (UserHostedCreditAccount) TableName() string { return "user_hosted_credit_accounts" }

func (a UserHostedCreditAccount) AvailableCredits() int {
	remaining := a.CreditBalance - a.CreditReserved
	if remaining < 0 {
		return 0
	}
	return remaining
}

type UserHostedCreditLedger struct {
	ID             uint64                   `gorm:"primaryKey" json:"id"`
	UserID         uint64                   `gorm:"column:user_id;index;not null" json:"user_id"`
	SourceType     HostedCreditLedgerSource `gorm:"column:source_type;size:64;index;not null" json:"source_type"`
	IdempotencyKey string                   `gorm:"column:idempotency_key;size:191;uniqueIndex;not null" json:"idempotency_key"`
	CreditDelta    int                      `gorm:"column:credit_delta;not null;default:0" json:"credit_delta"`
	ReservedDelta  int                      `gorm:"column:reserved_delta;not null;default:0" json:"reserved_delta"`
	UsageEventID   *uint64                  `gorm:"column:usage_event_id;index" json:"usage_event_id,omitempty"`
	OrderID        *uint64                  `gorm:"column:order_id;index" json:"order_id,omitempty"`
	Reason         string                   `gorm:"column:reason;size:191;not null" json:"reason"`
	MetadataJSON   string                   `gorm:"column:metadata_json;type:json;not null" json:"metadata_json"`
	CreatedAt      time.Time                `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (UserHostedCreditLedger) TableName() string { return "user_hosted_credit_ledger" }

type RedemptionCodeStatus string

const (
	RedemptionCodeStatusEnabled  RedemptionCodeStatus = "enabled"
	RedemptionCodeStatusDisabled RedemptionCodeStatus = "disabled"
)

type RedemptionSource string

const (
	RedemptionSourceApp     RedemptionSource = "app"
	RedemptionSourceCLI     RedemptionSource = "cli"
	RedemptionSourceTUI     RedemptionSource = "tui"
	RedemptionSourceDesktop RedemptionSource = "desktop"
)

func (s RedemptionSource) Valid() bool {
	switch s {
	case RedemptionSourceApp, RedemptionSourceCLI, RedemptionSourceTUI, RedemptionSourceDesktop:
		return true
	}
	return false
}

type RedemptionCode struct {
	ID              uint64               `gorm:"primaryKey" json:"id"`
	Code            string               `gorm:"column:code;size:64;uniqueIndex;not null" json:"code"`
	CreditAmount    int                  `gorm:"column:credit_amount;not null" json:"credit_amount"`
	MaxRedemptions  *int                 `gorm:"column:max_redemptions" json:"max_redemptions,omitempty"`
	RedemptionsUsed int                  `gorm:"column:redemptions_used;not null;default:0" json:"redemptions_used"`
	PerUserLimit    int                  `gorm:"column:per_user_limit;not null;default:1" json:"per_user_limit"`
	Status          RedemptionCodeStatus `gorm:"column:status;size:16;index;not null;default:disabled" json:"status"`
	ExpiresAt       *time.Time           `gorm:"column:expires_at;index" json:"expires_at,omitempty"`
	Notes           string               `gorm:"column:notes" json:"notes"`
	CreatedBy       string               `gorm:"column:created_by;size:191;not null;default:''" json:"created_by"`
	CreatedAt       time.Time            `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time            `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (RedemptionCode) TableName() string { return "redemption_codes" }

func (c RedemptionCode) IsEnabled() bool { return c.Status == RedemptionCodeStatusEnabled }

func (c RedemptionCode) IsExpired(now time.Time) bool {
	return c.ExpiresAt != nil && !c.ExpiresAt.IsZero() && !now.Before(*c.ExpiresAt)
}

func (c RedemptionCode) IsExhausted() bool {
	return c.MaxRedemptions != nil && c.RedemptionsUsed >= *c.MaxRedemptions
}

func (c RedemptionCode) RemainingRedemptions() *int {
	if c.MaxRedemptions == nil {
		return nil
	}
	remaining := *c.MaxRedemptions - c.RedemptionsUsed
	if remaining < 0 {
		remaining = 0
	}
	return &remaining
}

type RedemptionCodeRedemption struct {
	ID                   uint64           `gorm:"primaryKey" json:"id"`
	RedemptionCodeID     uint64           `gorm:"column:redemption_code_id;index;not null" json:"redemption_code_id"`
	Code                 string           `gorm:"column:code;size:64;index;not null" json:"code"`
	UserID               uint64           `gorm:"column:user_id;index;not null" json:"user_id"`
	CreditAmount         int              `gorm:"column:credit_amount;not null" json:"credit_amount"`
	LedgerEntryID        uint64           `gorm:"column:ledger_entry_id;not null" json:"ledger_entry_id"`
	Source               RedemptionSource `gorm:"column:source;size:16;index;not null" json:"source"`
	ClientIP             string           `gorm:"column:client_ip;size:64;not null;default:''" json:"client_ip"`
	UserAgent            string           `gorm:"column:user_agent;size:512;not null;default:''" json:"user_agent"`
	IdempotencyKey       string           `gorm:"column:idempotency_key;size:191;uniqueIndex;not null" json:"idempotency_key"`
	PerUserLimitAtClaim  int              `gorm:"column:per_user_limit_at_claim;not null;default:1" json:"per_user_limit_at_claim"`
	RedeemedAt           time.Time        `gorm:"column:redeemed_at;index;autoCreateTime" json:"redeemed_at"`
}

func (RedemptionCodeRedemption) TableName() string { return "redemption_code_redemptions" }

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
	PromptPer1MCostCredits     int64                  `gorm:"-" json:"prompt_per_1m_cost_credits"`
	OutputPer1MCostCredits     int64                  `gorm:"-" json:"output_per_1m_cost_credits"`
	ReasoningPer1MCostCredits  int64                  `gorm:"-" json:"reasoning_per_1m_cost_credits"`
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
	ID            uint64    `gorm:"primaryKey" json:"id"`
	MarkupBPS     int       `gorm:"column:markup_bps;not null;default:3000" json:"markup_bps"`
	Currency      string    `gorm:"column:currency;size:8;not null;default:usd" json:"currency"`
	CreditsPerUSD int       `gorm:"column:credits_per_usd;not null;default:100" json:"credits_per_usd"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at,omitempty"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at,omitempty"`
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
	TotalAPIKeys          int64                     `json:"total_api_keys"`
	ActiveAPIKeys         int64                     `json:"active_api_keys"`
	DisabledAPIKeys       int64                     `json:"disabled_api_keys"`
	ExpiredAPIKeys        int64                     `json:"expired_api_keys"`
	FreeMachines          int64                     `json:"free_machines"`
	ChecksLast24h         int64                     `json:"checks_last_24h"`
	ConsumesLast24h       int64                     `json:"consumes_last_24h"`
	BlockedLast24h        int64                     `json:"blocked_last_24h"`
	TotalUsers            int64                     `json:"total_users"`
	PaidOrdersLast24h     int64                     `json:"paid_orders_last_24h"`
	PaidQuotaAddedLast24h int64                     `json:"paid_quota_added_last_24h"`
	RemainingPaidQuota    int64                     `json:"remaining_paid_quota"`
	UsageTrend            []OverviewUsageTrendPoint `json:"usage_trend"`
	ModeBreakdown         []OverviewBreakdownItem   `json:"mode_breakdown"`
	ResultBreakdown       []OverviewBreakdownItem   `json:"result_breakdown"`
	APIKeyStatusBreakdown []OverviewBreakdownItem   `json:"api_key_status_breakdown"`
	OperationsFunnel30d   *OperationsFunnel         `json:"operations_funnel_30d,omitempty"`
}

type OverviewUsageTrendPoint struct {
	Date     string `json:"date"`
	Checks   int64  `json:"checks"`
	Consumes int64  `json:"consumes"`
	Blocked  int64  `json:"blocked"`
	Allowed  int64  `json:"allowed"`
	Total    int64  `json:"total"`
}

type OverviewBreakdownItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value int64  `json:"value"`
}

type FingerprintQuality struct {
	Summary []FingerprintQualityBucketSummary `json:"summary"`
	Rows    []FingerprintQualityRow           `json:"rows"`
}

type FingerprintQualityBucketSummary struct {
	Bucket          string `json:"bucket"`
	Reason          string `json:"reason"`
	Fingerprints    int64  `json:"fingerprints"`
	Events          int64  `json:"events"`
	DefaultFiltered bool   `json:"default_filtered"`
}

type FingerprintQualityRow struct {
	Bucket            string    `json:"bucket"`
	Reason            string    `json:"reason"`
	FingerprintHash   string    `json:"fingerprint_hash"`
	FingerprintPrefix string    `json:"fingerprint_prefix"`
	FirstAt           time.Time `json:"first_at"`
	LastAt            time.Time `json:"last_at"`
	Events            int64     `json:"events"`
	GenerateEvents    int64     `json:"generate_events"`
	StatusEvents      int64     `json:"status_events"`
	BlockedEvents     int64     `json:"blocked_events"`
	UserBoundEvents   int64     `json:"user_bound_events"`
	IPCount           int64     `json:"ip_count"`
	IPs               []string  `json:"ips"`
	CLIVersions       []string  `json:"cli_versions"`
	RuntimeModes      []string  `json:"runtime_modes"`
	DocumentTypes     []string  `json:"document_types"`
	UserAgents        []string  `json:"user_agents"`
}

type OperationsFunnel struct {
	WindowStart              time.Time                `json:"window_start"`
	WindowEnd                time.Time                `json:"window_end"`
	Visitors                 int64                    `json:"visitors"`
	PricingViews             int64                    `json:"pricing_views"`
	CTAClicks                int64                    `json:"cta_clicks"`
	LoginStarts              int64                    `json:"login_starts"`
	RegisteredUsers          int64                    `json:"registered_users"`
	ActivatedUsers           int64                    `json:"activated_users"`
	ActivatedRegisteredUsers int64                    `json:"activated_registered_users"`
	ActivationRate           float64                  `json:"activation_rate"`
	CLILoginStarted          int64                    `json:"cli_login_started"`
	CLILoginCompleted        int64                    `json:"cli_login_completed"`
	CLISessionsCreated       int64                    `json:"cli_sessions_created"`
	FirstUsageUsers          int64                    `json:"first_usage_users"`
	RepeatUsageUsers         int64                    `json:"repeat_usage_users"`
	BlockedRate              float64                  `json:"blocked_rate"`
	CheckoutStarts           int64                    `json:"checkout_starts"`
	OrdersCreated            int64                    `json:"orders_created"`
	PaidOrders               int64                    `json:"paid_orders"`
	PaidUsers                int64                    `json:"paid_users"`
	PaidRegisteredUsers      int64                    `json:"paid_registered_users"`
	Revenue                  int64                    `json:"revenue"`
	PaidConversionRate       float64                  `json:"paid_conversion_rate"`
	CheckoutToPaidRate       float64                  `json:"checkout_to_paid_rate"`
	RepeatPaidUsers          int64                    `json:"repeat_paid_users"`
	MachineQuality           OperationsMachineQuality `json:"machine_quality"`
	UsageQuality             OperationsUsageQuality   `json:"usage_quality"`
	RevenueQuality           OperationsRevenueQuality `json:"revenue_quality"`
}

type OperationsMachineQuality struct {
	TotalMachines     int64 `json:"total_machines"`
	ActiveMachines24h int64 `json:"active_machines_24h"`
	ActiveMachines7d  int64 `json:"active_machines_7d"`
}

type OperationsUsageQuality struct {
	FirstUsageUsers  int64   `json:"first_usage_users"`
	RepeatUsageUsers int64   `json:"repeat_usage_users"`
	BlockedRate      float64 `json:"blocked_rate"`
}

type OperationsRevenueQuality struct {
	PaidOrders          int64   `json:"paid_orders"`
	PaidUsers           int64   `json:"paid_users"`
	PaidRegisteredUsers int64   `json:"paid_registered_users"`
	Revenue             int64   `json:"revenue"`
	RepeatPaidUsers     int64   `json:"repeat_paid_users"`
	PaidConversionRate  float64 `json:"paid_conversion_rate"`
	CheckoutToPaidRate  float64 `json:"checkout_to_paid_rate"`
}

func Rate(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func StringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func StringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type IssueReport struct {
	ID                uint64     `gorm:"primaryKey" json:"id"`
	TicketID          string     `gorm:"column:ticket_id;size:40;uniqueIndex" json:"ticket_id"`
	UserID            *uint64    `gorm:"column:user_id;index" json:"user_id,omitempty"`
	ClientEventID     string     `gorm:"column:client_event_id;size:128;uniqueIndex" json:"client_event_id"`
	SchemaVersion     int        `gorm:"column:schema_version;not null" json:"schema_version"`
	RuntimeMode       string     `gorm:"column:runtime_mode;size:16;not null" json:"runtime_mode"`
	Category          string     `gorm:"column:category;size:64;not null;default:''" json:"category"`
	Severity          string     `gorm:"column:severity;size:16;not null;default:''" json:"severity"`
	Summary           string     `gorm:"column:summary;size:500;not null" json:"summary"`
	Detail            string     `gorm:"column:detail;size:2000;not null;default:''" json:"detail"`
	ContactEmail      *string    `gorm:"column:contact_email;size:254" json:"contact_email,omitempty"`
	TaskID            string     `gorm:"column:task_id;size:128;not null;default:''" json:"task_id,omitempty"`
	ErrorCode         string     `gorm:"column:error_code;size:64;not null;default:''" json:"error_code,omitempty"`
	ErrorMessage      string     `gorm:"column:error_message;size:500;not null;default:''" json:"error_message,omitempty"`
	Via               string     `gorm:"column:via;size:16;not null;default:'http'" json:"via"`
	ClientMeta        []byte     `gorm:"column:client_meta;type:jsonb;not null;default:'{}'" json:"client_meta,omitempty"`
	ClientTsMs        int64      `gorm:"column:client_ts_ms;not null" json:"client_ts_ms"`
	ServerNowMs       int64      `gorm:"column:server_now_ms;not null" json:"server_now_ms"`
	ClockSkewFallback bool       `gorm:"column:clock_skew_fallback;not null;default:false" json:"clock_skew_fallback"`
	ClientIP          string     `gorm:"column:client_ip;size:64;not null;default:''" json:"client_ip,omitempty"`
	UserAgent         string     `gorm:"column:user_agent;size:512;not null;default:''" json:"user_agent,omitempty"`
	Status            string     `gorm:"column:status;size:16;not null;default:'new'" json:"status"`
	TriagedAt         *time.Time `gorm:"column:triaged_at" json:"triaged_at,omitempty"`
	TriagedBy         string     `gorm:"column:triaged_by;size:191;not null;default:''" json:"triaged_by,omitempty"`
	CreatedAt         time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (IssueReport) TableName() string { return "issue_reports" }

type IssueReportRequestID struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	ReportID  uint64    `gorm:"column:report_id;not null;index;uniqueIndex:uq_issue_report_request_ids_report_request,priority:1" json:"report_id"`
	RequestID string    `gorm:"column:request_id;size:128;not null;index;uniqueIndex:uq_issue_report_request_ids_report_request,priority:2" json:"request_id"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (IssueReportRequestID) TableName() string { return "issue_report_request_ids" }
