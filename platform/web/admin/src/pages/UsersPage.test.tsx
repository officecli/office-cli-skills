import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import UsersPage from './UsersPage'

const fetchMock = vi.fn()

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={['/users']}>
        <Routes>
          <Route path="/users" element={<UsersPage />} />
        </Routes>
      </MemoryRouter>
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
              { id: 42, email: 'demo@example.com', name: 'Demo User', invite_code: 'invite-000016', status: 'active', paid_entitlement: true, paid_entitlement_source: 'stripe', credit_balance: 1250, created_at: '2026-04-01T00:00:00Z' },
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
    expect(screen.getByRole('columnheader', { name: 'Hosted credits' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Paid user' })).toBeInTheDocument()
    expect(screen.getByText('Paid')).toBeInTheDocument()
    expect(screen.getByText('1,250')).toBeInTheDocument()
  })

  it('can toggle paid user entitlement from the roster', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/users') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              { id: 42, email: 'demo@example.com', name: 'Demo User', invite_code: 'invite-000016', status: 'active', paid_entitlement: false, credit_balance: 1250, created_at: '2026-04-01T00:00:00Z' },
            ],
          }),
        }
      }
      if (url === '/api/admin/users/42' && init?.method === 'PATCH') {
        expect(JSON.parse(String(init.body))).toEqual({ paid_entitlement: true })
        return {
          ok: true,
          status: 200,
          json: async () => ({ data: { success: true } }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: /mark paid/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/admin/users/42',
        expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ paid_entitlement: true }) }),
      )
    })
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
  })

  it('opens usage-events in a new tab from the action link', async () => {
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

    const viewEventsLink = await screen.findByRole('link', { name: /^view events$/i })
    expect(viewEventsLink).toHaveAttribute('href', '/usage-events?user_id=42')
    expect(viewEventsLink).toHaveAttribute('target', '_blank')
    expect(viewEventsLink).toHaveAttribute('rel', 'noreferrer')
  })

  it('opens usage-events in a new tab from the user cell link', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/users') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              { id: 7, email: 'ops@example.com', name: 'Ops Operator', invite_code: 'invite-7', status: 'active', created_at: '2026-04-01T00:00:00Z' },
            ],
          }),
        }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    const userCellLink = await screen.findByRole('link', { name: /view usage events for ops operator/i })
    expect(userCellLink).toHaveAttribute('href', '/usage-events?user_id=7')
    expect(userCellLink).toHaveAttribute('target', '_blank')
    expect(userCellLink).toHaveAttribute('rel', 'noreferrer')
  })
})
