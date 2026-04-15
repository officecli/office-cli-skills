import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildGoogleLoginURL } from './analytics'

describe('app analytics helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
    window.history.replaceState({}, '', '/app')
    document.head.innerHTML = ''
    delete window.gtag
    delete window.dataLayer
  })

  it('propagates invite and utm params into google login redirects', () => {
    window.history.replaceState({}, '', '/app/login?invite=invite-xyz&utm_source=pricing')

    const loginURL = buildGoogleLoginURL('/app')
    const url = new URL(loginURL, 'https://platform.officecli.io')

    expect(url.pathname).toBe('/api/auth/google/login')
    expect(url.searchParams.get('return_to')).toBe('/app')
    expect(url.searchParams.get('invite')).toBe('invite-xyz')
    expect(url.searchParams.get('utm_source')).toBe('pricing')
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
