import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'
import App from './App'

const fetchMock = vi.fn()

describe('platform admin shell', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('renders a generic 404 page for /login without exposing admin details', async () => {
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/login']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /Page not found/i })).toBeInTheDocument()
    expect(screen.queryByText(/Authorized Google accounts only/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Continue with Google/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/luyang950@gmail.com/i)).not.toBeInTheDocument()
  })

  it('renders the access denied route with the blocked email context', async () => {
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/access-denied?email=blocked@example.com']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /Access not granted/i })).toBeInTheDocument()
    expect(screen.getByText(/blocked@example.com/i)).toBeInTheDocument()
  })

  it('redirects unauthenticated protected routes to the admin Google login endpoint', async () => {
    fetchMock.mockResolvedValue({ ok: false, status: 401, json: async () => ({ error: 'unauthorized' }) })
    vi.stubGlobal('fetch', fetchMock)

    vi.spyOn(window, 'location', 'get').mockReturnValue({
      ...window.location,
      pathname: '/admin/users',
      search: '?tab=ops',
      hash: '#team',
      href: 'https://platform.officecli.io/admin/users?tab=ops#team',
    } as Location)
    const loginSpy = vi.spyOn(api, 'login').mockImplementation(() => {})

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/users?tab=ops#team']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    await waitFor(() => {
      expect(loginSpy).toHaveBeenCalledWith('/admin/users?tab=ops#team')
    })
  })

  it('shows reward as an available usage-event mode filter', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url.startsWith('/api/admin/usage-events?')) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/usage-events']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /Recent usage events/i })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'reward' })).toBeInTheDocument()
  })

  it('renders the growth ledger route from the real growth API', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url === '/api/admin/growth') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              reward_grants: [{ id: 1, user_id: 42, source_type: 'invite_activation_reward', idempotency_key: 'invite-activation:42', amount_total: 5, amount_used: 1, reason: 'invite activation reward', metadata_json: '{}', created_at: '2026-04-01T00:00:00Z', updated_at: '2026-04-01T00:00:00Z' }],
              referrals: [{ id: 1, inviter_user_id: 42, invited_user_id: 99, invite_code: 'invite-xyz', registered_at: '2026-04-01T00:00:00Z', created_at: '2026-04-01T00:00:00Z', updated_at: '2026-04-01T00:00:00Z' }],
              discord_connections: [{ id: 1, user_id: 42, discord_user_id: 'discord-42', username: 'officecli-user', guild_member: false, connected_at: '2026-04-02T00:00:00Z', created_at: '2026-04-02T00:00:00Z', updated_at: '2026-04-02T00:00:00Z' }],
            },
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/growth']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /Reward grants, referrals, and Discord connections/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/growth', expect.anything())
    })
  })

  it('renders the quota sources route from the real admin API', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url === '/api/admin/quota-sources?') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              free_trial_devices: [{ id: 1, fingerprint_hash: 'fp-demo-01', usage_date: '2026-04-16', daily_limit: 10, daily_used: 2, remaining: 8, created_at: '2026-04-16T00:00:00Z', updated_at: '2026-04-16T01:00:00Z' }],
              reward_grants: [],
              paid_external_keys: [],
              hosted_keys: [],
            },
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/quota-sources']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /Quota sources/i })).toBeInTheDocument()
    expect(await screen.findByText(/fp-demo-01/i)).toBeInTheDocument()
  })
})
