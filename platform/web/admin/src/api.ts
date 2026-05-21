import type { AdminGrowth, AdminIdentity, AdminPreference, ApiKey, BillingEvent, DailyFreeQuota, Envelope, FingerprintQuality, HostedBillingConfig, HostedCreditPack, HostedModelPricingConfig, HostedPricingRule, HostedPricingSetting, OperationsFunnel, Order, Overview, QuotaSources, UsageEvent, User } from './types'

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
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
    const payload = await response.json().catch(() => ({ error: response.statusText }))
    throw new ApiError(payload.error || response.statusText, response.status)
  }

  const payload = (await response.json()) as Envelope<T>
  return payload.data
}

export const api = {
  session: () => request<AdminIdentity>('/api/admin/session'),
  login: (returnTo = '/admin') => {
    window.location.href = `/api/admin/auth/oauth2/login?return_to=${encodeURIComponent(returnTo)}`
  },
  logout: () => request('/api/admin/logout', { method: 'POST' }),
  overview: () => request<Overview>('/api/admin/overview'),
  fingerprintQuality: () => request<FingerprintQuality>('/api/admin/fingerprint-quality'),
  operationsFunnel: (range: '24h' | '7d' | '30d') => request<OperationsFunnel>(`/api/admin/operations/funnel?range=${encodeURIComponent(range)}`),
  growth: () => request<AdminGrowth>('/api/admin/growth'),
  apiKeys: (userID?: number) => request<ApiKey[]>(userID ? `/api/admin/api-keys?user_id=${encodeURIComponent(String(userID))}` : '/api/admin/api-keys'),
  getApiKeyPlaintext: (id: number) => request<{ plaintext_key: string }>(`/api/admin/api-keys/${id}/plaintext`),
  createApiKey: (payload: Record<string, unknown>) => request<{ plaintext_key: string; key_prefix: string }>('/api/admin/api-keys', { method: 'POST', body: JSON.stringify(payload) }),
  updateApiKey: (id: number, payload: Record<string, unknown>) => request(`/api/admin/api-keys/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  freeQuotas: (fingerprint = '', usageDate = '') => request<DailyFreeQuota[]>(`/api/admin/free-quotas?fingerprint=${encodeURIComponent(fingerprint)}&usage_date=${encodeURIComponent(usageDate)}`),
  updateFreeQuota: (id: number, free_limit: number) => request(`/api/admin/free-quotas/${id}`, { method: 'PATCH', body: JSON.stringify({ free_limit }) }),
  quotaSources: (params: URLSearchParams) => request<QuotaSources>(`/api/admin/quota-sources?${params.toString()}`),
  usageEvents: (params: URLSearchParams) => request<UsageEvent[]>(`/api/admin/usage-events?${params.toString()}`),
  adminPreference: (pageKey: string) => request<AdminPreference>(`/api/admin/preferences/${encodeURIComponent(pageKey)}`),
  saveAdminPreference: (pageKey: string, payload: AdminPreference) => request<AdminPreference>(`/api/admin/preferences/${encodeURIComponent(pageKey)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  users: (query = '') => request<User[]>(query ? `/api/admin/users?query=${encodeURIComponent(query)}` : '/api/admin/users'),
  updateUser: (id: number, payload: Record<string, unknown>) => request(`/api/admin/users/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  orders: () => request<Order[]>('/api/admin/orders'),
  updateOrder: (id: number, payload: Record<string, unknown>) => request(`/api/admin/orders/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  billingEvents: () => request<BillingEvent[]>('/api/admin/billing-events'),
  hostedPricingRules: () => request<HostedPricingRule[]>('/api/admin/hosted-pricing-rules'),
  hostedBilling: () => request<HostedBillingConfig>('/api/admin/hosted-billing'),
  updateHostedPricingSettings: (payload: Partial<HostedPricingSetting>) => request<HostedPricingSetting>('/api/admin/hosted-pricing-settings', { method: 'PATCH', body: JSON.stringify(payload) }),
  createHostedModelPricingConfig: (payload: HostedModelPricingConfig) => request<HostedModelPricingConfig>('/api/admin/hosted-model-pricing-configs', { method: 'POST', body: JSON.stringify(payload) }),
  updateHostedModelPricingConfig: (id: number, payload: HostedModelPricingConfig) => request<HostedModelPricingConfig>(`/api/admin/hosted-model-pricing-configs/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  createHostedPricingRule: (payload: HostedPricingRule) => request<HostedPricingRule>('/api/admin/hosted-pricing-rules', { method: 'POST', body: JSON.stringify(payload) }),
  updateHostedPricingRule: (id: number, payload: HostedPricingRule) => request<HostedPricingRule>(`/api/admin/hosted-pricing-rules/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  createHostedCreditPack: (payload: HostedCreditPack) => request<HostedCreditPack>('/api/admin/hosted-credit-packs', { method: 'POST', body: JSON.stringify(payload) }),
  updateHostedCreditPack: (id: number, payload: HostedCreditPack) => request<HostedCreditPack>(`/api/admin/hosted-credit-packs/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
}
