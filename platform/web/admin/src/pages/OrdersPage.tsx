import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'

export default function OrdersPage() {
  const queryClient = useQueryClient()
  const { data: orders = [] } = useQuery({ queryKey: ['admin-orders'], queryFn: api.orders })
  const update = useMutation({
    mutationFn: ({ id, status }: { id: number; status: string }) => api.updateOrder(id, { status }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['admin-orders'] })
    },
  })

  return (
    <Panel>
      <SectionHeading
        eyebrow="Revenue trail"
        title="Orders and checkout state"
        body="Inspect account hosted credit purchases, legacy target-key metadata, and last-mile payment status from a single operator surface."
      />
      {orders.length ? (
        <DataTable
          headers={['Order', 'Pack', 'Destination', 'Status', 'Created', 'Action']}
          columns="minmax(0,0.9fr) minmax(0,1.4fr) minmax(0,1fr) minmax(0,0.8fr) minmax(0,1fr) minmax(0,0.9fr)"
          rows={orders.map((order) => [
            <div key={`order-${order.id}`}>
              <div className="font-semibold text-white">#{order.id}</div>
              <div className="mt-1 text-xs text-outline">user #{order.user_id}</div>
            </div>,
            <div key={`pack-${order.id}`}>
              <div className="font-semibold text-white">{order.pack_name}</div>
              <div className="mt-1 text-xs text-outline">{order.pack_code} / {orderAmountLabel(order)}</div>
            </div>,
            <span key={`target-${order.id}`}>{order.target_api_key_id ? `Legacy key #${order.target_api_key_id}` : 'Account credits'}</span>,
            <StatusPill key={`status-${order.id}`} value={order.status} />,
            <span key={`created-${order.id}`}>{formatDate(order.created_at)}</span>,
            <button
              key={`action-${order.id}`}
              type="button"
              className="admin-secondary-button"
              disabled={update.isPending || order.status === 'refunded'}
              onClick={() => update.mutate({ id: order.id, status: order.status === 'pending' ? 'failed' : 'refunded' })}
            >
              {order.status === 'pending' ? 'Mark failed' : 'Mark refunded'}
            </button>,
          ])}
        />
      ) : (
        <EmptyState title="No orders yet" body="Stripe checkout orders will appear here after the first billing flow starts." />
      )}
    </Panel>
  )
}

function orderAmountLabel(order: { pack_kind?: string; credit_amount?: number; amount_total: number; currency: string }) {
  if (order.pack_kind === 'hosted_credits') {
    return `${order.credit_amount ?? 0} credits / ≈ ${(order.amount_total / 100).toFixed(2)} ${order.currency.toUpperCase()}`
  }
  return `${(order.amount_total / 100).toFixed(2)} ${order.currency.toUpperCase()}`
}
