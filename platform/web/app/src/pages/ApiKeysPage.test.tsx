import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ApiKeysPage from './ApiKeysPage'

const fetchMock = vi.fn()

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ApiKeysPage />
    </QueryClientProvider>,
  )
}

describe('app api keys page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('shows the latest quota totals returned by the platform API', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/app/api-keys') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 7,
              key_prefix: 'cop_live_growth',
              status: 'active',
              plan_name: 'Growth 120',
              quota_total: 120,
              quota_used: 80,
              quota_remaining: 40,
              created_at: '2026-04-01T00:00:00Z',
              last_used_at: '2026-04-01T02:00:00Z',
            }],
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText(/cop_live_growth/i)).toBeInTheDocument()
    expect(screen.getByText('Growth 120')).toBeInTheDocument()
    expect(screen.getByText('80')).toBeInTheDocument()
    expect(screen.getByText('40')).toBeInTheDocument()
  })
})
