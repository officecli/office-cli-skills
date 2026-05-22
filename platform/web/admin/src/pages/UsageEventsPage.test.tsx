import { render, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import UsageEventsPage from './UsageEventsPage'

const fetchMock = vi.fn()

function renderWithUrl(initialUrl: string) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <UsageEventsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function mockEmpty() {
  fetchMock.mockImplementation(async () => ({
    ok: true,
    status: 200,
    json: async () => ({ data: [] }),
  }))
  vi.stubGlobal('fetch', fetchMock)
}

describe('admin usage events page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('pre-populates the user_id filter from the URL search params', async () => {
    mockEmpty()
    renderWithUrl('/usage-events?user_id=42')

    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((call) => String(call[0]))
      expect(urls.some((url) => url.startsWith('/api/admin/usage-events') && url.includes('user_id=42'))).toBe(true)
    })
  })

  it('queries without user_id when the URL has no user_id param', async () => {
    mockEmpty()
    renderWithUrl('/usage-events')

    await waitFor(() => {
      const usageCalls = fetchMock.mock.calls
        .map((call) => String(call[0]))
        .filter((url) => url.startsWith('/api/admin/usage-events'))
      expect(usageCalls.length).toBeGreaterThan(0)
      expect(usageCalls.every((url) => !url.includes('user_id='))).toBe(true)
    })
  })
})
