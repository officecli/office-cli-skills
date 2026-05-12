import { FormEvent, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'

interface FilterState {
  mode: string
  result: string
  reason_code: string
  fingerprint_hash: string
}

const defaultFilters: FilterState = {
  mode: '',
  result: '',
  reason_code: '',
  fingerprint_hash: '',
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
          <div className="md:col-span-4 flex gap-3">
            <button type="submit" className="tonal-button">Apply filters</button>
            <button type="button" className="ghost-button" onClick={() => {
              setDraft(defaultFilters)
              setFilters(defaultFilters)
            }}>Reset</button>
          </div>
        </form>
        {data.length ? (
          <DataTable
            headers={['Mode', 'Action', 'Result', 'Model', 'Charge', 'Cost', 'Profit', 'Timestamp']}
            rows={data.map((event) => [
              <span key="mode" className="text-white">{event.mode}</span>,
              <span key="action" className="text-white">{event.action}</span>,
              <StatusPill key="result" value={event.result} />,
              <span key="model" className="text-outline">{event.model_name || event.reason_code || '--'}</span>,
              <span key="charge" className="text-white">{event.mode === 'hosted' ? `${event.settled_credits ?? 0} credits` : `${event.billed_units ?? 0} ${event.unit_type ?? ''}`}</span>,
              <span key="cost">{event.mode === 'hosted' ? creditsFromMicrousd(event.upstream_cost_microusd) : '--'}</span>,
              <span key="profit" className={event.cap_applied ? 'text-amber-200' : 'text-outline'}>{event.mode === 'hosted' ? `${creditsFromMicrousd(event.profit_microusd)}${event.cap_applied ? ' capped' : ''}` : '--'}</span>,
              <span key="created">{formatDate(event.created_at)}</span>,
            ])}
          />
        ) : (
          <EmptyState title="No usage events matched" body="Adjust the filter set or wait for fresh policy traffic to enter the audit stream." />
        )}
      </Panel>
    </div>
  )
}

function creditsFromMicrousd(value?: number) {
  const credits = Math.ceil(((value ?? 0) * 100) / 1_000_000)
  return `${credits} credits`
}
