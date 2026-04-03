import type { AdminGrowth, AdminIdentity, ApiKey, BillingEvent, Envelope, FreeQuota, HostedPricingRule, Order, Overview, UsageEvent, User } from './types'

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
    throw new Error(payload.error || response.statusText)
  }

  const payload = (await response.json()) as Envelope<T>
  return payload.data
}

export const api = {
  session: () => request<AdminIdentity>('/api/admin/session'),
  login: (returnTo = '/admin') => {
    window.location.href = `/api/admin/auth/google/login?return_to=${encodeURIComponent(returnTo)}`
  },
  logout: () => request('/api/admin/logout', { method: 'POST' }),
  overview: () => request<Overview>('/api/admin/overview'),
  growth: () => request<AdminGrowth>('/api/admin/growth'),
  apiKeys: () => request<ApiKey[]>('/api/admin/api-keys'),
  createApiKey: (payload: Record<string, unknown>) => request<{ plaintext_key: string; key_prefix: string }>('/api/admin/api-keys', { method: 'POST', body: JSON.stringify(payload) }),
  updateApiKey: (id: number, payload: Record<string, unknown>) => request(`/api/admin/api-keys/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  freeQuotas: (fingerprint = '') => request<FreeQuota[]>(`/api/admin/free-quotas?fingerprint=${encodeURIComponent(fingerprint)}`),
  updateFreeQuota: (id: number, free_limit: number) => request(`/api/admin/free-quotas/${id}`, { method: 'PATCH', body: JSON.stringify({ free_limit }) }),
  usageEvents: (params: URLSearchParams) => request<UsageEvent[]>(`/api/admin/usage-events?${params.toString()}`),
  users: () => request<User[]>('/api/admin/users'),
  updateUser: (id: number, payload: Record<string, unknown>) => request(`/api/admin/users/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  orders: () => request<Order[]>('/api/admin/orders'),
  updateOrder: (id: number, payload: Record<string, unknown>) => request(`/api/admin/orders/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  billingEvents: () => request<BillingEvent[]>('/api/admin/billing-events'),
  hostedPricingRules: () => request<HostedPricingRule[]>('/api/admin/hosted-pricing-rules'),
}
