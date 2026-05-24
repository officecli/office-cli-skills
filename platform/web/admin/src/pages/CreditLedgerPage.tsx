import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Switch } from 'antd'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, formatDate } from '../components/ui'

export default function CreditLedgerPage() {
  const [includeZeroDelta, setIncludeZeroDelta] = useState(false)
  const { data: entries = [] } = useQuery({
    queryKey: ['admin-credit-ledger', includeZeroDelta],
    queryFn: () => api.creditLedger(includeZeroDelta),
  })

  return (
    <Panel>
      <SectionHeading
        eyebrow="Hosted credits"
        title="Credit ledger"
        body="Account-level hosted credit transactions. By default, zero-delta rows are hidden."
      />
      <div className="mb-4 flex items-center gap-3">
        <Switch checked={includeZeroDelta} onChange={setIncludeZeroDelta} />
        <span className="text-sm text-outline">Show zero-delta rows</span>
      </div>
      {entries.length ? (
        <DataTable
          headers={['User', 'Source', 'Delta', 'Reason', 'Usage', 'Created']}
          columns="minmax(0,0.6fr) minmax(0,1fr) minmax(0,0.6fr) minmax(0,1.2fr) minmax(0,0.6fr) minmax(0,1fr)"
          rows={entries.map((entry) => [
            <span key={`user-${entry.id}`}>#{entry.user_id}</span>,
            <span key={`source-${entry.id}`} className="font-mono text-xs">{entry.source_type}</span>,
            <span key={`delta-${entry.id}`} className={entry.credit_delta > 0 ? 'text-green-400' : entry.credit_delta < 0 ? 'text-red-400' : 'text-outline'}>{entry.credit_delta > 0 ? '+' : ''}{entry.credit_delta}</span>,
            <span key={`reason-${entry.id}`} className="text-xs">{entry.reason}</span>,
            <span key={`usage-${entry.id}`}>{entry.usage_event_id ? `#${entry.usage_event_id}` : '—'}</span>,
            <span key={`created-${entry.id}`}>{formatDate(entry.created_at)}</span>,
          ])}
        />
      ) : (
        <EmptyState title="No credit ledger entries" body="Hosted credit transactions will appear here once credits are granted, charged, or transferred." />
      )}
    </Panel>
  )
}
