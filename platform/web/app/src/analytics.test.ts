import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildOAuth2LoginURL, normalizeAppReturnTo } from './analytics'

describe('app analytics helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
    window.history.replaceState({}, '', '/app')
    document.head.innerHTML = ''
    delete window.gtag
    delete window.dataLayer
  })

  it('propagates invite and utm params into oauth2 login redirects', () => {
    window.history.replaceState({}, '', '/app/login?invite=invite-xyz&utm_source=pricing')

    const loginURL = buildOAuth2LoginURL('/app')
    const url = new URL(loginURL, 'https://platform.officecli.io')

    expect(url.pathname).toBe('/api/auth/oauth2/login')
    expect(url.searchParams.get('return_to')).toBe('/app')
    expect(url.searchParams.get('invite')).toBe('invite-xyz')
    expect(url.searchParams.get('utm_source')).toBe('pricing')
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
