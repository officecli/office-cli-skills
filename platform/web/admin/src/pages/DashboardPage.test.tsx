import { fireEvent, render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import DashboardPage from './DashboardPage'

vi.mock('@ant-design/plots', () => ({
  Line: () => <div data-testid="line-chart" />,
  Pie: () => <div data-testid="pie-chart" />,
}))

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
    vi.unstubAllGlobals()
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
