import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ImageTemplatesPage from './ImageTemplatesPage'

const fetchMock = vi.fn()

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
    vi.unstubAllGlobals()
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
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(fileInput, { target: { files: [new File(['png'], 'thumb.png', { type: 'image/png' })] } })

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/admin/image-templates/7/thumbnail', expect.objectContaining({ method: 'POST' })))
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
  })
})
