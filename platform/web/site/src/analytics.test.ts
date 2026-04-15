import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildTrackedURL } from './analytics'

describe('site analytics helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
    document.head.innerHTML = ''
    delete window.gtag
    delete window.dataLayer
  })

  it('propagates invite and utm params into platform links', () => {
    const target = buildTrackedURL('https://platform.officecli.io/app', '?invite=invite-xyz&utm_source=pricing&utm_campaign=q2')
    const url = new URL(target)

    expect(url.searchParams.get('invite')).toBe('invite-xyz')
    expect(url.searchParams.get('utm_source')).toBe('pricing')
    expect(url.searchParams.get('utm_campaign')).toBe('q2')
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
