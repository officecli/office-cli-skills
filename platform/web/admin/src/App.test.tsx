import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'
import App from './App'

vi.mock('echarts/core', () => ({
  init: () => ({
    setOption: vi.fn(),
    resize: vi.fn(),
    dispose: vi.fn(),
  }),
  use: vi.fn(),
}))

vi.mock('echarts/charts', () => ({ LineChart: {}, PieChart: {} }))
vi.mock('echarts/components', () => ({ GridComponent: {}, LegendComponent: {}, TooltipComponent: {} }))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

const fetchMock = vi.fn()

function requestBody(call: unknown[]) {
  return JSON.parse(String((call[1] as RequestInit).body))
}

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
    expect(document.title).toBe('OfficeCLI Admin | Page Not Found')
    expect(screen.queryByText(/Authorized company accounts only/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Continue with company OAuth2/i)).not.toBeInTheDocument()
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
    expect(document.title).toBe('OfficeCLI Admin | Access Denied')
    expect(screen.getByText(/blocked@example.com/i)).toBeInTheDocument()
  })

  it('redirects unauthenticated protected routes to the admin OAuth2 login endpoint', async () => {
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

  it('renders overview trend and breakdown charts', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url === '/api/admin/overview') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              total_api_keys: 9,
              active_api_keys: 6,
              disabled_api_keys: 2,
              expired_api_keys: 1,
              free_machines: 4,
              checks_last_24h: 12,
              consumes_last_24h: 8,
              blocked_last_24h: 3,
              total_users: 5,
              paid_orders_last_24h: 1,
              paid_quota_added_last_24h: 100,
              remaining_paid_quota: 500,
              daily_new_users: [
                { date: '2026-05-12', users: 1 },
                { date: '2026-05-13', users: 2 },
              ],
              usage_trend: [
                { date: '2026-05-12', checks: 2, consumes: 1, blocked: 0, allowed: 3, total: 3 },
                { date: '2026-05-13', checks: 4, consumes: 3, blocked: 1, allowed: 6, total: 7 },
              ],
              result_breakdown: [
                { key: 'allowed', label: 'Allowed', value: 9 },
                { key: 'blocked', label: 'Blocked', value: 1 },
              ],
              mode_breakdown: [
                { key: 'free', label: 'Free', value: 3 },
                { key: 'reward', label: 'Reward', value: 2 },
                { key: 'paid', label: 'Paid', value: 1 },
                { key: 'hosted', label: 'Hosted', value: 4 },
              ],
              api_key_status_breakdown: [
                { key: 'active', label: 'Active', value: 6 },
                { key: 'disabled', label: 'Disabled', value: 2 },
                { key: 'expired', label: 'Expired', value: 1 },
                { key: 'other', label: 'Other', value: 0 },
              ],
            },
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /7-day usage trend/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /Result mix/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /Mode mix/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /API key status/i })).toBeInTheDocument()
    expect(await screen.findAllByTestId('echarts-line-chart')).toHaveLength(2)
    expect(await screen.findAllByTestId('echarts-pie-chart')).toHaveLength(3)
  })

  it('shows overview chart empty states when chart data is empty', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url === '/api/admin/overview') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              total_api_keys: 0,
              active_api_keys: 0,
              disabled_api_keys: 0,
              expired_api_keys: 0,
              free_machines: 0,
              checks_last_24h: 0,
              consumes_last_24h: 0,
              blocked_last_24h: 0,
              total_users: 0,
              paid_orders_last_24h: 0,
              paid_quota_added_last_24h: 0,
              remaining_paid_quota: 0,
              usage_trend: [],
              result_breakdown: [],
              mode_breakdown: [],
              api_key_status_breakdown: [],
            },
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /7-day usage trend/i })).toBeInTheDocument()
    expect((await screen.findAllByText(/No chart data available/i)).length).toBeGreaterThanOrEqual(4)
  })

  it('renders operations funnel page and dashboard funnel entry', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url === '/api/admin/overview') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              total_api_keys: 0,
              active_api_keys: 0,
              disabled_api_keys: 0,
              expired_api_keys: 0,
              free_machines: 0,
              checks_last_24h: 0,
              consumes_last_24h: 0,
              blocked_last_24h: 0,
              total_users: 0,
              paid_orders_last_24h: 0,
              paid_quota_added_last_24h: 0,
              remaining_paid_quota: 0,
              usage_trend: [],
              result_breakdown: [],
              mode_breakdown: [],
              api_key_status_breakdown: [],
            },
          }),
        }
      }
      if (url.startsWith('/api/admin/operations/funnel?')) {
        return { ok: true, status: 200, json: async () => ({ data: operationsFunnelFixture() }) }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/operations/funnel']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /Acquisition, activation, usage, and revenue/i })).toBeInTheDocument()
    expect(document.title).toBe('OfficeCLI Admin | Operations Funnel')
    expect(await screen.findByText('Total Machines')).toBeInTheDocument()
    expect(await screen.findByText('Paid Conversion')).toBeInTheDocument()
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
    expect(document.title).toBe('OfficeCLI Admin | Usage Events')
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('mode=reward'), expect.objectContaining({ credentials: 'include' }))
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('mode=hosted'), expect.objectContaining({ credentials: 'include' }))
  })

  it('renders usage event audit details and sends audit filters', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url.startsWith('/api/admin/usage-events?')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 99,
              request_id: 'req-audit-99',
              fingerprint_hash: 'fp-audit-machine',
              mode: 'hosted',
              action: 'generate',
              result: 'allowed',
              api_key_id: 7,
              user_id: 42,
              client_ip: '203.0.113.42',
              forwarded_for: '198.51.100.7, 10.0.0.4',
              user_agent: 'officecli/0.2.65 audit-test',
              request_host: 'platform.officecli.io',
              request_path: '/api/llm/v1/text',
              request_method: 'POST',
              cli_version: '0.2.65',
              document_type: 'pptx',
              runtime_mode: 'hosted',
              provider: 'aigateway',
              model_name: 'gpt-4.1',
              prompt_tokens: 123,
              completion_tokens: 45,
              reasoning_tokens: 6,
              image_count: 2,
              reserved_credits: 20,
              settled_credits: 8,
              refund_credits: 12,
              upstream_cost_microusd: 30000,
              profit_microusd: 50000,
              cap_applied: true,
              created_at: '2026-05-15T08:00:00Z',
            }],
          }),
        }
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

    expect((await screen.findAllByRole('table')).length).toBeGreaterThan(0)
    expect(document.querySelector('.max-w-7xl')).not.toBeInTheDocument()
    expect((await screen.findAllByText(/203\.0\.113\.42/i)).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/fp-audit-machine/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/req-audit-99/i)).toBeInTheDocument()
    expect(screen.getByText(/officecli\/0\.2\.65 audit-test/i)).toBeInTheDocument()
    expect(screen.getByText(/198\.51\.100\.7, 10\.0\.0\.4/i)).toBeInTheDocument()
    expect(screen.getByText(/gpt-4\.1/i)).toBeInTheDocument()
    expect(screen.getByText(/174 total tokens/i)).toBeInTheDocument()
    expect(screen.getByText(/prompt 123 \/ completion 45 \/ reasoning 6/i)).toBeInTheDocument()
    expect(screen.getByText(/reserved 20 \/ settled 8 \/ refund 12/i)).toBeInTheDocument()
    expect(screen.getByText(/capped/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/API key ID/i), { target: { value: '7' } })
    fireEvent.change(screen.getByLabelText(/User ID/i), { target: { value: '42' } })
    fireEvent.change(screen.getByLabelText(/Client IP/i, { selector: 'input' }), { target: { value: '203.0.113' } })
    fireEvent.change(screen.getByLabelText(/Request ID/i, { selector: 'input' }), { target: { value: 'req-audit' } })
    fireEvent.click(screen.getByRole('button', { name: /Apply filters/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/usage-events?mode=free&mode=reward&mode=paid&mode=hosted&result=allowed&result=blocked&action=generate&api_key_id=7&user_id=42&client_ip=203.0.113&request_id=req-audit', expect.objectContaining({ credentials: 'include' }))
    })
  })

  it('loads and saves usage-event table column preferences', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url === '/api/admin/preferences/usage-events-table' && init?.method === 'PUT') {
        return { ok: true, status: 200, json: async () => ({ data: requestBody([input, init]) }) }
      }
      if (url === '/api/admin/preferences/usage-events-table') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              version: 1,
              columns: [
                { key: 'created_at', visible: true, fixed: 'left', width: 210 },
                { key: 'mode_result', visible: true },
                { key: 'user_agent', visible: false },
              ],
            },
          }),
        }
      }
      if (url.startsWith('/api/admin/usage-events?')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 100,
              request_id: 'req-config-100',
              fingerprint_hash: 'fp-config-machine',
              mode: 'paid',
              action: 'generate',
              result: 'blocked',
              client_ip: '203.0.113.100',
              user_agent: 'hidden-agent',
              created_at: '2026-05-15T09:00:00Z',
            }],
          }),
        }
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

    expect(await screen.findByRole('columnheader', { name: /Timestamp/i })).toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: /User-Agent/i })).not.toBeInTheDocument()

    fireEvent.mouseDown(screen.getAllByRole('button', { name: /Resize Timestamp/i })[0], { clientX: 210 })
    fireEvent.mouseMove(document, { clientX: 260 })
    fireEvent.mouseUp(document, { clientX: 260 })

    await waitFor(() => {
      const resizeCall = fetchMock.mock.calls.find(([url, init]) => {
        if (url !== '/api/admin/preferences/usage-events-table' || (init as RequestInit)?.method !== 'PUT') return false
        return requestBody([url, init]).columns?.some((column: { key: string; width?: number }) => column.key === 'created_at' && column.width === 260)
      })
      expect(resizeCall).toBeTruthy()
    })

    fireEvent.click(screen.getByRole('button', { name: /Columns/i }))
    fireEvent.click(await screen.findByRole('menuitemcheckbox', { name: /User-Agent/i }))

    await waitFor(() => {
      const saveCall = fetchMock.mock.calls.find(([url, init]) => {
        if (url !== '/api/admin/preferences/usage-events-table' || (init as RequestInit)?.method !== 'PUT') return false
        return requestBody([url, init]).columns?.some((column: { key: string; visible?: boolean }) => column.key === 'user_agent' && column.visible === true)
      })
      expect(saveCall).toBeTruthy()
      expect(requestBody(saveCall!)).toMatchObject({
        version: 1,
        columns: expect.arrayContaining([
          expect.objectContaining({ key: 'user_agent', visible: true }),
        ]),
      })
    })
  })

  it('renders hosted billing controls from the DB-backed admin API', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url === '/api/admin/hosted-billing') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              settings: { id: 1, markup_bps: 3500, currency: 'usd', credits_per_usd: 100 },
              model_configs: [
                { id: 4, key: 'text_default', kind: 'text', provider: 'aigateway', model: 'gpt-4.1', prompt_per_1m_cost_microusd: 1000000, output_per_1m_cost_microusd: 2000000, reasoning_per_1m_cost_microusd: 4000000, prompt_per_1m_cost_credits: 100, output_per_1m_cost_credits: 200, reasoning_per_1m_cost_credits: 400, enabled: true },
                { id: 5, key: 'image_default', kind: 'image', provider: 'aigateway', model: 'gpt-image-2', prompt_per_1m_cost_microusd: 8000000, output_per_1m_cost_microusd: 30000000, reasoning_per_1m_cost_microusd: 0, prompt_per_1m_cost_credits: 800, output_per_1m_cost_credits: 3000, reasoning_per_1m_cost_credits: 0, enabled: true },
              ],
              rules: [{ id: 9, document_profile: 'text', provider: 'aigateway', model: 'gpt-4.1', text_model_key: 'text_default', image_model_key: '', prompt_per_1k_cost_microusd: 10000, output_per_1k_cost_microusd: 20000, reasoning_per_1k_cost_microusd: 40000, image_per_asset_cost_microusd: 0, minimum_charge_credits: 2, markup_bps: 5000, enabled: true }],
              packs: [{ id: 3, code: 'hosted-300', name: 'Hosted 300', description: '300 hosted credits', currency: 'usd', amount_total: 300, credit_amount: 300, enabled: true }],
            },
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/hosted-pricing']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByRole('heading', { name: /Hosted pricing controls/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /Model pricing/i })).toBeInTheDocument()
    expect((await screen.findAllByText(/text_default \/ gpt-4\.1/i)).length).toBeGreaterThan(0)
    expect(await screen.findByText(/prompt \$1\.00 \/ output \$2\.00 \/ reasoning \$4\.00 USD per 1M tokens/i)).toBeInTheDocument()
    const promptUSDInput = screen.getByLabelText(/Prompt USD per 1M text_default/i) as HTMLInputElement
    expect(promptUSDInput.value).toBe('1.00')
    expect(screen.queryByLabelText(/Prompt credits per 1M text_default/i)).not.toBeInTheDocument()
    expect(await screen.findByText(/Text generation \/ text text_default/i)).toBeInTheDocument()
    const profileSelect = screen.getByLabelText(/Profile 9/i) as HTMLSelectElement
    expect(profileSelect).toHaveValue('text')
    expect(screen.getAllByRole('option', { name: /Text generation/i }).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('option', { name: /Image generation/i }).length).toBeGreaterThan(0)
    expect(screen.getByLabelText(/Text model 9/i)).toHaveValue('text_default')
    expect(screen.getAllByRole('option', { name: /None/i }).length).toBeGreaterThan(0)
    expect(screen.queryByText(/microUSD/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Price cents/i)).not.toBeInTheDocument()
    expect(await screen.findByText(/hosted-300/i)).toBeInTheDocument()

    fireEvent.change(promptUSDInput, { target: { value: '0' } })
    expect(promptUSDInput.value).toBe('0')
    fireEvent.change(promptUSDInput, { target: { value: '0.' } })
    expect(promptUSDInput.value).toBe('0.')
    fireEvent.change(promptUSDInput, { target: { value: '0.36' } })
    expect(promptUSDInput.value).toBe('0.36')
    fireEvent.click(await screen.findByRole('button', { name: /Save hosted model pricing config text_default/i }))
    fireEvent.click(await screen.findByRole('button', { name: /Save hosted pricing rule 9/i }))
    fireEvent.click(await screen.findByRole('button', { name: /Save hosted credit pack 3/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/hosted-model-pricing-configs/4', expect.objectContaining({ method: 'PATCH' }))
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/hosted-pricing-rules/9', expect.objectContaining({ method: 'PATCH' }))
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/hosted-credit-packs/3', expect.objectContaining({ method: 'PATCH' }))
    })
    const modelPatch = fetchMock.mock.calls.find(([url, init]) => url === '/api/admin/hosted-model-pricing-configs/4' && (init as RequestInit)?.method === 'PATCH')?.[1] as RequestInit
    expect(JSON.parse(String(modelPatch.body))).toMatchObject({ prompt_per_1m_cost_microusd: 360000, prompt_per_1m_cost_credits: 36 })
  })

  it('keeps hosted credit ratio locked and requires confirmation before saving changes', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/session') {
        return { ok: true, status: 200, json: async () => ({ data: { email: 'admin@example.com', name: 'Admin User', auth_method: 'google' } }) }
      }
      if (url === '/api/admin/hosted-billing') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              settings: { id: 1, markup_bps: 3500, currency: 'usd', credits_per_usd: 100 },
              model_configs: [],
              rules: [],
              packs: [],
            },
          }),
        }
      }
      if (url === '/api/admin/hosted-pricing-settings') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 1, markup_bps: 3500, currency: 'usd', credits_per_usd: 200 } }) }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    const confirmMock = vi.fn()
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true)
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true)
    vi.stubGlobal('confirm', confirmMock)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/hosted-pricing']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    const ratioInput = await screen.findByLabelText(/Credits per USD/i)
    expect(ratioInput).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: /Unlock credit ratio/i }))
    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(ratioInput).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: /Unlock credit ratio/i }))
    expect(confirmMock).toHaveBeenCalledTimes(2)
    expect(ratioInput).not.toBeDisabled()

    fireEvent.change(ratioInput, { target: { value: '200' } })
    fireEvent.click(screen.getByRole('button', { name: /Save pricing settings/i }))
    expect(confirmMock).toHaveBeenCalledTimes(3)
    expect(fetchMock.mock.calls.some(([url, init]) => url === '/api/admin/hosted-pricing-settings' && (init as RequestInit)?.method === 'PATCH')).toBe(false)

    fireEvent.click(screen.getByRole('button', { name: /Save pricing settings/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/hosted-pricing-settings', expect.objectContaining({ method: 'PATCH' }))
    })
    const settingsPatch = fetchMock.mock.calls.find(([url, init]) => url === '/api/admin/hosted-pricing-settings' && (init as RequestInit)?.method === 'PATCH')
    expect(requestBody(settingsPatch!)).toMatchObject({ markup_bps: 3500, currency: 'usd', credits_per_usd: 200 })
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
    expect(document.title).toBe('OfficeCLI Admin | Growth')
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
  })
})

function operationsFunnelFixture() {
  return {
    window_start: '2026-04-20T00:00:00Z',
    window_end: '2026-05-20T00:00:00Z',
    visitors: 10,
    pricing_views: 8,
    cta_clicks: 6,
    login_starts: 5,
    registered_users: 4,
    activated_users: 3,
    activated_registered_users: 3,
    activation_rate: 0.75,
    cli_login_started: 3,
    cli_login_completed: 2,
    cli_sessions_created: 2,
    first_usage_users: 2,
    repeat_usage_users: 1,
    blocked_rate: 0.1,
    checkout_starts: 2,
    orders_created: 2,
    paid_orders: 1,
    paid_users: 1,
    paid_registered_users: 1,
    revenue: 2900,
    paid_conversion_rate: 0.25,
    checkout_to_paid_rate: 0.5,
    repeat_paid_users: 0,
    machine_quality: { total_machines: 9, active_machines_24h: 3, active_machines_7d: 7 },
    usage_quality: { first_usage_users: 2, repeat_usage_users: 1, blocked_rate: 0.1 },
    revenue_quality: { paid_orders: 1, paid_users: 1, paid_registered_users: 1, revenue: 2900, repeat_paid_users: 0, paid_conversion_rate: 0.25, checkout_to_paid_rate: 0.5 },
  }
}
