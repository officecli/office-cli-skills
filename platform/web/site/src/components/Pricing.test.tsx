import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Pricing from './Pricing'

const hostedPacks = [
  { code: 'hosted-100', name: 'Hosted 100', description: '100 hosted credits', currency: 'usd', amount_total: 100, quota_amount: 0, credit_amount: 100, pack_kind: 'hosted_credits' },
  { code: 'hosted-300', name: 'Hosted 300', description: '300 hosted credits', currency: 'usd', amount_total: 2900, quota_amount: 0, credit_amount: 300, pack_kind: 'hosted_credits' },
  { code: 'hosted-1200', name: 'Hosted 1200', description: '1200 hosted credits', currency: 'usd', amount_total: 9900, quota_amount: 0, credit_amount: 1200, pack_kind: 'hosted_credits' },
]

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('Pricing component', () => {
  it('renders 3 hosted credit pack cards with deep-link CTAs', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
      if (String(input) === '/api/events/track') {
        return { ok: true, json: async () => ({ data: { success: true } }) } as Response
      }
      if (String(input) === '/api/pricing') {
        return { ok: true, json: async () => ({ data: hostedPacks }) } as Response
      }
      throw new Error(`unexpected request: ${String(input)}`)
    })

    render(
      <MemoryRouter initialEntries={['/pricing']}>
        <Pricing standalone />
      </MemoryRouter>,
    )

    const ctas = await screen.findAllByRole('link', { name: /Open Secure Checkout/i })
    expect(ctas).toHaveLength(3)

    const hrefs = ctas.map(cta => cta.getAttribute('href')!)
    expect(hrefs.some(h => h.includes('pack=hosted-100') && h.includes('autostart=1'))).toBe(true)
    expect(hrefs.some(h => h.includes('pack=hosted-300') && h.includes('autostart=1'))).toBe(true)
    expect(hrefs.some(h => h.includes('pack=hosted-1200') && h.includes('autostart=1'))).toBe(true)
  })

  it('shows exactly one Best value badge on the lowest $/credit pack', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
      if (String(input) === '/api/events/track') {
        return { ok: true, json: async () => ({ data: { success: true } }) } as Response
      }
      if (String(input) === '/api/pricing') {
        return { ok: true, json: async () => ({ data: hostedPacks }) } as Response
      }
      throw new Error(`unexpected request: ${String(input)}`)
    })

    render(
      <MemoryRouter initialEntries={['/pricing']}>
        <Pricing standalone />
      </MemoryRouter>,
    )

    await screen.findAllByRole('link', { name: /Open Secure Checkout/i })

    const bestValueBadges = screen.getAllByText('Best value')
    expect(bestValueBadges).toHaveLength(1)

    expect(screen.getByText('Hosted 100')).toBeInTheDocument()
  })

  it('renders pack names for all three tiers', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
      if (String(input) === '/api/events/track') {
        return { ok: true, json: async () => ({ data: { success: true } }) } as Response
      }
      if (String(input) === '/api/pricing') {
        return { ok: true, json: async () => ({ data: hostedPacks }) } as Response
      }
      throw new Error(`unexpected request: ${String(input)}`)
    })

    render(
      <MemoryRouter initialEntries={['/pricing']}>
        <Pricing standalone />
      </MemoryRouter>,
    )

    expect(await screen.findByText('Hosted 100')).toBeInTheDocument()
    expect(screen.getByText('Hosted 300')).toBeInTheDocument()
    expect(screen.getByText('Hosted 1200')).toBeInTheDocument()
  })
})
