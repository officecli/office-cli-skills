import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import BillingPage from './BillingPage'

const fetchMock = vi.fn()

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <BillingPage />
    </QueryClientProvider>,
  )
}

describe('billing page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('renders English pack copy even if pricing and order payloads contain Chinese text', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              code: 'hosted-300',
              name: '托管 300',
              description: '300 credits，适合托管 LLM 试用和低频文档生成。',
              currency: 'usd',
              amount_total: 2900,
              quota_amount: 0,
              credit_amount: 300,
              pack_kind: 'hosted_credits',
            }],
          }),
        }
      }
      if (url === '/api/app/api-keys') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/orders') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 11,
              status: 'paid',
              currency: 'usd',
              amount_total: 2900,
              pack_code: 'hosted-300',
              pack_name: '托管 300',
              pack_kind: 'hosted_credits',
              quota_amount: 0,
              credit_amount: 300,
              created_at: '2026-04-03T00:00:00Z',
            }],
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText('Hosted 300')).toBeInTheDocument()
    expect(screen.getByText(/300 hosted credits for low-volume runs on the platform-managed LLM runtime\./i)).toBeInTheDocument()
    expect(screen.queryByText(/托管 300/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/适合托管 LLM/i)).not.toBeInTheDocument()
  })

  it('posts checkout to the existing app endpoint and keeps Stripe copy explicit', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              code: 'external-100',
              name: 'External 100',
              description: '100 external generations for workflows that already bring their own LLM.',
              currency: 'usd',
              amount_total: 1900,
              quota_amount: 100,
              pack_kind: 'external_quota',
            }],
          }),
        }
      }
      if (url === '/api/app/api-keys') {
        return {
          ok: true,
          status: 200,
          json: async () => ({ data: [{ id: 7, key_prefix: 'cop_live_demo', plan_name: 'Production', quota_total: 100, quota_used: 0, quota_remaining: 100, credit_balance: 0 }] }),
        }
      }
      if (url === '/api/app/orders') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/checkout') {
        expect(init?.method).toBe('POST')
        expect(init?.body).toBe(JSON.stringify({ pack_code: 'external-100', target_api_key_id: 7 }))
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              order: { id: 21, status: 'pending' },
              checkout_url: 'https://checkout.stripe.com/pay/test_session',
            },
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText('External 100')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('combobox'), { target: { value: '7' } })
    fireEvent.click(screen.getByRole('button', { name: /Continue to Stripe Checkout/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/app/checkout', expect.objectContaining({
        method: 'POST',
      }))
    })
  })
})
