import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as navigation from '../lib/navigation'
import BillingPage from './BillingPage'

const fetchMock = vi.fn()

function renderPage(path = '/billing') {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[path]}>
        <BillingPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('billing page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('renders hosted pricing while preserving historical external order history', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              code: 'hosted-300',
              name: 'Hosted 300',
              description: '300 hosted credits for OfficeCLI-managed generation.',
              currency: 'usd',
              amount_total: 300,
              credit_amount: 300,
              pack_kind: 'hosted_credits',
            }],
          }),
        }
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
              amount_total: 500,
              pack_code: 'external-100',
              pack_name: 'External 100',
              pack_kind: 'external_generation',
              quota_amount: 100,
              target_api_key_id: 7,
              stripe_payment_intent_id: 'pi_test_123',
              created_at: '2026-04-03T00:00:00Z',
            }],
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect((await screen.findAllByText('Hosted 300')).length).toBeGreaterThan(0)
    expect(screen.getByText(/300 hosted credits for OfficeCLI-managed generation\./i)).toBeInTheDocument()
    expect(screen.getByText(/300 credits/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Continue to Stripe Checkout/i })).toBeInTheDocument()
    expect(screen.queryByText(/external-100/i)).not.toBeInTheDocument()
    expect(screen.getByText('pi_test_123')).toBeInTheDocument()
    expect(screen.getByText(/Legacy order target API key #7/i)).toBeInTheDocument()
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
              code: 'hosted-300',
              name: 'Hosted 300',
              description: '300 hosted credits for platform-managed generation.',
              currency: 'usd',
              amount_total: 300,
              credit_amount: 300,
              pack_kind: 'hosted_credits',
            }],
          }),
        }
      }
      if (url === '/api/app/orders') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/checkout') {
        expect(init?.method).toBe('POST')
        expect(init?.body).toBe(JSON.stringify({ pack_code: 'hosted-300' }))
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

    expect(await screen.findByText('Hosted 300')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Continue to Stripe Checkout/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/app/checkout', expect.objectContaining({
        method: 'POST',
      }))
    })
    expect(redirectSpy).toHaveBeenCalledWith('https://checkout.stripe.com/pay/test_session')
  })

  it('renders hosted credit packs and posts hosted checkout payloads', async () => {
    const redirectSpy = vi.spyOn(navigation, 'redirectTo').mockImplementation(() => {})
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return { ok: true, status: 200, json: async () => ({ data: [{ code: 'hosted-300', name: 'Hosted 300', description: '300 hosted credits for platform-managed generation.', currency: 'usd', amount_total: 300, credit_amount: 300, pack_kind: 'hosted_credits' }] }) }
      }
      if (url === '/api/app/orders') {
        return { ok: true, status: 200, json: async () => ({ data: [{ id: 31, status: 'paid', currency: 'usd', amount_total: 300, pack_code: 'hosted-300', pack_name: 'Hosted 300', pack_kind: 'hosted_credits', quota_amount: 0, credit_amount: 300, target_api_key_id: 7, created_at: '2026-04-03T00:00:00Z' }] }) }
      }
      if (url === '/api/app/checkout') {
        expect(init?.body).toBe(JSON.stringify({ pack_code: 'hosted-300' }))
        return { ok: true, status: 200, json: async () => ({ data: { order: { id: 32, status: 'pending' }, checkout_url: 'https://checkout.stripe.com/pay/hosted' } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect((await screen.findAllByText('Hosted 300')).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/300 hosted credits/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/300 credits/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/≈ \$3\.00 USD/i).length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('button', { name: /Continue to Stripe Checkout/i }))

    await waitFor(() => expect(redirectSpy).toHaveBeenCalledWith('https://checkout.stripe.com/pay/hosted'))
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
              code: 'hosted-300',
              name: 'Hosted 300',
              description: '300 hosted credits for platform-managed generation.',
              currency: 'usd',
              amount_total: 300,
              credit_amount: 300,
              pack_kind: 'hosted_credits',
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

    await screen.findByText('Hosted 300')
    const checkoutButton = await screen.findByRole('button', { name: /continue to stripe checkout/i })
    fireEvent.click(checkoutButton)

    expect(await screen.findByText(/Checkout failed: target api key is disabled \(request_id: req_checkout_123\)/i)).toBeInTheDocument()
  })

  it('reconciles a successful checkout return and refreshes recent billing activity', async () => {
    let ordersCallCount = 0
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/orders') {
        ordersCallCount++
        const status = ordersCallCount > 1 ? 'paid' : 'pending'
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 11,
              status,
              currency: 'usd',
              amount_total: 500,
              pack_code: 'external-100',
              pack_name: 'External 100',
              pack_kind: 'external_generation',
              quota_amount: 100,
              target_api_key_id: 7,
              stripe_checkout_session_id: 'cs_test_123',
              stripe_payment_intent_id: status === 'paid' ? 'pi_test_123' : undefined,
              created_at: '2026-04-03T00:00:00Z',
            }],
          }),
        }
      }
      if (url === '/api/app/orders/reconcile') {
        expect(init?.method).toBe('POST')
        expect(init?.body).toBe(JSON.stringify({ checkout_session_id: 'cs_test_123' }))
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              order: {
                id: 11,
                status: 'paid',
                currency: 'usd',
                amount_total: 500,
                pack_code: 'external-100',
                pack_name: 'External 100',
                pack_kind: 'external_generation',
                quota_amount: 100,
                target_api_key_id: 7,
                stripe_checkout_session_id: 'cs_test_123',
                stripe_payment_intent_id: 'pi_test_123',
                created_at: '2026-04-03T00:00:00Z',
              },
              awaiting_confirmation: false,
            },
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage('/billing?status=success&session_id=cs_test_123')

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/app/orders/reconcile', expect.objectContaining({
        method: 'POST',
      }))
    })
    expect(await screen.findByText('pi_test_123')).toBeInTheDocument()
  })

  it('shows the payment-still-confirming alert when reconcile returns awaiting_confirmation', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/orders') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 22,
              status: 'pending',
              currency: 'usd',
              amount_total: 100,
              pack_code: 'hosted-100',
              pack_name: 'Hosted 100',
              pack_kind: 'hosted_credits',
              quota_amount: 0,
              credit_amount: 100,
              stripe_checkout_session_id: 'cs_live_pending_xyz',
              created_at: '2026-05-24T00:00:00Z',
            }],
          }),
        }
      }
      if (url === '/api/app/orders/reconcile') {
        expect(init?.method).toBe('POST')
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              order: {
                id: 22,
                status: 'pending',
                currency: 'usd',
                amount_total: 100,
                pack_code: 'hosted-100',
                pack_name: 'Hosted 100',
                pack_kind: 'hosted_credits',
                quota_amount: 0,
                credit_amount: 100,
                stripe_checkout_session_id: 'cs_live_pending_xyz',
                created_at: '2026-05-24T00:00:00Z',
              },
              awaiting_confirmation: true,
            },
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage('/billing?status=success&session_id=cs_live_pending_xyz')

    expect(await screen.findByText('Payment still confirming')).toBeInTheDocument()
  })

  it('auto-starts checkout when ?pack=hosted-300&autostart=1 and pack matches', async () => {
    const redirectSpy = vi.spyOn(navigation, 'redirectTo').mockImplementation(() => {})
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return { ok: true, status: 200, json: async () => ({ data: [{ code: 'hosted-300', name: 'Hosted 300', description: 'x', currency: 'usd', amount_total: 2900, credit_amount: 300, pack_kind: 'hosted_credits' }] }) }
      }
      if (url === '/api/app/orders') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/checkout') {
        expect(init?.method).toBe('POST')
        expect(init?.body).toBe(JSON.stringify({ pack_code: 'hosted-300' }))
        return { ok: true, status: 200, json: async () => ({ data: { order: { id: 41, status: 'pending' }, checkout_url: 'https://stripe.test/abc' } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage('/billing?pack=hosted-300&autostart=1')

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/app/checkout', expect.objectContaining({ method: 'POST' }))
    })
    const checkoutCalls = fetchMock.mock.calls.filter((call: unknown[]) => call[0] === '/api/app/checkout')
    expect(checkoutCalls).toHaveLength(1)
    expect(redirectSpy).toHaveBeenCalledWith('https://stripe.test/abc')
  })

  it('does not auto-start twice on re-render', async () => {
    const redirectSpy = vi.spyOn(navigation, 'redirectTo').mockImplementation(() => {})
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return { ok: true, status: 200, json: async () => ({ data: [{ code: 'hosted-300', name: 'Hosted 300', description: 'x', currency: 'usd', amount_total: 2900, credit_amount: 300, pack_kind: 'hosted_credits' }] }) }
      }
      if (url === '/api/app/orders') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/checkout') {
        return { ok: true, status: 200, json: async () => ({ data: { order: { id: 42, status: 'pending' }, checkout_url: 'https://stripe.test/def' } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const { rerender } = renderPage('/billing?pack=hosted-300&autostart=1')

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/app/checkout', expect.objectContaining({ method: 'POST' }))
    })

    rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={['/billing?pack=hosted-300&autostart=1']}>
          <BillingPage />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    const checkoutCalls = fetchMock.mock.calls.filter((call: unknown[]) => call[0] === '/api/app/checkout')
    expect(checkoutCalls).toHaveLength(1)
  })

  it('ignores autostart when pack not in pricing list', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return { ok: true, status: 200, json: async () => ({ data: [{ code: 'hosted-300', name: 'Hosted 300', description: 'x', currency: 'usd', amount_total: 2900, credit_amount: 300, pack_kind: 'hosted_credits' }] }) }
      }
      if (url === '/api/app/orders') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage('/billing?pack=does-not-exist&autostart=1')

    await screen.findByText('Hosted 300')

    const checkoutCalls = fetchMock.mock.calls.filter((call: unknown[]) => call[0] === '/api/app/checkout')
    expect(checkoutCalls).toHaveLength(0)
  })

  it('renders hosted credit balance from overview and a success alert when reconcile finalizes a paid hosted-credits order', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/pricing') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/overview') {
        return { ok: true, status: 200, json: async () => ({ data: { api_key_count: 1, total_remaining: 0, hosted_credit_balance: 1100244, signup_credit_bonus: 100, reward_remaining: 0, invite_code: 'x', invite_limit: 0, invite_remaining: 0, reward_per_invite: 0, referral_count: 0, activated_referral_count: 0, discord_connected: false, discord_guild_member: false, recent_usage_count: 0, recent_orders_count: 4, pricing: [] } }) }
      }
      if (url === '/api/app/orders') {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/app/orders/reconcile') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              order: {
                id: 19,
                status: 'paid',
                currency: 'usd',
                amount_total: 100,
                pack_code: 'hosted-100',
                pack_name: 'Hosted 100',
                pack_kind: 'hosted_credits',
                quota_amount: 0,
                credit_amount: 100,
                stripe_checkout_session_id: 'cs_live_a1OF4f',
                stripe_payment_intent_id: 'pi_3TaSFn086Uy792wD0BVr1Zoz',
                created_at: '2026-05-24T02:58:42Z',
              },
              awaiting_confirmation: false,
            },
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage('/billing?status=success&session_id=cs_live_a1OF4f')

    expect(await screen.findByText('1,100,244')).toBeInTheDocument()
    expect(await screen.findByText(/Payment received — order #19 paid/)).toBeInTheDocument()
    expect(await screen.findByText(/100 hosted credits have been added/)).toBeInTheDocument()
  })
})
