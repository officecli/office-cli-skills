import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildOAuth2LoginURL, ensureVisitorID, normalizeAppReturnTo, trackEvent } from './analytics'

describe('app analytics helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
    window.history.replaceState({}, '', '/app')
    document.cookie = 'ocli_visitor_id=; Max-Age=0; Path=/'
    document.head.innerHTML = ''
    delete window.gtag
    delete window.dataLayer
    vi.unstubAllGlobals()
  })

  it('propagates invite and utm params into oauth2 login redirects', () => {
    window.history.replaceState({}, '', '/app/login?invite=invite-xyz&utm_source=pricing')

    const loginURL = buildOAuth2LoginURL('/app')
    const url = new URL(loginURL, 'https://platform.officecli.io')

    expect(url.pathname).toBe('/api/auth/oauth2/login')
    expect(url.searchParams.get('return_to')).toBe('/app')
    expect(url.searchParams.get('invite')).toBe('invite-xyz')
    expect(url.searchParams.get('visitor_id')).toBe(ensureVisitorID())
    expect(url.searchParams.get('utm_source')).toBe('pricing')
  })

	it('posts first-party events with keepalive even when ga4 is not configured', () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    window.history.replaceState({}, '', '/app/billing?invite=invite-xyz')

    trackEvent('checkout_start', { surface: 'app', pack_code: 'hosted-300' })

    const payload = JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))
    expect(payload.event_name).toBe('checkout_start')
    expect(payload.surface).toBe('app')
    expect(payload.visitor_id).toBe(ensureVisitorID())
    expect(payload.invite).toBe('invite-xyz')
    expect(payload.metadata.pack_code).toBe('hosted-300')
    expect(fetchMock).toHaveBeenCalledWith('/api/events/track', expect.objectContaining({ method: 'POST', keepalive: true, credentials: 'include' }))
		expect(window.dataLayer).toBeUndefined()
	})

	it('initializes visitor cookie from a valid URL visitor_id and reuses it for events and oauth login', () => {
		const fetchMock = vi.fn().mockResolvedValue({ ok: true })
		vi.stubGlobal('fetch', fetchMock)
		window.history.replaceState({}, '', '/app/login?visitor_id=url-visitor-123&utm_source=site')

		trackEvent('login_start', { surface: 'app', placement: 'login-page' })
		const loginURL = buildOAuth2LoginURL('/app')
		const url = new URL(loginURL, 'https://platform.officecli.io')
		const payload = JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))

		expect(ensureVisitorID()).toBe('url-visitor-123')
		expect(document.cookie).toContain('ocli_visitor_id=url-visitor-123')
		expect(payload.visitor_id).toBe('url-visitor-123')
		expect(url.searchParams.get('visitor_id')).toBe('url-visitor-123')
		expect(url.searchParams.get('utm_source')).toBe('site')
	})

	it('keeps an existing visitor cookie ahead of raw URL visitor_id for events and oauth login', () => {
		const fetchMock = vi.fn().mockResolvedValue({ ok: true })
		vi.stubGlobal('fetch', fetchMock)
		document.cookie = 'ocli_visitor_id=cookie-visitor; Path=/'
		window.history.replaceState({}, '', '/app/login?visitor_id=url-visitor&utm_source=site')

		trackEvent('login_start', { surface: 'app', visitor_id: 'url-visitor', placement: 'login-page' })
		const loginURL = buildOAuth2LoginURL('/app')
		const url = new URL(loginURL, 'https://platform.officecli.io')
		const payload = JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))

		expect(ensureVisitorID()).toBe('cookie-visitor')
		expect(payload.visitor_id).toBe('cookie-visitor')
		expect(url.searchParams.get('visitor_id')).toBe('cookie-visitor')
		expect(url.searchParams.get('utm_source')).toBe('site')
	})

  it('normalizes app-internal return targets under /app before redirecting to oauth2 login', () => {
    const loginURL = buildOAuth2LoginURL('/billing?status=success&session_id=cs_test_123')
    const url = new URL(loginURL, 'https://platform.officecli.io')

    expect(url.searchParams.get('return_to')).toBe('/app/billing?status=success&session_id=cs_test_123')
  })

  it('keeps absolute preview return targets unchanged', () => {
    expect(normalizeAppReturnTo('https://officecli.io/p/share-token')).toBe('https://officecli.io/p/share-token')
  })

  it('initializes gtag using arguments objects so ga4 can consume the queue', async () => {
    vi.stubEnv('VITE_GA4_MEASUREMENT_ID', 'G-TESTAPP123')

    const { initAnalytics } = await import('./analytics')
    initAnalytics()

    const dataLayer = window.dataLayer as IArguments[]

    expect(dataLayer).toHaveLength(2)
    expect(Array.isArray(dataLayer[0])).toBe(false)
    expect(dataLayer[0][0]).toBe('js')
    expect(dataLayer[1][0]).toBe('config')
    expect(dataLayer[1][1]).toBe('G-TESTAPP123')
    expect(document.getElementById('ga4-script')?.getAttribute('src')).toContain('G-TESTAPP123')
  })
})
