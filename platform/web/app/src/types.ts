export interface User { id: number; email: string; name: string; avatar_url?: string; status: string }
export interface PricingPack { code: string; name: string; description: string; currency: string; amount_total: number; quota_amount: number; credit_amount?: number; pack_kind?: string }
export interface AppOverview {
  api_key_count: number
  total_remaining: number
  reward_remaining: number
  invite_code?: string
  invite_limit: number
  invite_remaining: number
  reward_per_invite: number
  referral_count: number
  activated_referral_count: number
  discord_connected: boolean
  discord_guild_member: boolean
  recent_usage_count: number
  recent_orders_count: number
  pricing: PricingPack[]
}
export interface AppGrowth {
  invite_code?: string
  invite_limit: number
  invite_remaining: number
  reward_per_invite: number
  reward_remaining: number
  reward_grants: RewardGrant[]
  referrals: Referral[]
  discord_connection?: DiscordConnection
}
export interface DiscordStatus {
  connection?: DiscordConnection
  reward_remaining: number
}
export interface ConnectDiscordResponse {
  connection: DiscordConnection
  reward_granted: boolean
  reward_remaining: number
  reward_source_type?: string
  reward_idempotency_key?: string
}
export interface ApiKey { id: number; key_prefix: string; status: 'active' | 'disabled'; plan_name: string; note?: string; expires_at?: string; quota_total?: number; quota_used: number; quota_remaining: number; created_at: string; last_used_at?: string }
export interface Order { id: number; status: string; currency: string; amount_total: number; pack_code: string; pack_name: string; pack_kind?: string; quota_amount: number; credit_amount?: number; target_api_key_id?: number; created_at: string }
export interface UsageEvent { id: number; mode: string; action: string; result: string; created_at: string; fingerprint_hash: string; reason_code?: string; runtime_mode?: string; provider?: string; model_name?: string; prompt_tokens?: number; completion_tokens?: number; image_count?: number; settled_credits?: number }
export interface RewardGrant { source_type: string; amount_total: number; amount_used: number; remaining: number; reason: string; metadata_json: string; created_at: string; updated_at: string }
export interface Referral { invite_code: string; registered_at: string; activated_at?: string; reward_granted_at?: string }
export interface DiscordConnection { username: string; guild_member: boolean; connected_at: string; reward_granted_at?: string; verification_status: string; verification_blocked_reason?: string }
export interface Envelope<T> { data?: T; error?: string; request_id?: string }
