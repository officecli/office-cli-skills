import { FormEvent, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'
import type { UsageEvent } from '../types'

interface FilterState {
  mode: string
  result: string
  reason_code: string
  fingerprint_hash: string
  api_key_id: string
  user_id: string
  client_ip: string
  request_id: string
  start_time: string
  end_time: string
}

const defaultFilters: FilterState = {
  mode: '',
  result: '',
  reason_code: '',
  fingerprint_hash: '',
  api_key_id: '',
  user_id: '',
  client_ip: '',
  request_id: '',
  start_time: '',
  end_time: '',
}

export default function UsageEventsPage() {
  const [draft, setDraft] = useState<FilterState>(defaultFilters)
  const [filters, setFilters] = useState<FilterState>(defaultFilters)

  const { data = [] } = useQuery({
    queryKey: ['admin-usage-events', filters],
    queryFn: () => {
      const params = new URLSearchParams()
      if (filters.mode) params.set('mode', filters.mode)
      if (filters.result) params.set('result', filters.result)
      if (filters.reason_code) params.set('reason_code', filters.reason_code)
      if (filters.fingerprint_hash) params.set('fingerprint_hash', filters.fingerprint_hash)
      if (filters.api_key_id) params.set('api_key_id', filters.api_key_id)
      if (filters.user_id) params.set('user_id', filters.user_id)
      if (filters.client_ip) params.set('client_ip', filters.client_ip)
      if (filters.request_id) params.set('request_id', filters.request_id)
      if (filters.start_time) params.set('start_time', filters.start_time)
      if (filters.end_time) params.set('end_time', filters.end_time)
      return api.usageEvents(params)
    },
  })

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading eyebrow="Audit visibility" title="Recent usage events" body="Filter the event stream to understand why requests were allowed, blocked, or routed through a specific mode." />
        <form className="surface-console soft-panel mb-6 grid gap-4 border border-outline-variant/20 p-5 md:grid-cols-4" onSubmit={(event: FormEvent) => {
          event.preventDefault()
          setFilters(draft)
        }}>
          <label className="text-sm text-outline">Mode
            <select className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.mode} onChange={(event) => setDraft((current) => ({ ...current, mode: event.target.value }))}>
              <option value="">All modes</option>
              <option value="free">free</option>
              <option value="reward">reward</option>
              <option value="paid">paid</option>
              <option value="hosted">hosted</option>
            </select>
          </label>
          <label className="text-sm text-outline">Result
            <select className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.result} onChange={(event) => setDraft((current) => ({ ...current, result: event.target.value }))}>
              <option value="">All results</option>
              <option value="allowed">allowed</option>
              <option value="blocked">blocked</option>
            </select>
          </label>
          <label className="text-sm text-outline">Reason code
            <input className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.reason_code} onChange={(event) => setDraft((current) => ({ ...current, reason_code: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Fingerprint
            <input className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.fingerprint_hash} onChange={(event) => setDraft((current) => ({ ...current, fingerprint_hash: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">API key ID
            <input className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.api_key_id} onChange={(event) => setDraft((current) => ({ ...current, api_key_id: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">User ID
            <input className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.user_id} onChange={(event) => setDraft((current) => ({ ...current, user_id: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Client IP
            <input className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.client_ip} onChange={(event) => setDraft((current) => ({ ...current, client_ip: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Request ID
            <input className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.request_id} onChange={(event) => setDraft((current) => ({ ...current, request_id: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Start time
            <input className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" placeholder="2026-05-15T00:00:00Z" value={draft.start_time} onChange={(event) => setDraft((current) => ({ ...current, start_time: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">End time
            <input className="surface-console-muted mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" placeholder="2026-05-16T00:00:00Z" value={draft.end_time} onChange={(event) => setDraft((current) => ({ ...current, end_time: event.target.value }))} />
          </label>
          <div className="md:col-span-4 flex gap-3">
            <button type="submit" className="tonal-button">Apply filters</button>
            <button type="button" className="ghost-button" onClick={() => {
              setDraft(defaultFilters)
              setFilters(defaultFilters)
            }}>Reset</button>
          </div>
        </form>
        {data.length ? (
          <div className="space-y-4">
            {data.map((event) => <UsageAuditEvent key={event.id} event={event} />)}
          </div>
        ) : (
          <EmptyState title="No usage events matched" body="Adjust the filter set or wait for fresh policy traffic to enter the audit stream." />
        )}
      </Panel>
    </div>
  )
}

function UsageAuditEvent({ event }: { event: UsageEvent }) {
  return (
    <article className="soft-panel overflow-hidden border border-outline-variant/15">
      <div className="grid gap-4 bg-surface-container-high/60 p-5 text-sm lg:grid-cols-[1fr_1fr_1.3fr_1fr_1fr]">
        <div>
          <div className="info-eyebrow-tight text-outline">Mode / Result</div>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className="font-semibold text-white">{event.mode}</span>
            <StatusPill value={event.result} />
          </div>
          <div className="mt-2 text-outline">{event.action}{event.reason_code ? ` / ${event.reason_code}` : ''}</div>
        </div>
        <AuditBlock label="User / Key" value={`user ${valueOrDash(event.user_id)} / key ${valueOrDash(event.api_key_id)}`} />
        <AuditBlock label="Machine / IP" value={`${event.fingerprint_hash} / ${valueOrDash(event.client_ip)}`} mono />
        <AuditBlock label="Model / Charge" value={`${event.model_name || event.provider || '--'} / ${chargeLabel(event)}`} />
        <AuditBlock label="Timestamp" value={formatDate(event.created_at)} />
      </div>
      <div className="grid gap-4 p-5 text-sm md:grid-cols-2 xl:grid-cols-3">
        <AuditBlock label="Request" value={`${valueOrDash(event.request_method)} ${valueOrDash(event.request_host)}${valueOrDash(event.request_path)}`} mono />
        <AuditBlock label="Request ID" value={valueOrDash(event.request_id)} mono />
        <AuditBlock label="Machine fingerprint" value={event.fingerprint_hash} mono />
        <AuditBlock label="Client IP" value={valueOrDash(event.client_ip)} mono />
        <AuditBlock label="Forwarded-For" value={valueOrDash(event.forwarded_for)} mono />
        <AuditBlock label="User-Agent" value={valueOrDash(event.user_agent)} mono />
        <AuditBlock label="CLI / Document" value={`${valueOrDash(event.cli_version)} / ${valueOrDash(event.document_type)}`} />
        <AuditBlock label="Runtime / Provider" value={`${valueOrDash(event.runtime_mode)} / ${valueOrDash(event.provider)}`} />
        <AuditBlock label="Tokens" value={`${event.prompt_tokens ?? 0} / ${event.completion_tokens ?? 0} / ${event.reasoning_tokens ?? 0}`} />
        <AuditBlock label="Images" value={`${event.image_count ?? 0}`} />
        <AuditBlock label="Credits" value={`reserved ${event.reserved_credits ?? 0} / settled ${event.settled_credits ?? 0} / refund ${event.refund_credits ?? 0}`} />
        <AuditBlock label="Cost / Profit" value={`${creditsFromMicrousd(event.upstream_cost_microusd)} / ${creditsFromMicrousd(event.profit_microusd)}${event.cap_applied ? ' capped' : ''}`} />
      </div>
    </article>
  )
}

function AuditBlock({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <div className="info-eyebrow-tight text-outline">{label}</div>
      <div className={`mt-2 break-words text-white ${mono ? 'font-mono text-xs' : ''}`}>{value}</div>
    </div>
  )
}

function chargeLabel(event: UsageEvent) {
  if (event.mode === 'hosted') {
    return `${event.settled_credits ?? 0} credits`
  }
  return `${event.billed_units ?? 0} ${event.unit_type ?? ''}`.trim()
}

function valueOrDash(value?: string | number) {
  if (value === undefined || value === null || value === '') {
    return '--'
  }
  return String(value)
}

function creditsFromMicrousd(value?: number) {
  const credits = Math.ceil(((value ?? 0) * 100) / 1_000_000)
  return `${credits} credits`
}
