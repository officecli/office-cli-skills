import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, CreditCard } from 'lucide-react'
import { api } from '../api'
import { trackEvent } from '../analytics'
import { APP_ANALYTICS_EVENTS } from '../analytics-events'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'
import type { Order, PricingPack } from '../types'

const PACK_COPY: Record<string, { name: string; description: string }> = {
  'external-100': {
    name: 'External 100',
    description: '100 external generations for workflows that already bring their own LLM.',
  },
  'external-500': {
    name: 'External 500',
    description: '500 external generations for teams running document work in bulk.',
  },
  'hosted-300': {
    name: 'Hosted 300',
    description: '300 hosted credits for low-volume runs on the platform-managed LLM runtime.',
  },
  'hosted-1200': {
    name: 'Hosted 1200',
    description: '1200 hosted credits for teams that want the platform-managed LLM runtime.',
  },
}

function billingPackCopy(pack: Pick<PricingPack, 'code' | 'name' | 'description'>) {
  return PACK_COPY[pack.code] ?? { name: pack.name, description: pack.description }
}

function billingOrderPackName(order: Pick<Order, 'pack_code' | 'pack_name'>) {
  return PACK_COPY[order.pack_code]?.name ?? order.pack_name
}

export default function BillingPage() {
  const queryClient = useQueryClient()
  const { data: pricing = [] } = useQuery({ queryKey: ['pricing'], queryFn: api.pricing })
  const { data: keys = [] } = useQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
  const { data: orders = [] } = useQuery({ queryKey: ['app-orders'], queryFn: api.orders })
  const [selectedKey, setSelectedKey] = useState<number | null>(null)

  const checkout = useMutation({
    mutationFn: ({ packCode, keyID }: { packCode: string; keyID: number }) => api.checkout({ pack_code: packCode, target_api_key_id: keyID }),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: ['app-orders'] })
      window.location.href = result.checkout_url
    },
  })

  const activeKey = useMemo(() => keys.find((item) => item.id === selectedKey), [keys, selectedKey])

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading eyebrow="Simple kinetic billing" title="Attach new credits to a real key" body="Select the key that should receive fresh capacity, then move into Stripe checkout from the same shell." />
        <div className="billing-shell mb-6">
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-primary">Target destination</div>
            <select className="surface-console mt-4 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={selectedKey ?? ''} onChange={(event) => setSelectedKey(event.target.value ? Number(event.target.value) : null)}>
              <option value="">Choose a key</option>
              {keys.map((key) => <option key={key.id} value={key.id}>{key.key_prefix} / {key.plan_name}</option>)}
            </select>
            <div className="mt-4 text-sm text-outline">{activeKey ? `${activeKey.key_prefix} has external ${activeKey.quota_remaining ?? activeKey.quota_total ?? 0} / hosted ${activeKey.credit_balance ?? 0}.` : 'Pick the production destination before starting checkout.'}</div>
          </div>
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-tertiary">Billing flow</div>
            <ol className="mt-4 space-y-3 text-sm text-outline">
              <li>1. Choose the target API key.</li>
              <li>2. Pick a pricing pack below.</li>
              <li>3. Complete checkout and land back in the billing view.</li>
            </ol>
          </div>
        </div>
        {pricing.length ? (
          <div className="grid gap-4 lg:grid-cols-3">
            {pricing.map((pack) => (
              <div key={pack.code} className="panel-muted flex flex-col justify-between p-5">
                {(() => {
                  const copy = billingPackCopy(pack)

                  return (
                    <>
                      <div>
                        <div className="info-eyebrow text-outline">{pack.code}</div>
                        <div className="mt-3 text-2xl font-bold text-white">{copy.name}</div>
                        <div className="mt-2 text-sm text-outline">{copy.description}</div>
                        <div className="mt-6 text-4xl font-bold text-primary">{(pack.amount_total / 100).toFixed(2)} <span className="text-sm text-outline">{pack.currency.toUpperCase()}</span></div>
                        <div className="mt-2 text-sm text-outline">{pack.pack_kind === 'hosted_credits' ? `${pack.credit_amount ?? 0} hosted credits per purchase` : `${pack.quota_amount} external generations per purchase`}</div>
                      </div>
                      <button
                        type="button"
                        className="tonal-button mt-8 w-full"
                        disabled={!selectedKey || checkout.isPending}
                        onClick={() => {
                          if (!selectedKey) return
                          trackEvent(APP_ANALYTICS_EVENTS.checkoutStart, { surface: 'app', pack_code: pack.code, target_api_key_id: selectedKey })
                          checkout.mutate({ packCode: pack.code, keyID: selectedKey })
                        }}
                      >
                        <CreditCard size={16} /> Checkout pack
                      </button>
                    </>
                  )
                })()}
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="Pricing not loaded" body="The marketing site pricing API is currently unavailable for this workspace." />
        )}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Order trail" title="Recent billing activity" body="Every completed, pending, or failed pack purchase remains visible here for reconciliation." />
        {orders.length ? (
          <div className="space-y-3">
            {orders.map((order) => (
              <div key={order.id} className="panel-muted flex flex-wrap items-center justify-between gap-4 p-5">
                <div>
                  <div className="text-lg font-semibold text-white">Order #{order.id}</div>
                  <div className="mt-1 text-sm text-outline">{billingOrderPackName(order)} / created {formatDate(order.created_at)}</div>
                </div>
                <div className="flex items-center gap-4">
                  <div className="text-sm text-outline">{(order.amount_total / 100).toFixed(2)} {order.currency.toUpperCase()} <ArrowRight size={14} className="inline" /> {order.pack_kind === 'hosted_credits' ? `${order.credit_amount ?? 0} hosted credits` : `${order.quota_amount} external generations`}</div>
                  <StatusPill value={order.status} />
                </div>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No billing events yet" body="Checkout history will appear here as soon as the first pack is purchased." />
        )}
      </Panel>
    </div>
  )
}
