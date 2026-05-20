import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildTrackedURL, ensureVisitorID, trackEvent } from './analytics'

describe('site analytics helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
    window.history.replaceState({}, '', '/')
    document.head.innerHTML = ''
    document.cookie = 'ocli_visitor_id=; Max-Age=0; Path=/'
    delete window.gtag
    delete window.dataLayer
    vi.unstubAllGlobals()
  })

  it('propagates invite and utm params into platform links', () => {
    const target = buildTrackedURL('https://platform.officecli.io/app', '?invite=invite-xyz&utm_source=pricing&utm_campaign=q2')
    const url = new URL(target)

    expect(url.searchParams.get('invite')).toBe('invite-xyz')
    expect(url.searchParams.get('visitor_id')).toBe(ensureVisitorID())
    expect(url.searchParams.get('utm_source')).toBe('pricing')
    expect(url.searchParams.get('utm_campaign')).toBe('q2')
  })

  it('uses the same visitor id for carried links and first-party event payloads', () => {
    vi.stubEnv('VITE_GA4_MEASUREMENT_ID', '')
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    const target = buildTrackedURL('/app', '?utm_source=hero')
    const url = new URL(target, window.location.origin)

    trackEvent('download_click', { surface: 'site', placement: 'hero' })

    const payload = JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))
    expect(payload.visitor_id).toBe(url.searchParams.get('visitor_id'))
    expect(fetchMock).toHaveBeenCalledWith('/api/events/track', expect.objectContaining({ method: 'POST', keepalive: true, credentials: 'include' }))
    expect(window.dataLayer).toBeUndefined()
  })

  it('initializes visitor cookie from a valid URL visitor_id and reuses it for links and event payloads', () => {
    vi.stubEnv('VITE_GA4_MEASUREMENT_ID', '')
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    window.history.replaceState({}, '', '/?visitor_id=url-site-visitor-123&utm_source=hero')

    const target = buildTrackedURL('/app', window.location.search)
    const url = new URL(target, window.location.origin)
    trackEvent('download_click', { surface: 'site', placement: 'hero' })

    const payloads = fetchMock.mock.calls.map((call) => JSON.parse(String((call[1] as RequestInit).body)))
    const downloadPayload = payloads.find((payload) => payload.event_name === 'download_click')

    expect(document.cookie).toContain('ocli_visitor_id=url-site-visitor-123')
    expect(url.searchParams.get('visitor_id')).toBe('url-site-visitor-123')
    expect(downloadPayload?.visitor_id).toBe('url-site-visitor-123')
    expect(downloadPayload?.utm_source).toBe('hero')
  })

  it('initializes gtag using arguments objects so ga4 can consume the queue', async () => {
    vi.stubEnv('VITE_GA4_MEASUREMENT_ID', 'G-TESTSITE123')

    const { initAnalytics } = await import('./analytics')
    initAnalytics()

    const dataLayer = window.dataLayer as IArguments[]

    expect(dataLayer).toHaveLength(2)
    expect(Array.isArray(dataLayer[0])).toBe(false)
    expect(dataLayer[0][0]).toBe('js')
    expect(dataLayer[1][0]).toBe('config')
    expect(dataLayer[1][1]).toBe('G-TESTSITE123')
    expect(document.getElementById('ga4-script')?.getAttribute('src')).toContain('G-TESTSITE123')
  })
})
