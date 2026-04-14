import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as navigation from '../lib/navigation'
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

  it('renders external-only pricing and order history', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              code: 'external-100',
              name: 'External 100',
              description: '100 external generations for lightweight evaluation and individual workflows.',
              currency: 'usd',
              amount_total: 990,
              quota_amount: 100,
              pack_kind: 'external_generation',
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
              amount_total: 990,
              pack_code: 'external-100',
              pack_name: 'External 100',
              pack_kind: 'external_generation',
              quota_amount: 100,
              created_at: '2026-04-03T00:00:00Z',
            }],
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText('External 100')).toBeInTheDocument()
    expect(screen.getByText(/100 external generations for lightweight evaluation and individual workflows\./i)).toBeInTheDocument()
    expect(screen.getByText(/100 external generations per purchase/i)).toBeInTheDocument()
    expect(screen.queryByText(/hosted/i)).not.toBeInTheDocument()
  })

  it('posts checkout to the existing app endpoint and keeps Stripe copy explicit', async () => {
    const redirectSpy = vi.spyOn(navigation, 'redirectTo').mockImplementation(() => {})
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
              description: '100 external generations for lightweight evaluation and individual workflows.',
              currency: 'usd',
              amount_total: 990,
              quota_amount: 100,
              pack_kind: 'external_generation',
            }],
          }),
        }
      }
      if (url === '/api/app/api-keys') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 7,
              key_prefix: 'cop_live_demo',
              status: 'active',
              plan_name: 'Production',
              quota_total: 100,
              quota_used: 0,
              quota_remaining: 100,
              created_at: '2026-04-03T00:00:00Z',
            }],
          }),
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
    expect(redirectSpy).toHaveBeenCalledWith('https://checkout.stripe.com/pay/test_session')
  })

  it('shows checkout error details and request id when checkout fails', async () => {
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
              description: '100 external generations for lightweight evaluation and individual workflows.',
              currency: 'usd',
              amount_total: 990,
              quota_amount: 100,
              pack_kind: 'external_generation',
            }],
          }),
        }
      }
      if (url === '/api/app/api-keys') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 7,
              key_prefix: 'cop_test',
              status: 'active',
              plan_name: 'Growth',
              quota_used: 0,
              quota_remaining: 100,
              created_at: '2026-04-03T00:00:00Z',
            }],
          }),
        }
      }
      if (url === '/api/app/orders') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/checkout' && init?.method === 'POST') {
        return {
          ok: false,
          status: 400,
          json: async () => ({
            error: 'target api key is disabled',
            request_id: 'req_checkout_123',
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    await screen.findByText(/cop_test \/ Growth/i)
    const checkoutButton = await screen.findByRole('button', { name: /continue to stripe checkout/i })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: '7' } })
    fireEvent.click(checkoutButton)

    expect(await screen.findByText(/Checkout failed: target api key is disabled \(request_id: req_checkout_123\)/i)).toBeInTheDocument()
  })
})
