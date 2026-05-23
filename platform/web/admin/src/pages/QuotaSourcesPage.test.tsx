import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import QuotaSourcesPage from './QuotaSourcesPage'

const fetchMock = vi.fn()

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <QuotaSourcesPage />
    </QueryClientProvider>,
  )
}

describe('admin quota sources page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('renders reward grants surfaced from quota sources', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/quota-sources?') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              reward_grants: [{
                id: 11,
                user_id: 42,
                source_type: 'invite_activation_reward',
                amount_total: 10,
                amount_used: 4,
                reason: 'invite activation reward',
                metadata_json: '{}',
                created_at: '2026-05-20T00:00:00Z',
                updated_at: '2026-05-20T03:00:00Z',
              }],
              paid_external_keys: [],
              hosted_keys: [],
            },
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText(/invite activation reward/i)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Reward grants/i })).toBeInTheDocument()
  })
})
