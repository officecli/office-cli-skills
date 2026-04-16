import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { api } from './api'

const fetchMock = vi.fn()

function renderApp(path = '/') {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('platform app shell', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('shows a minimal login page when no user session exists', async () => {
    fetchMock.mockResolvedValue({ ok: false, status: 401, json: async () => ({ error: 'unauthorized' }) })
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/')

    expect(await screen.findByRole('heading', { name: /Authorized Google accounts only/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Continue with Google/i })).toBeInTheDocument()
    expect(screen.queryByText(/Production document control for every workflow/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Issue production keys/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Attach credits to a target key/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Ship faster from the terminal/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/What unlocks after sign-in/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/\$ officecli auth status/i)).not.toBeInTheDocument()
  })

  it('keeps the requested billing destination through login', async () => {
    fetchMock.mockResolvedValue({ ok: false, status: 401, json: async () => ({ error: 'unauthorized' }) })
    vi.stubGlobal('fetch', fetchMock)
    const loginSpy = vi.spyOn(api, 'login').mockImplementation(() => {})

    renderApp('/billing?status=success')

    expect(await screen.findByRole('heading', { name: /Authorized Google accounts only/i })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Continue with Google/i }))

    expect(loginSpy).toHaveBeenCalledWith('/billing?status=success')
  })

  it('renders the access denied route with the blocked email context', async () => {
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/access-denied?email=blocked@example.com')

    expect(await screen.findByRole('heading', { name: /Access not granted/i })).toBeInTheDocument()
    expect(screen.getByText('blocked@example.com')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Try another Google account/i })).toHaveAttribute('href', '/api/auth/google/login?return_to=%2Fapp')
  })

  it('renders the overview after login and shows the remaining credits board', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/auth/me') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 1, email: 'user@example.com', name: 'Demo User', status: 'active' } }) }
      }
      if (url === '/api/app/overview') {
        return { ok: true, status: 200, json: async () => ({ data: { api_key_count: 2, total_remaining: 50, reward_remaining: 6, invite_code: 'invite-abc', invite_limit: 5, invite_remaining: 2, reward_per_invite: 2, referral_count: 3, activated_referral_count: 1, discord_connected: true, discord_guild_member: false, recent_usage_count: 4, recent_orders_count: 1, pricing: [] } }) }
      }
      if (url === '/api/app/growth') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              invite_code: 'invite-abc',
              invite_limit: 5,
              invite_remaining: 4,
              reward_per_invite: 2,
              reward_remaining: 6,
              reward_grants: [{ source_type: 'invite_activation_reward', amount_total: 2, amount_used: 0, remaining: 2, reason: 'invite activation reward', metadata_json: '{}', created_at: '2026-04-01T00:00:00Z', updated_at: '2026-04-01T00:00:00Z' }],
              referrals: [{ invite_code: 'invite-abc', registered_at: '2026-04-01T00:00:00Z' }],
              discord_connection: { username: 'officecli-user', guild_member: false, connected_at: '2026-04-02T00:00:00Z', verification_status: 'verification_blocked', verification_blocked_reason: 'discord guild verification is not configured in this build yet' },
            },
          }),
        }
      }
      if (url === '/api/app/api-keys') {
        return { ok: true, status: 200, json: async () => ({ data: [{ id: 9, key_prefix: 'cop_live_demo', status: 'active', plan_name: 'Production', quota_total: 100, quota_used: 50, quota_remaining: 50, created_at: '2026-04-01T00:00:00Z', last_used_at: '2026-04-01T01:00:00Z' }] }) }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/')

    expect(await screen.findByRole('heading', { name: /Remaining Credits/i })).toBeInTheDocument()
    expect(screen.getAllByText(/Reward Credits/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Referral Progress/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Discord Status/i).length).toBeGreaterThan(0)
    expect(screen.getByRole('heading', { name: /Reward grants and referral progress/i })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Link Discord for growth rewards/i })).toBeInTheDocument()
  })

  it('renders the usage page empty state when the API returns null data', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/auth/me') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 1, email: 'user@example.com', name: 'Demo User', status: 'active' } }) }
      }
      if (url === '/api/app/usage-events') {
        return { ok: true, status: 200, json: async () => ({ data: null }) }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/usage')

    expect(await screen.findByRole('heading', { name: /Recent workflow usage/i })).toBeInTheDocument()
    expect(await screen.findByText(/No usage events recorded/i)).toBeInTheDocument()
  })

  it('renders the quota page with reward and paid account quota only', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/auth/me') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 1, email: 'user@example.com', name: 'Demo User', status: 'active' } }) }
      }
      if (url === '/api/app/quota-summary') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              reward_quota: {
                remaining: 6,
                grants: [{ source_type: 'invite_activation_reward', amount_total: 6, amount_used: 0, remaining: 6, reason: 'invite activation reward', metadata_json: '{}', created_at: '2026-04-01T00:00:00Z', updated_at: '2026-04-01T00:00:00Z' }],
              },
              paid_external_quota: {
                total_remaining: 40,
                keys: [{ id: 9, key_prefix: 'cop_live_demo', status: 'active', plan_name: 'Production', quota_total: 100, quota_used: 60, quota_remaining: 40, created_at: '2026-04-01T00:00:00Z' }],
              },
              trial_policy: {
                cli_binary_only: true,
                message: 'Anonymous trial counts only apply to the local officecli binary and never count as account balance.',
              },
            },
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/quota')

    expect(await screen.findByRole('heading', { name: /Reward and paid quota/i })).toBeInTheDocument()
    expect(await screen.findByText(/CLI trial only/i)).toBeInTheDocument()
    expect(await screen.findByText(/cop_live_demo/i)).toBeInTheDocument()
    expect(screen.queryByText(/current browser trial status/i)).not.toBeInTheDocument()
  })

  it('renders the downloads page with brew, npm, and the stable install script', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/auth/me') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 1, email: 'user@example.com', name: 'Demo User', status: 'active' } }) }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/downloads')

    expect(await screen.findByRole('heading', { name: /Install OfficeCLI for document operations/i })).toBeInTheDocument()
    expect(screen.getByText(/brew tap officecli\/officecli && brew install officecli/i)).toBeInTheDocument()
    expect(screen.getByText(/npm install -g officecli/i)).toBeInTheDocument()
    expect(screen.getByText(/raw\.githubusercontent\.com\/officecli\/officecli\/main\/scripts\/install-officecli\.sh/i)).toBeInTheDocument()
    expect(screen.queryByText(/officecli\.io\/install\.sh/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Sign in with Google from the terminal/i)).not.toBeInTheDocument()
  })
})
