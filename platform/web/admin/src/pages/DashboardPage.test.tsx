import { fireEvent, render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import DashboardPage from './DashboardPage'

const echartsMock = vi.hoisted(() => {
  const chart = {
    setOption: vi.fn(),
    resize: vi.fn(),
    dispose: vi.fn(),
  }
  return {
    chart,
    init: vi.fn(() => chart),
    use: vi.fn(),
  }
})

vi.mock('echarts/core', () => ({
  init: echartsMock.init,
  use: echartsMock.use,
}))

vi.mock('echarts/charts', () => ({ LineChart: {}, PieChart: {} }))
vi.mock('echarts/components', () => ({ GridComponent: {}, LegendComponent: {}, TooltipComponent: {} }))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

const fetchMock = vi.fn()

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('admin dashboard fingerprint quality', () => {
  afterEach(() => {
    fetchMock.mockReset()
    echartsMock.init.mockClear()
    echartsMock.use.mockClear()
    echartsMock.chart.setOption.mockClear()
    echartsMock.chart.resize.mockClear()
    echartsMock.chart.dispose.mockClear()
    vi.unstubAllGlobals()
  })

  it('renders overview charts with ECharts value tooltips', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/overview') {
        return ok({
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
          daily_new_users: [
            { date: '2026-06-03', users: 1 },
            { date: '2026-06-04', users: 2 },
          ],
          usage_trend: [
            { date: '2026-06-03', checks: 2, consumes: 1, blocked: 0, allowed: 3, total: 3 },
            { date: '2026-06-04', checks: 4, consumes: 2, blocked: 1, allowed: 5, total: 6 },
          ],
          mode_breakdown: [
            { key: 'free', label: 'Free', value: 3 },
            { key: 'hosted', label: 'Hosted', value: 4 },
          ],
          result_breakdown: [
            { key: 'allowed', label: 'Allowed', value: 9 },
            { key: 'blocked', label: 'Blocked', value: 1 },
          ],
          api_key_status_breakdown: [
            { key: 'active', label: 'Active', value: 6 },
            { key: 'disabled', label: 'Disabled', value: 2 },
          ],
        })
      }
      if (url === '/api/admin/operations/funnel?range=30d') {
        return ok({ activated_users: 0, paid_users: 0, activation_rate: 0, paid_conversion_rate: 0, machine_quality: { total_machines: 0 } })
      }
      if (url === '/api/admin/fingerprint-quality') {
        return ok({ summary: [], rows: [] })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    const dailyHeading = await screen.findByRole('heading', { name: /daily new users/i })
    const usageHeading = await screen.findByRole('heading', { name: /7-day usage trend/i })
    expect(dailyHeading.compareDocumentPosition(usageHeading) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    expect(await screen.findAllByTestId('echarts-line-chart')).toHaveLength(2)
    expect(await screen.findAllByTestId('echarts-pie-chart')).toHaveLength(3)
    expect(echartsMock.init).toHaveBeenCalledTimes(5)

    const dailyOption = echartsMock.chart.setOption.mock.calls[0]?.[0] as {
      xAxis: { data: string[] }
      series: Array<{ name: string; data: number[] }>
      tooltip: { formatter: (params: Array<{ axisValue: string; seriesName: string; data: number }>) => string }
    }
    expect(dailyOption.xAxis.data).toEqual(['2026-06-03', '2026-06-04'])
    expect(dailyOption.series).toMatchObject([{ name: 'Users', data: [1, 2] }])
    expect(dailyOption.tooltip.formatter([{ axisValue: '2026-06-04', seriesName: 'Users', data: 1200 }])).toContain('1,200')

    const trendOption = echartsMock.chart.setOption.mock.calls[1]?.[0] as {
      xAxis: { data: string[] }
      series: Array<{ name: string; data: number[] }>
      tooltip: { formatter: (params: Array<{ axisValue: string; seriesName: string; data: number }>) => string }
    }
    expect(trendOption.xAxis.data).toEqual(['2026-06-03', '2026-06-04'])
    expect(trendOption.series).toMatchObject([
      { name: 'Checks', data: [2, 4] },
      { name: 'Consumes', data: [1, 2] },
      { name: 'Blocked', data: [0, 1] },
    ])
    expect(trendOption.tooltip.formatter([{ axisValue: '2026-06-04', seriesName: 'Blocked', data: 1200 }])).toContain('1,200')

    const resultPieOption = echartsMock.chart.setOption.mock.calls[2]?.[0] as {
      color: string[]
      series: Array<{ name: string; type: string; label: { show: boolean; formatter: string }; data: Array<{ name: string; value: number }> }>
      tooltip: { formatter: (params: { name: string; value: number }) => string }
    }
    expect(resultPieOption.color).toEqual(['#34d399', '#fb7185'])
    expect(resultPieOption.series).toMatchObject([
      {
        name: 'Result mix',
        type: 'pie',
        label: { show: true, formatter: '{c}' },
        data: [
          { name: 'Allowed', value: 9 },
          { name: 'Blocked', value: 1 },
        ],
      },
    ])
    expect(resultPieOption.tooltip.formatter({ name: 'Allowed', value: 1200 })).toContain('1,200')
  })

  it('hides default-filtered fingerprint rows by default and keeps bucket summary visible', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/overview') {
        return ok({ total_api_keys: 0, active_api_keys: 0, disabled_api_keys: 0, expired_api_keys: 0, free_machines: 0, checks_last_24h: 0, consumes_last_24h: 0, blocked_last_24h: 0, total_users: 0, paid_orders_last_24h: 0, paid_quota_added_last_24h: 0, remaining_paid_quota: 0, usage_trend: [], mode_breakdown: [], result_breakdown: [], api_key_status_breakdown: [] })
      }
      if (url === '/api/admin/operations/funnel?range=30d') {
        return ok({ activated_users: 0, paid_users: 0, activation_rate: 0, paid_conversion_rate: 0, machine_quality: { total_machines: 0 } })
      }
      if (url === '/api/admin/fingerprint-quality') {
        return ok({
          summary: [
            { bucket: 'internal_inspection', reason: 'production inspection agent', fingerprints: 1, events: 1, default_filtered: true },
            { bucket: 'candidate_real_or_unknown', reason: 'no machine/test signals matched', fingerprints: 1, events: 2, default_filtered: false },
          ],
          rows: [
            baseRow({ bucket: 'internal_inspection', reason: 'production inspection agent', fingerprint_hash: 'inspection-fingerprint', fingerprint_prefix: 'inspection', events: 1 }),
            baseRow({ bucket: 'candidate_real_or_unknown', reason: 'no machine/test signals matched', fingerprint_hash: 'candidate-fingerprint', fingerprint_prefix: 'candidate', events: 2 }),
          ],
        })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    const checkbox = await screen.findByRole('checkbox', { name: /hide default-filtered buckets/i })
    expect(checkbox).toBeChecked()
    expect(await screen.findByText('candidate-fingerprint')).toBeInTheDocument()
    expect(screen.queryByText('inspection-fingerprint')).not.toBeInTheDocument()
    expect(within(screen.getByTestId('fingerprint-quality-summary')).getByText('internal_inspection')).toBeInTheDocument()

    fireEvent.click(checkbox)

    expect(await screen.findByText('inspection-fingerprint')).toBeInTheDocument()
  })
})

function ok(data: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => ({ data }),
  }
}

function baseRow(overrides: Record<string, unknown>) {
  return {
    bucket: '',
    reason: '',
    fingerprint_hash: '',
    fingerprint_prefix: '',
    first_at: '2026-05-20T00:00:00Z',
    last_at: '2026-05-20T00:00:00Z',
    events: 0,
    generate_events: 0,
    status_events: 0,
    blocked_events: 0,
    user_bound_events: 0,
    ip_count: 0,
    ips: [],
    cli_versions: [],
    runtime_modes: [],
    document_types: [],
    user_agents: [],
    ...overrides,
  }
}
