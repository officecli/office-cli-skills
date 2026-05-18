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

    expect(await screen.findByRole('heading', { name: /Continue to OfficeCLI/i })).toBeInTheDocument()
    expect(document.title).toBe('OfficeCLI App | Sign In')
    expect(screen.getByRole('button', { name: /Continue with company OAuth2/i })).toBeInTheDocument()
    expect(screen.queryByText(/Production document control for every workflow/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Issue production keys/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Buy quota for an API key/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Ship faster from the terminal/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/What unlocks after sign-in/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/\$ officecli auth status/i)).not.toBeInTheDocument()
  })

  it('keeps the requested billing destination through login', async () => {
    fetchMock.mockResolvedValue({ ok: false, status: 401, json: async () => ({ error: 'unauthorized' }) })
    vi.stubGlobal('fetch', fetchMock)
    const loginSpy = vi.spyOn(api, 'login').mockImplementation(() => {})

    renderApp('/billing?status=success')

    expect(await screen.findByRole('heading', { name: /Continue to OfficeCLI/i })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Continue with company OAuth2/i }))

    expect(loginSpy).toHaveBeenCalledWith('/billing?status=success')
  })

  it('renders the access denied route with the blocked email context', async () => {
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/access-denied?email=blocked@example.com')

    expect(await screen.findByRole('heading', { name: /Access not granted/i })).toBeInTheDocument()
    expect(screen.getByText('blocked@example.com')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Try another account/i })).toHaveAttribute('href', '/api/auth/oauth2/login?return_to=%2Fapp')
  })

  it('renders the overview after login and shows the remaining quota board', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/auth/me') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 1, email: 'user@example.com', name: 'Demo User', status: 'active' } }) }
      }
      if (url === '/api/app/overview') {
        return { ok: true, status: 200, json: async () => ({ data: { api_key_count: 2, total_remaining: 50, hosted_credit_balance: 30, signup_credit_bonus: 30, reward_remaining: 100, invite_code: 'invite-abc', invite_limit: 5, invite_remaining: 2, reward_per_invite: 100, referral_count: 3, activated_referral_count: 1, discord_connected: true, discord_guild_member: false, recent_usage_count: 4, recent_orders_count: 1, pricing: [] } }) }
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
              reward_per_invite: 100,
              reward_remaining: 100,
              reward_grants: [{ source_type: 'invite_activation_reward', amount_total: 100, amount_used: 0, remaining: 100, reason: 'invite activation reward', metadata_json: '{}', created_at: '2026-04-01T00:00:00Z', updated_at: '2026-04-01T00:00:00Z' }],
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

    expect(await screen.findByRole('heading', { name: /Account Hosted Credits/i })).toBeInTheDocument()
    expect(document.title).toBe('OfficeCLI App | Overview')
    expect(await screen.findByText(/^Invite Credits$/i)).toBeInTheDocument()
    expect(await screen.findByText(/^Referral Progress$/i)).toBeInTheDocument()
    expect(screen.getByText(/Invite code: invite-abc · 100 hosted credits per activated invite/i)).toBeInTheDocument()
    expect((await screen.findAllByText(/^Discord Status$/i)).length).toBeGreaterThan(0)
    expect(screen.getByRole('heading', { name: /Reward grants and referral progress/i })).toBeInTheDocument()
    expect(screen.getByText(/every activated referral adds 100 hosted credits/i)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Link Discord for 100 hosted credits/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Docs' })).toHaveAttribute('href', 'https://officecli.io/docs#invite-rewards')
    expect(screen.getByRole('link', { name: /How invite rewards work/i })).toHaveAttribute('href', 'https://officecli.io/docs#invite-rewards')
    expect(screen.getByRole('link', { name: /Full invite guide/i })).toHaveAttribute('href', 'https://officecli.io/docs#invite-rewards')
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
    expect(document.title).toBe('OfficeCLI App | Usage')
    expect(await screen.findByText(/No usage events recorded/i)).toBeInTheDocument()
  })

  it('renders hosted usage charges without exposing cost or profit', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/auth/me') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 1, email: 'user@example.com', name: 'Demo User', status: 'active' } }) }
      }
      if (url === '/api/app/usage-events') {
        return { ok: true, status: 200, json: async () => ({ data: [{ id: 1, mode: 'hosted', action: 'generate', result: 'allowed', fingerprint_hash: 'hosted', model_name: 'gpt-4.1', settled_credits: 7, upstream_cost_microusd: 20000, profit_microusd: 50000, created_at: '2026-04-03T00:00:00Z' }] }) }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/usage')

    expect(await screen.findByText('7 credits')).toBeInTheDocument()
    expect(screen.getByText('gpt-4.1')).toBeInTheDocument()
    expect(screen.queryByText(/\$0\.0200/)).not.toBeInTheDocument()
  })

  it('renders the quota page with legacy reward and external quota context', async () => {
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
                remaining: 10,
                grants: [{ source_type: 'legacy_reward', amount_total: 10, amount_used: 0, remaining: 10, reason: 'legacy reward grant', metadata_json: '{}', created_at: '2026-04-01T00:00:00Z', updated_at: '2026-04-01T00:00:00Z' }],
              },
              paid_external_quota: {
                total_remaining: 40,
                keys: [{ id: 9, key_prefix: 'cop_live_demo', status: 'active', plan_name: 'Production', quota_total: 100, quota_used: 60, quota_remaining: 40, created_at: '2026-04-01T00:00:00Z' }],
              },
              trial_policy: {
                cli_binary_only: true,
                message: 'External Mode uses your configured model endpoint and does not consume OfficeCLI quota.',
              },
            },
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderApp('/quota')

    expect(await screen.findByRole('heading', { name: /Historical reward and external quota/i })).toBeInTheDocument()
    expect(await screen.findByText(/External Mode free unlimited/i)).toBeInTheDocument()
    expect(await screen.findByText(/cop_live_demo/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /How invite rewards work/i })).toHaveAttribute('href', 'https://officecli.io/docs#invite-rewards')
    expect(screen.getByRole('link', { name: /Referral rules/i })).toHaveAttribute('href', 'https://officecli.io/docs#invite-rewards')
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
    expect(screen.getByText(/raw\.githubusercontent\.com\/officecli\/officecli-dist\/main\/scripts\/install-officecli\.sh/i)).toBeInTheDocument()
    expect(screen.queryByText(/officecli\.io\/install\.sh/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Sign in with Google from the terminal/i)).not.toBeInTheDocument()
  })
})
