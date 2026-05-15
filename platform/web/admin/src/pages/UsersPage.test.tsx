import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import UsersPage from './UsersPage'

const fetchMock = vi.fn()

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <UsersPage />
    </QueryClientProvider>,
  )
}

describe('admin users page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('shows user uid in the roster', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/users') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              { id: 42, email: 'demo@example.com', name: 'Demo User', invite_code: 'invite-000016', status: 'active', created_at: '2026-04-01T00:00:00Z' },
            ],
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText('42')).toBeInTheDocument()
    expect(screen.getByText('demo@example.com')).toBeInTheDocument()
  })

  it('shows owned api keys and quota for a user', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/users') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              { id: 42, email: 'demo@example.com', name: 'Demo User', invite_code: 'invite-000016', status: 'active', created_at: '2026-04-01T00:00:00Z' },
            ],
          }),
        }
      }
      if (url === '/api/admin/api-keys?user_id=42') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              {
                id: 7,
                key_prefix: 'cop_hosted_demo',
                plaintext_available: true,
                status: 'active',
                plan_name: 'Hosted',
                owner_user_id: 42,
                allowed_modes: 'hosted_only',
                hosted_enabled: true,
                default_runtime_mode: 'hosted',
                quota_used: 3,
                quota_total: 20,
                quota_remaining: 17,
                credit_balance: 120,
                credit_reserved: 5,
                created_at: '2026-04-02T00:00:00Z',
              },
            ],
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    const viewButton = await screen.findByRole('button', { name: /view api keys/i })
    fireEvent.click(viewButton)

    expect(await screen.findByText('cop_hosted_demo')).toBeInTheDocument()
    expect(screen.getByText('External 17 / 20')).toBeInTheDocument()
    expect(screen.getByText('Used 3')).toBeInTheDocument()
    expect(screen.getByText('Hosted credits account-level')).toBeInTheDocument()
    expect(screen.getByText('Key reserved legacy 5')).toBeInTheDocument()
  })
})
