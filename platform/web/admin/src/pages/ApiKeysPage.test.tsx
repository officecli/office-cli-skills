import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

describe('admin api keys page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('copies stored plaintext for supported keys and disables legacy ones', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/api-keys') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              {
                id: 3,
                key_prefix: 'cop_admin_live',
                plaintext_available: true,
                status: 'active',
                plan_name: 'Ops',
                quota_used: 2,
                quota_remaining: 8,
                created_at: '2026-04-01T00:00:00Z',
              },
              {
                id: 4,
                key_prefix: 'cop_admin_legacy',
                plaintext_available: false,
                status: 'disabled',
                plan_name: 'Legacy',
                quota_used: 5,
                quota_remaining: 0,
                created_at: '2026-03-01T00:00:00Z',
              },
            ],
          }),
        }
      }
      if (url === '/api/admin/api-keys/3/plaintext') {
        return {
          ok: true,
          status: 200,
          json: async () => ({ data: { plaintext_key: 'cop_admin_secret_123' } }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText(/cop_admin_live/i)).toBeInTheDocument()
    const buttons = screen.getAllByRole('button', { name: /copy full key/i })
    fireEvent.click(buttons[0])

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('cop_admin_secret_123')
    })
    expect(await screen.findByRole('button', { name: /copied/i })).toBeInTheDocument()
    expect(screen.getByText(/cannot be copied again/i)).toBeInTheDocument()
    expect(buttons[1]).toBeDisabled()
  })
})
