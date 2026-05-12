export interface AdminIdentity {
  email: string
  name?: string
  auth_method?: string
}

export interface ApiKey {
  id: number
  key_prefix: string
  plaintext_available?: boolean
  status: 'active' | 'disabled'
  plan_name: string
  owner_user_id?: number
  plan_code?: string
  allowed_modes?: string
  hosted_enabled?: boolean
  default_runtime_mode?: string
  expires_at?: string
  created_at: string
  note?: string
  last_used_at?: string
  quota_total?: number
  quota_used: number
  quota_remaining?: number
  credit_balance?: number
  credit_reserved?: number
}

export interface DailyFreeQuota {
  id: number
  fingerprint_hash: string
  usage_date: string
  daily_limit: number
  daily_used: number
  remaining: number
  created_at: string
  updated_at: string
}

export interface QuotaSources {
  free_trial_devices: DailyFreeQuota[]
  reward_grants: RewardGrant[]
  paid_external_keys: ApiKey[]
  hosted_keys: ApiKey[]
}

export interface UsageEvent {
  id: number
  fingerprint_hash: string
  mode: 'free' | 'reward' | 'paid' | 'hosted'
  action: 'generate' | 'status'
  result: 'allowed' | 'blocked'
  reason_code?: string
  created_at: string
  api_key_id?: number
  user_id?: number
  runtime_mode?: string
  provider?: string
  model_name?: string
  billed_units?: number
  unit_type?: string
  prompt_tokens?: number
  completion_tokens?: number
  reasoning_tokens?: number
  image_count?: number
  markup_bps?: number
  upstream_cost_microusd?: number
  uncapped_charge_credits?: number
  settled_credits?: number
  profit_microusd?: number
  cap_applied?: boolean
}

export interface User {
  id: number
  email: string
  name: string
  invite_code?: string
  status: 'active' | 'disabled'
  created_at: string
}

export interface Order {
  id: number
  user_id: number
  status: string
  currency: string
  amount_total: number
  pack_code: string
  pack_name: string
  quota_amount: number
  credit_amount?: number
  pack_kind?: string
  target_api_key_id?: number
  created_at: string
  note?: string
}

export interface BillingEvent {
  id: number
  order_id?: number
  event_id: string
  event_type: string
  status: string
  error_message?: string
  processed_at?: string
  created_at: string
}

export interface AdminGrowth {
  reward_grants: RewardGrant[]
  hosted_credit_grants?: HostedCreditGrant[]
  referrals: Referral[]
  discord_connections: DiscordConnection[]
}

export interface HostedCreditGrant {
  id: number
  user_id: number
  api_key_id: number
  source_type: string
  idempotency_key: string
  credit_amount: number
  reason: string
  metadata_json: string
  created_at: string
}

export interface HostedPricingRule {
  id?: number
  document_profile: string
  provider: string
  model: string
  text_model_key?: string
  image_model_key?: string
  prompt_per_1k_cost_microusd: number
  output_per_1k_cost_microusd: number
  reasoning_per_1k_cost_microusd: number
  image_per_asset_cost_microusd: number
  reservation_credits: number
  minimum_charge_credits: number
  markup_bps?: number
  enabled: boolean
}

export interface HostedModelPricingConfig {
  id?: number
  key: string
  kind: 'text' | 'image'
  provider: string
  model: string
  prompt_per_1m_cost_microusd: number
  output_per_1m_cost_microusd: number
  reasoning_per_1m_cost_microusd: number
  prompt_per_1m_cost_credits: number
  output_per_1m_cost_credits: number
  reasoning_per_1m_cost_credits: number
  enabled: boolean
}

export interface HostedPricingSetting {
  id: number
  markup_bps: number
  currency: string
  credits_per_usd: number
}

export interface HostedCreditPack {
  id?: number
  code: string
  name: string
  description: string
  currency: string
  amount_total: number
  credit_amount: number
  enabled: boolean
}

export interface HostedBillingConfig {
  settings: HostedPricingSetting
  model_configs: HostedModelPricingConfig[]
  rules: HostedPricingRule[]
  packs: HostedCreditPack[]
}

export interface RewardGrant {
  id: number
  user_id: number
  source_type: string
  idempotency_key: string
  amount_total: number
  amount_used: number
  reason: string
  metadata_json: string
  created_at: string
  updated_at: string
}

export interface Referral {
  id: number
  inviter_user_id: number
  invited_user_id: number
  invite_code: string
  registered_at: string
  activated_at?: string
  reward_granted_at?: string
  created_at: string
  updated_at: string
}

export interface DiscordConnection {
  id: number
  user_id: number
  discord_user_id: string
  username: string
  guild_member: boolean
  connected_at: string
  reward_granted_at?: string
  created_at: string
  updated_at: string
}

export interface Overview {
  total_api_keys: number
  active_api_keys: number
  disabled_api_keys: number
  expired_api_keys: number
  free_machines: number
  checks_last_24h: number
  consumes_last_24h: number
  blocked_last_24h: number
  total_users: number
  paid_orders_last_24h: number
  paid_quota_added_last_24h: number
  remaining_paid_quota: number
}

export interface Envelope<T> {
  data: T
}
