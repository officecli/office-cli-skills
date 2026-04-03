import { afterEach, describe, expect, it } from 'vitest'
import { buildGoogleLoginURL } from './analytics'

describe('app analytics helpers', () => {
  afterEach(() => {
    window.history.replaceState({}, '', '/app')
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
})
