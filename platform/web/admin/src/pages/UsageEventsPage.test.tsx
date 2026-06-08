import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import UsageEventsPage, { buildUsageEventParams } from './UsageEventsPage'

const fetchMock = vi.fn()

function renderWithUrl(initialUrl: string) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <UsageEventsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function mockEmpty() {
  fetchMock.mockImplementation(async () => ({
    ok: true,
    status: 200,
    json: async () => ({ data: [] }),
  }))
  vi.stubGlobal('fetch', fetchMock)
}

function usageEventURLs() {
  return fetchMock.mock.calls
    .map((call) => String(call[0]))
    .filter((url) => url.startsWith('/api/admin/usage-events'))
}

function paramsFromUsageURL(url: string) {
  return new URL(url, 'https://admin.test').searchParams
}

describe('admin usage events page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('pre-populates the user_id filter from the URL search params', async () => {
    mockEmpty()
    renderWithUrl('/usage-events?user_id=42')

    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((call) => String(call[0]))
      expect(urls.some((url) => url.startsWith('/api/admin/usage-events') && url.includes('user_id=42'))).toBe(true)
    })
  })

  it('queries without user_id when the URL has no user_id param', async () => {
    mockEmpty()
    renderWithUrl('/usage-events')

    await waitFor(() => {
      const usageCalls = usageEventURLs()
      expect(usageCalls.length).toBeGreaterThan(0)
      expect(usageCalls.every((url) => !url.includes('user_id='))).toBe(true)
    })
  })

  it('queries default multi-select filters with generate action only', async () => {
    mockEmpty()
    renderWithUrl('/usage-events')

    await waitFor(() => {
      const usageCalls = usageEventURLs()
      expect(usageCalls.length).toBeGreaterThan(0)
      const params = paramsFromUsageURL(usageCalls[0])
      expect(params.getAll('mode')).toEqual(['free', 'reward', 'paid', 'hosted'])
      expect(params.getAll('result')).toEqual(['allowed', 'blocked'])
      expect(params.getAll('action')).toEqual(['generate'])
      expect(params.getAll('action')).not.toContain('check')
      expect(params.getAll('action')).not.toContain('status')
    })
  })

  it('moves audit identity columns to the end even when saved preferences put them first', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/preferences/usage-events-table') {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: {
              version: 1,
              columns: [
                { key: 'fingerprint_hash', visible: true, fixed: 'left', width: 220 },
                { key: 'request_id', visible: true, fixed: 'left', width: 180 },
                { key: 'client_ip', visible: true, fixed: 'left', width: 150 },
                { key: 'request', visible: true, fixed: 'left', width: 260 },
                { key: 'created_at', visible: true, width: 180 },
                { key: 'mode_result', visible: true, width: 160 },
                { key: 'action_reason', visible: true, width: 170 },
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
              id: 202,
              request_id: 'req-order-202',
              fingerprint_hash: 'fp-order-machine',
              mode: 'hosted',
              action: 'generate',
              result: 'allowed',
              client_ip: '203.0.113.10',
              request_method: 'POST',
              request_host: 'platform.officecli.io',
              request_path: '/api/license/check',
              created_at: '2026-05-15T08:00:00Z',
            }],
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithUrl('/usage-events')

    await screen.findByText(/fp-order-machine/i)
    const headers = Array.from(document.querySelectorAll('th'))
      .map((header) => header.textContent?.replace(/\s+/g, ' ').trim() ?? '')
      .filter(Boolean)

    expect(headers.slice(-4)).toEqual(['Client IP', 'Request', 'Fingerprint', 'Request ID'])
  })

  it('renders start and end time as date-time picker controls', async () => {
    mockEmpty()
    renderWithUrl('/usage-events')

    expect(screen.getByPlaceholderText('Select start time')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Select end time')).toBeInTheDocument()
  })

  it('shows loading feedback instead of the empty state while usage events are loading', () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/preferences/usage-events-table') {
        return { ok: true, status: 200, json: async () => ({ data: null }) }
      }
      if (url.startsWith('/api/admin/usage-events?')) {
        return new Promise(() => undefined)
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithUrl('/usage-events')

    expect(screen.getByText(/loading usage events/i)).toBeInTheDocument()
    expect(screen.queryByText(/no usage events matched/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/adjust the filter set/i)).not.toBeInTheDocument()
  })

  it('shows total token usage with prompt, completion, and reasoning details', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/preferences/usage-events-table') {
        return { ok: true, status: 200, json: async () => ({ data: null }) }
      }
      if (url.startsWith('/api/admin/usage-events?')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [{
              id: 101,
              request_id: 'req-token-101',
              fingerprint_hash: 'fp-token-machine',
              mode: 'hosted',
              action: 'generate',
              result: 'allowed',
              model_name: 'gpt-4.1',
              prompt_tokens: 123,
              completion_tokens: 45,
              reasoning_tokens: 6,
              created_at: '2026-05-15T08:00:00Z',
            }],
          }),
        }
      }
      return { ok: true, status: 200, json: async () => ({ data: [] }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithUrl('/usage-events')

    expect(await screen.findByText(/174 total tokens/i)).toBeInTheDocument()
    expect(screen.getByText(/prompt 123 \/ completion 45 \/ reasoning 6/i)).toBeInTheDocument()
  })

  it('serializes multi-select filter changes as repeated query params', () => {
    const params = buildUsageEventParams({
      mode: ['free', 'paid'],
      result: ['allowed'],
      action: ['check', 'generate', 'status'],
      reason_code: 'quota_ok',
      fingerprint_hash: '',
      api_key_id: '',
      user_id: '42',
      client_ip: '',
      request_id: '',
      start_time: '',
      end_time: '',
    })

    expect(params.getAll('mode')).toEqual(['free', 'paid'])
    expect(params.getAll('result')).toEqual(['allowed'])
    expect(params.getAll('action')).toEqual(['check', 'generate', 'status'])
    expect(params.get('reason_code')).toBe('quota_ok')
    expect(params.get('user_id')).toBe('42')
  })
})
