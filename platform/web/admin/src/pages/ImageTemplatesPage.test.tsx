import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ImageTemplatesPage from './ImageTemplatesPage'

const fetchMock = vi.fn()
const messageApi = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
}))

vi.mock('antd', async (importOriginal) => {
  const actual = await importOriginal<typeof import('antd')>()
  return {
    ...actual,
    App: {
      ...actual.App,
      useApp: () => ({
        message: messageApi,
      }),
    },
  }
})

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ImageTemplatesPage />
    </QueryClientProvider>,
  )
}

describe('admin image templates page', () => {
  afterEach(() => {
    fetchMock.mockReset()
    messageApi.success.mockReset()
    messageApi.error.mockReset()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('lists templates and creates a template', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/image-templates' && (!init || init.method === undefined)) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              { id: 7, slug: 'poster', title: 'Poster', description: 'Poster style', prompt_preset: 'cinematic preset', thumbnail_url: '/api/image-templates/7/thumbnail', sort_order: 10, enabled: true },
            ],
          }),
        }
      }
      if (url === '/api/admin/image-templates/publish-requests?status=pending' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/admin/image-templates' && init?.method === 'POST') {
        expect(JSON.parse(String(init.body))).toMatchObject({ slug: 'liblib-poster', title: 'Liblib Poster', prompt_preset: 'preset prompt', enabled: true })
        return { ok: true, status: 200, json: async () => ({ data: { id: 8, slug: 'liblib-poster', title: 'Liblib Poster', description: '', prompt_preset: 'preset prompt', sort_order: 0, enabled: true } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByDisplayValue('Poster')).toBeInTheDocument()
    expect(screen.getByDisplayValue('cinematic preset')).toBeInTheDocument()

    fireEvent.change(screen.getAllByLabelText('Slug')[0], { target: { value: 'liblib-poster' } })
    fireEvent.change(screen.getAllByLabelText('Title')[0], { target: { value: 'Liblib Poster' } })
    fireEvent.change(screen.getAllByLabelText('Prompt preset')[0], { target: { value: 'preset prompt' } })
    fireEvent.click(screen.getByRole('button', { name: /create template/i }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/admin/image-templates', expect.objectContaining({ method: 'POST' })))
    await waitFor(() => expect(messageApi.success).toHaveBeenCalledWith('Template created.'))
  })

  it('uploads a thumbnail with multipart form data', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/image-templates' && (!init || init.method === undefined)) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              { id: 7, slug: 'poster', title: 'Poster', description: 'Poster style', prompt_preset: 'cinematic preset', sort_order: 10, enabled: true },
            ],
          }),
        }
      }
      if (url === '/api/admin/image-templates/publish-requests?status=pending' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/admin/image-templates/7/thumbnail' && init?.method === 'POST') {
        expect(init.body).toBeInstanceOf(FormData)
        expect((init.headers as Record<string, string> | undefined)?.['Content-Type']).toBeUndefined()
        return { ok: true, status: 200, json: async () => ({ data: { id: 7, slug: 'poster', title: 'Poster', description: 'Poster style', prompt_preset: 'cinematic preset', thumbnail_url: '/api/image-templates/7/thumbnail', sort_order: 10, enabled: true } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    await screen.findByDisplayValue('Poster')
    const fileInput = document.querySelector('.image-template-thumbnail-input') as HTMLInputElement
    fireEvent.change(fileInput, { target: { files: [new File(['png'], 'thumb.png', { type: 'image/png' })] } })

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/admin/image-templates/7/thumbnail', expect.objectContaining({ method: 'POST' })))
    await waitFor(() => expect(messageApi.success).toHaveBeenCalledWith('Thumbnail uploaded.'))
  })

  it('exports configured templates as importable JSON', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/image-templates' && (!init || init.method === undefined)) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              {
                id: 7,
                slug: 'poster',
                title: 'Poster',
                description: 'Poster style',
                prompt_preset: 'cinematic {{product}}',
                sort_order: 10,
                enabled: true,
                slots: [{ key: 'product', label: 'Product', default_value: 'bicycle', required: true }],
              },
            ],
          }),
        }
      }
      if (url === '/api/admin/image-templates/publish-requests?status=pending' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    if (!URL.createObjectURL) URL.createObjectURL = vi.fn(() => 'blob:templates')
    if (!URL.revokeObjectURL) URL.revokeObjectURL = vi.fn()
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:templates')
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)

    renderPage()

    await screen.findByDisplayValue('Poster')
    fireEvent.click(screen.getByRole('button', { name: /export json/i }))

    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob))
    expect(click).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:templates')
    expect(messageApi.success).toHaveBeenCalledWith('Exported 1 image templates.')
  })

  it('copies configured templates JSON to the clipboard', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/image-templates' && (!init || init.method === undefined)) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              {
                id: 7,
                slug: 'poster',
                title: 'Poster',
                description: 'Poster style',
                prompt_preset: 'cinematic {{product}}',
                sort_order: 10,
                enabled: true,
              },
            ],
          }),
        }
      }
      if (url === '/api/admin/image-templates/publish-requests?status=pending' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    const writeText = vi.fn(async (_text: string) => undefined)
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })

    renderPage()

    await screen.findByDisplayValue('Poster')
    fireEvent.click(screen.getByRole('button', { name: /copy json/i }))

    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1))
    expect(JSON.parse(writeText.mock.calls[0][0])).toMatchObject({
      version: 1,
      templates: [{ slug: 'poster', title: 'Poster', prompt_preset: 'cinematic {{product}}' }],
    })
    expect(messageApi.success).toHaveBeenCalledWith('Copied 1 image templates.')
  })

  it('imports image template JSON and creates each template', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/image-templates' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/admin/image-templates/publish-requests?status=pending' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/admin/image-templates' && init?.method === 'POST') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 9, ...JSON.parse(String(init.body)) } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    await screen.findByText(/No image templates/i)
    const file = new File([
      JSON.stringify({
        version: 1,
        templates: [
          {
            slug: 'admission',
            title: 'Admission',
            description: 'Admission letter',
            promptPreset: 'Hold {{university_name}} letter',
            sortOrder: 12,
            enabled: true,
            slots: [{ key: 'university_name', label: 'University name', defaultValue: 'Cambridge', required: true }],
          },
        ],
      }),
    ], 'templates.json', { type: 'application/json' })
    const input = document.querySelector('.image-template-json-input') as HTMLInputElement
    fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() => {
      const createCalls = fetchMock.mock.calls.filter((call) => call[0] === '/api/admin/image-templates' && call[1]?.method === 'POST')
      expect(createCalls).toHaveLength(1)
      expect(JSON.parse(String(createCalls[0][1]?.body))).toMatchObject({
        slug: 'admission',
        title: 'Admission',
        prompt_preset: 'Hold {{university_name}} letter',
        sort_order: 12,
        slots: [{ key: 'university_name', label: 'University name', default_value: 'Cambridge', required: true }],
      })
    })
    await waitFor(() => expect(messageApi.success).toHaveBeenCalledWith('Imported 1 image templates.'))
  })

  it('imports pasted image template JSON and closes the paste dialog', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/image-templates' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/admin/image-templates/publish-requests?status=pending' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/admin/image-templates' && init?.method === 'POST') {
        return { ok: true, status: 200, json: async () => ({ data: { id: 9, ...JSON.parse(String(init.body)) } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    await screen.findByText(/No image templates/i)
    fireEvent.click(screen.getByRole('button', { name: /paste json/i }))
    fireEvent.change(await screen.findByPlaceholderText(/paste image-template json here/i), {
      target: {
        value: JSON.stringify({
          templates: [
            {
              title: 'Pasted Poster',
              promptPreset: 'Pasted prompt',
              enabled: true,
            },
          ],
        }),
      },
    })
    fireEvent.click(screen.getByRole('button', { name: /^import$/i }))

    await waitFor(() => {
      const createCalls = fetchMock.mock.calls.filter((call) => call[0] === '/api/admin/image-templates' && call[1]?.method === 'POST')
      expect(createCalls).toHaveLength(1)
      expect(JSON.parse(String(createCalls[0][1]?.body))).toMatchObject({
        slug: 'pasted-poster',
        title: 'Pasted Poster',
        prompt_preset: 'Pasted prompt',
      })
    })
    await waitFor(() => expect(messageApi.success).toHaveBeenCalledWith('Imported 1 image templates.'))
    expect(screen.queryByPlaceholderText(/paste image-template json here/i)).not.toBeInTheDocument()
  })

  it('reviews a pending publish request', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/admin/image-templates' && (!init || init.method === undefined)) {
        return { ok: true, status: 200, json: async () => ({ data: [] }) }
      }
      if (url === '/api/admin/image-templates/publish-requests?status=pending' && (!init || init.method === undefined)) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            data: [
              { id: 3, private_template_id: 9, requester_user_id: 42, provenance_id: 11, status: 'pending', submitter_note: 'Launch poster' },
            ],
          }),
        }
      }
      if (url === '/api/admin/image-templates/publish-requests/3/review' && init?.method === 'POST') {
        expect(JSON.parse(String(init.body))).toMatchObject({ action: 'approve' })
        return { ok: true, status: 200, json: async () => ({ data: { id: 3, private_template_id: 9, requester_user_id: 42, provenance_id: 11, status: 'approved', public_template_id: 10 } }) }
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()

    expect(await screen.findByText(/Request #3/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /approve/i }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/admin/image-templates/publish-requests/3/review', expect.objectContaining({ method: 'POST' })))
    await waitFor(() => expect(messageApi.success).toHaveBeenCalledWith('Publish request approved.'))
  })
})
