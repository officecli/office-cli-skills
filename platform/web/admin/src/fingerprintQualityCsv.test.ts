import { describe, expect, it } from 'vitest'
import { FINGERPRINT_QUALITY_CSV_HEADERS, buildFingerprintQualityCsv } from './fingerprintQualityCsv'
import type { FingerprintQualityRow } from './types'

describe('fingerprint quality CSV export', () => {
  it('uses the required 17-column header and stable field order', () => {
    const csv = buildFingerprintQualityCsv([row({
      bucket: 'candidate_real_or_unknown',
      reason: 'no machine/test signals matched',
      fingerprint_hash: 'a'.repeat(64),
      fingerprint_prefix: 'a'.repeat(12),
    })])

    const header = csv.split('\n')[0]
    expect(header.split(',')).toEqual(FINGERPRINT_QUALITY_CSV_HEADERS)
    expect(FINGERPRINT_QUALITY_CSV_HEADERS).toHaveLength(17)
    expect(csv.split('\n')[1].startsWith('candidate_real_or_unknown,no machine/test signals matched,')).toBe(true)
  })

  it('keeps bucket and reason and escapes commas quotes and newlines', () => {
    const csv = buildFingerprintQualityCsv([row({
      bucket: 'test_fake_fingerprint',
      reason: 'invalid "hash", empty\nvalue',
      fingerprint_hash: 'bad',
      ips: ['10.0.0.2', '10.0.0.1'],
      cli_versions: ['dev', 'current-1'],
      user_agents: ['agent, one', 'agent "two"'],
    })])

    expect(csv).toContain('test_fake_fingerprint')
    expect(csv).toContain('"invalid ""hash"", empty\nvalue"')
    expect(csv).toContain('10.0.0.1; 10.0.0.2')
    expect(csv).toContain('"agent ""two""; agent, one"')
  })
})

function row(overrides: Partial<FingerprintQualityRow>): FingerprintQualityRow {
  return {
    bucket: '',
    reason: '',
    fingerprint_hash: '',
    fingerprint_prefix: '',
    first_at: '',
    last_at: '',
    events: 0,
    generate_events: 0,
    status_events: 0,
    blocked_events: 0,
    user_bound_events: 0,
    ip_count: 0,
    ips: [],
    cli_versions: [],
    runtime_modes: [],
    document_types: [],
    user_agents: [],
    ...overrides,
  }
}
