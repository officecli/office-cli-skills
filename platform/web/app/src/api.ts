import type { ApiKey, AppGrowth, AppOverview, AppQuotaSummary, ConnectDiscordResponse, DiscordStatus, Envelope, Order, PricingPack, UsageEvent, User } from './types'
import { buildGoogleLoginURL } from './analytics'

export class ApiError extends Error {
  status: number
  requestId?: string

  constructor(message: string, status: number, requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.requestId = requestId
  }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  })

  if (response.status === 401) {
    throw new Error('UNAUTHORIZED')
  }
  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: response.statusText })) as Partial<Envelope<unknown>>
    throw new ApiError(payload.error || response.statusText, response.status, payload.request_id)
  }

  const payload = (await response.json()) as Envelope<T>
  if (payload.data === undefined) {
    throw new Error('empty response payload')
  }
  return payload.data
}

export const api = {
  me: () => request<User>('/api/auth/me'),
  login: (returnTo = '/app') => { window.location.href = buildGoogleLoginURL(returnTo) },
  logout: () => request('/api/auth/logout', { method: 'POST' }),
  overview: () => request<AppOverview>('/api/app/overview'),
  quotaSummary: () => request<AppQuotaSummary>('/api/app/quota-summary'),
  growth: () => request<AppGrowth>('/api/app/growth'),
  discordStatus: () => request<DiscordStatus>('/api/app/discord/status'),
  connectDiscord: (payload: { discord_user_id: string; username: string }) => request<ConnectDiscordResponse>('/api/app/discord/connect', { method: 'POST', body: JSON.stringify(payload) }),
  apiKeys: () => request<ApiKey[]>('/api/app/api-keys'),
  createApiKey: (payload: Record<string, unknown>) => request<{ plaintext_key: string; key: ApiKey }>('/api/app/api-keys', { method: 'POST', body: JSON.stringify(payload) }),
  updateApiKey: (id: number, payload: Record<string, unknown>) => request(`/api/app/api-keys/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  orders: () => request<Order[]>('/api/app/orders'),
  usage: () => request<UsageEvent[]>('/api/app/usage-events'),
  pricing: () => request<PricingPack[]>('/api/pricing'),
  checkout: (payload: { pack_code: string; target_api_key_id: number }) => request<{ order: Order; checkout_url: string }>('/api/app/checkout', { method: 'POST', body: JSON.stringify(payload) }),
}
