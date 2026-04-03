import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FreeQuotasPage from './FreeQuotasPage'

const fetchMock = vi.fn()

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <FreeQuotasPage />
    </QueryClientProvider>,
  )
}

describe('admin free quotas page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('shows the latest free quota limits returned by the admin API', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/free-quotas?fingerprint=') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 3,
              fingerprint_hash: 'fp-demo-01',
              free_limit: 15,
              free_used: 3,
              created_at: '2026-04-01T00:00:00Z',
              updated_at: '2026-04-01T03:00:00Z',
            }],
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText(/fp-demo-01/i)).toBeInTheDocument()
    expect(screen.getByText('15')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
  })
})
