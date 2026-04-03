import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'

export default function BillingEventsPage() {
  const { data: events = [] } = useQuery({ queryKey: ['admin-billing-events'], queryFn: api.billingEvents })

  return (
    <Panel>
      <SectionHeading
        eyebrow="Webhook ledger"
        title="Billing events"
        body="Track Stripe webhook ingestion, ignored events, and failed payment-side transitions without digging through raw logs."
      />
      {events.length ? (
        <DataTable
          headers={['Event', 'Type', 'Order', 'Status', 'Processed', 'Error']}
          columns="minmax(0,1.4fr) minmax(0,1.2fr) minmax(0,0.8fr) minmax(0,0.8fr) minmax(0,1fr) minmax(0,1.2fr)"
          rows={events.map((event) => [
            <div key={`event-${event.id}`}>
              <div className="font-semibold text-white">{event.event_id}</div>
              <div className="mt-1 text-xs text-outline">#{event.id}</div>
            </div>,
            <span key={`type-${event.id}`}>{event.event_type}</span>,
            <span key={`order-${event.id}`}>{event.order_id ? `#${event.order_id}` : '—'}</span>,
            <StatusPill key={`status-${event.id}`} value={event.status} />,
            <span key={`processed-${event.id}`}>{formatDate(event.processed_at || event.created_at)}</span>,
            <span key={`error-${event.id}`} className="text-xs">{event.error_message || '—'}</span>,
          ])}
        />
      ) : (
        <EmptyState title="No billing events yet" body="Webhook events will appear here after Stripe sends the first callback." />
      )}
    </Panel>
  )
}
