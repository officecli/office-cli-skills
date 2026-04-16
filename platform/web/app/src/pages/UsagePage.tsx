import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, StatusPill, SkeletonDataTable, formatDate } from '../components/ui'

export default function UsagePage() {
  const { data, isLoading } = useQuery({ queryKey: ['app-usage'], queryFn: api.usage })
  const events = Array.isArray(data) ? data : []

  return (
    <Panel>
      <SectionHeading eyebrow="Execution trail" title="Recent workflow usage" body="Track which requests were allowed, what mode they used, and why any call was blocked before it reached document generation." />
      {isLoading ? (
        <SkeletonDataTable columns={6} rows={5} />
      ) : events.length ? (
        <DataTable
          headers={['Mode', 'Action', 'Result', 'Fingerprint', 'Reason', 'Timestamp']}
          rows={events.map((event) => [
            <span key="mode" className="text-white">{event.mode}</span>,
            <span key="action" className="text-white">{event.action}</span>,
            <StatusPill key="result" value={event.result} />,
            <code key="fingerprint" className="font-mono text-xs text-outline">{event.fingerprint_hash}</code>,
            <span key="reason">{event.reason_code || '--'}</span>,
            <span key="created">{formatDate(event.created_at)}</span>,
          ])}
        />
      ) : (
        <EmptyState title="No usage events recorded" body="This workspace has not logged any app usage yet. Once requests hit the platform, the event stream will populate here." />
      )}
    </Panel>
  )
}
