import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ApiKeysPage from './ApiKeysPage'

const fetchMock = vi.fn()

function apiKeysResponse(data: unknown[] = []) {
  return {
    ok: true,
    status: 200,
    json: async () => ({ data }),
  }
}

function okResponse(data: unknown = { success: true }) {
  return {
    ok: true,
    status: 200,
    json: async () => ({ data }),
  }
}

function requestBody(call: unknown[]) {
  return JSON.parse(String((call[1] as RequestInit | undefined)?.body ?? '{}'))
}

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

  it('creates hosted-only keys by default with hosted entitlement fields', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/api-keys' && init?.method === 'POST') {
        return okResponse({ plaintext_key: 'cop_hosted_secret', key_prefix: 'cop_hosted' })
      }
      if (url === '/api/admin/users?query=demo%40example.com') {
        return okResponse([
          { id: 42, email: 'demo@example.com', name: 'Demo User', status: 'active', created_at: '2026-04-01T00:00:00Z' },
        ])
      }
      if (url === '/api/admin/api-keys') {
        return apiKeysResponse()
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: /create api key/i }))
    expect(screen.getByLabelText(/key type/i)).toHaveValue('hosted_only')
    expect(screen.queryByLabelText(/^plan name/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/owner user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^user$/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/external quota/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/note/i)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^create key$/i })).toBeDisabled()
    fireEvent.change(screen.getByLabelText(/^user$/i), { target: { value: 'demo@example.com' } })
    fireEvent.click(await screen.findByRole('button', { name: /uid 42/i }))
    fireEvent.change(screen.getByLabelText(/hosted credits/i), { target: { value: '120' } })
    fireEvent.click(screen.getByRole('button', { name: /^create key$/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/api-keys', expect.objectContaining({ method: 'POST' }))
    })
    const postCall = fetchMock.mock.calls.find(([url, init]) => String(url) === '/api/admin/api-keys' && (init as RequestInit | undefined)?.method === 'POST')
    expect(postCall).toBeTruthy()
    expect(requestBody(postCall!)).toMatchObject({
      plan_name: 'Hosted Key',
      allowed_modes: 'hosted_only',
      hosted_enabled: true,
      default_runtime_mode: 'hosted',
      owner_user_id: 42,
      credit_balance: 120,
    })
    expect(requestBody(postCall!)).not.toHaveProperty('quota_total')
  })

  it('does not submit hosted or hybrid creation until a user is selected', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/api-keys') {
        return apiKeysResponse()
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: /create api key/i }))
    expect(screen.getByRole('button', { name: /^create key$/i })).toBeDisabled()
    fireEvent.change(screen.getByLabelText(/key type/i), { target: { value: 'hybrid' } })
    expect(screen.getByRole('button', { name: /^create key$/i })).toBeDisabled()
    fireEvent.change(screen.getByLabelText(/key type/i), { target: { value: 'external_only' } })
    expect(screen.getByRole('button', { name: /^create key$/i })).not.toBeDisabled()
  })

  it('keeps create advanced limited to optional metadata', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/admin/api-keys') {
        return apiKeysResponse()
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: /create api key/i }))
    expect(screen.queryByLabelText(/allowed modes/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/default runtime/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/hosted enabled/i)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^advanced$/i }))
    expect(screen.getByLabelText(/plan name override/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/owner user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/plan code/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/expires at/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/note/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/allowed modes/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/default runtime/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/hosted enabled/i)).not.toBeInTheDocument()
  })

  it('creates external-only keys from the preset with external quota fields', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/api-keys' && init?.method === 'POST') {
        return okResponse({ plaintext_key: 'cop_external_secret', key_prefix: 'cop_external' })
      }
      if (url === '/api/admin/api-keys') {
        return apiKeysResponse()
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: /create api key/i }))
    fireEvent.change(screen.getByLabelText(/key type/i), { target: { value: 'external_only' } })
    expect(screen.queryByLabelText(/hosted credits/i)).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/external quota/i), { target: { value: '50' } })
    fireEvent.click(screen.getByRole('button', { name: /^create key$/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/api-keys', expect.objectContaining({ method: 'POST' }))
    })
    const postCall = fetchMock.mock.calls.find(([url, init]) => String(url) === '/api/admin/api-keys' && (init as RequestInit | undefined)?.method === 'POST')
    expect(requestBody(postCall!)).toMatchObject({
      plan_name: 'External Key',
      allowed_modes: 'external_only',
      hosted_enabled: false,
      default_runtime_mode: 'external',
      quota_total: 50,
    })
    expect(requestBody(postCall!)).not.toHaveProperty('credit_balance')
  })

  it('creates hybrid keys with both external quota and hosted credits', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/api-keys' && init?.method === 'POST') {
        return okResponse({ plaintext_key: 'cop_hybrid_secret', key_prefix: 'cop_hybrid' })
      }
      if (url === '/api/admin/users?query=42') {
        return okResponse([
          { id: 42, email: 'demo@example.com', name: 'Demo User', status: 'active', created_at: '2026-04-01T00:00:00Z' },
        ])
      }
      if (url === '/api/admin/api-keys') {
        return apiKeysResponse()
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: /create api key/i }))
    fireEvent.change(screen.getByLabelText(/key type/i), { target: { value: 'hybrid' } })
    fireEvent.change(screen.getByLabelText(/^user$/i), { target: { value: '42' } })
    fireEvent.click(await screen.findByRole('button', { name: /uid 42/i }))
    fireEvent.change(screen.getByLabelText(/external quota/i), { target: { value: '40' } })
    fireEvent.change(screen.getByLabelText(/hosted credits/i), { target: { value: '80' } })
    fireEvent.click(screen.getByRole('button', { name: /^create key$/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/api-keys', expect.objectContaining({ method: 'POST' }))
    })
    const postCall = fetchMock.mock.calls.find(([url, init]) => String(url) === '/api/admin/api-keys' && (init as RequestInit | undefined)?.method === 'POST')
    expect(requestBody(postCall!)).toMatchObject({
      plan_name: 'Hybrid Key',
      allowed_modes: 'hybrid',
      hosted_enabled: true,
      default_runtime_mode: 'hosted',
      owner_user_id: 42,
      quota_total: 40,
      credit_balance: 80,
    })
  })

  it('saves hosted entitlement changes while editing an existing key', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/api-keys/7' && init?.method === 'PATCH') {
        return okResponse()
      }
      if (url === '/api/admin/api-keys') {
        return apiKeysResponse([
          {
            id: 7,
            key_prefix: 'cop_admin_external',
            plaintext_available: true,
            status: 'active',
            plan_name: 'External',
            allowed_modes: 'external_only',
            hosted_enabled: false,
            default_runtime_mode: 'external',
            quota_used: 0,
            quota_remaining: 20,
            credit_balance: 0,
            created_at: '2026-04-01T00:00:00Z',
          },
        ])
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText(/cop_admin_external/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /edit metadata/i }))
    fireEvent.change(screen.getByLabelText(/edit key type/i), { target: { value: 'hosted_only' } })
    expect(screen.queryByLabelText(/allowed modes/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/default runtime/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/hosted enabled/i)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/admin/api-keys/7', expect.objectContaining({ method: 'PATCH' }))
    })
    const patchCall = fetchMock.mock.calls.find(([url, init]) => String(url) === '/api/admin/api-keys/7' && (init as RequestInit | undefined)?.method === 'PATCH')
    expect(requestBody(patchCall!)).toMatchObject({
      allowed_modes: 'hosted_only',
      hosted_enabled: true,
      default_runtime_mode: 'hosted',
    })
  })
})
