export interface AdminIdentity {
  email: string
  name?: string
  auth_method?: string
}

export interface ApiKey {
  id: number
  key_prefix: string
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

export interface FreeQuota {
  id: number
  fingerprint_hash: string
  free_limit: number
  free_used: number
  created_at: string
  updated_at: string
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
  prompt_tokens?: number
  completion_tokens?: number
  image_count?: number
  settled_credits?: number
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
  referrals: Referral[]
  discord_connections: DiscordConnection[]
}

export interface HostedPricingRule {
  document_profile: string
  provider: string
  model: string
  prompt_per_1k_credits: number
  output_per_1k_credits: number
  reasoning_per_1k_credits: number
  image_per_asset_credits: number
  reservation_credits: number
  minimum_charge_credits: number
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
