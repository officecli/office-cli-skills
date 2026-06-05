import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import { afterEach, describe, expect, it, vi } from 'vitest'
import RedemptionCodesPage from './RedemptionCodesPage'

const fetchMock = vi.fn()

function renderPage() {
  return render(
    <AntApp>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RedemptionCodesPage />
      </QueryClientProvider>
    </AntApp>,
  )
}

describe('admin redemption codes page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('deletes a redemption code after confirmation', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/api/admin/redemption-codes?') && (!init || init.method === undefined)) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              total: 1,
              items: [{
                id: 12,
                code: 'SUMMER2026',
                credit_amount: 100,
                max_redemptions: null,
                redemptions_used: 0,
                per_user_limit: 1,
                status: 'disabled',
                expires_at: null,
                notes: 'Launch promo',
                created_by: 'admin@example.com',
                created_at: '2026-06-01T00:00:00Z',
                updated_at: '2026-06-01T00:00:00Z',
              }],
            },
          }),
        }
      }
      if (url === '/api/admin/redemption-codes/12' && init?.method === 'DELETE') {
        return { ok: true, status: 200, json: async () => ({ data: { success: true } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn(() => true))

    renderPage()

    expect(await screen.findByText('Redemption Code Management')).toBeInTheDocument()
    expect(await screen.findByText('SUMMER2026')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /delete/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/redemption-codes/12', expect.objectContaining({ method: 'DELETE' }))
    })
  })
})
