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

  it('explains daily free quota rows as a legacy audit and compatibility view', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/quota-sources?') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              free_trial_devices: [{
                id: 3,
                fingerprint_hash: 'fp-daily-legacy',
                usage_date: '2026-05-20',
                daily_limit: 5,
                daily_used: 2,
                remaining: 3,
                created_at: '2026-05-20T00:00:00Z',
                updated_at: '2026-05-20T03:00:00Z',
              }],
              reward_grants: [],
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

    expect(await screen.findByText(/fp-daily-legacy/i)).toBeInTheDocument()
    expect(screen.getByText(/daily_free_quotas rows are a legacy daily audit and compatibility view/i)).toBeInTheDocument()
    expect(screen.getByText(/current anonymous license state is tracked in free_quotas/i)).toBeInTheDocument()
  })
})
