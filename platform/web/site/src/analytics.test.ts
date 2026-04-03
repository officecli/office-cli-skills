import { describe, expect, it } from 'vitest'
import { buildTrackedURL } from './analytics'

describe('site analytics helpers', () => {
  it('propagates invite and utm params into platform links', () => {
    const target = buildTrackedURL('https://platform.officecli.io/app', '?invite=invite-xyz&utm_source=pricing&utm_campaign=q2')
    const url = new URL(target)

    expect(url.searchParams.get('invite')).toBe('invite-xyz')
    expect(url.searchParams.get('utm_source')).toBe('pricing')
    expect(url.searchParams.get('utm_campaign')).toBe('q2')
  })
})
