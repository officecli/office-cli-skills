import type { FingerprintQualityRow } from './types'

export const FINGERPRINT_QUALITY_CSV_HEADERS = [
  'bucket',
  'reason',
  'fingerprint_hash',
  'fingerprint_prefix',
  'first_at',
  'last_at',
  'events',
  'generate_events',
  'status_events',
  'blocked_events',
  'user_bound_events',
  'ip_count',
  'ips',
  'cli_versions',
  'runtime_modes',
  'document_types',
  'user_agents',
] as const

export function buildFingerprintQualityCsv(rows: FingerprintQualityRow[]) {
  return [
    FINGERPRINT_QUALITY_CSV_HEADERS.join(','),
    ...rows.map((row) => FINGERPRINT_QUALITY_CSV_HEADERS.map((header) => escapeCsv(csvValue(row, header))).join(',')),
  ].join('\n')
}

function csvValue(row: FingerprintQualityRow, header: typeof FINGERPRINT_QUALITY_CSV_HEADERS[number]) {
  const value = row[header]
  if (Array.isArray(value)) {
    return [...value].sort().join('; ')
  }
  return String(value ?? '')
}

function escapeCsv(value: string) {
  if (/[",\n\r]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`
  }
  return value
}
