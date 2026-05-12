import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLocation } from 'react-router-dom'
import { ArrowRight, Copy, CreditCard } from 'lucide-react'
import { ApiError, api } from '../api'
import { trackEvent } from '../analytics'
import { APP_ANALYTICS_EVENTS } from '../analytics-events'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'
import { redirectTo } from '../lib/navigation'
import type { ApiKey, Order } from '../types'

export default function BillingPage() {
  const location = useLocation()
  const queryClient = useQueryClient()
  const { data: pricing = [] } = useQuery({ queryKey: ['pricing'], queryFn: api.pricing })
  const { data: keys = [] } = useQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
  const pendingPollInterval = 5000
  const { data: orders = [] } = useQuery({
    queryKey: ['app-orders'],
    queryFn: api.orders,
    refetchInterval: (query) => query.state.data?.some((item) => item.status === 'pending') ? pendingPollInterval : false,
  })
  const activeKeys = useMemo(() => keys.filter((item) => item.status === 'active'), [keys])
  const keyByID = useMemo(() => new Map(keys.map((item) => [item.id, item])), [keys])
  const searchParams = useMemo(() => new URLSearchParams(location.search), [location.search])
  const checkoutSessionID = searchParams.get('session_id')?.trim() ?? ''
  const shouldAttemptReconcile = searchParams.get('status') === 'success' && checkoutSessionID !== ''
  const [selectedKey, setSelectedKey] = useState<number | null>(null)
  const [copiedReference, setCopiedReference] = useState<string | null>(null)
  const [reconciledSessionID, setReconciledSessionID] = useState<string | null>(null)

  const checkout = useMutation({
    mutationFn: ({ packCode, keyID }: { packCode: string; keyID: number }) => api.checkout({ pack_code: packCode, target_api_key_id: keyID }),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: ['app-orders'] })
      redirectTo(result.checkout_url)
    },
  })
  const reconcile = useMutation({
    mutationFn: ({ sessionID }: { sessionID: string }) => api.reconcileOrder({ checkout_session_id: sessionID }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['app-orders'] })
    },
  })

  const activeKey = useMemo(() => activeKeys.find((item) => item.id === selectedKey), [activeKeys, selectedKey])
  const checkoutError = checkout.error instanceof ApiError
    ? `${checkout.error.message}${checkout.error.requestId ? ` (request_id: ${checkout.error.requestId})` : ''}`
    : checkout.error?.message
  const reconcileError = reconcile.error instanceof ApiError
    ? `${reconcile.error.message}${reconcile.error.requestId ? ` (request_id: ${reconcile.error.requestId})` : ''}`
    : reconcile.error?.message

  useEffect(() => {
    if (!shouldAttemptReconcile || reconciledSessionID === checkoutSessionID) {
      return
    }
    setReconciledSessionID(checkoutSessionID)
    reconcile.mutate({ sessionID: checkoutSessionID })
  }, [checkoutSessionID, reconcile, reconciledSessionID, shouldAttemptReconcile])

  async function copyReference(reference: string) {
    await navigator.clipboard?.writeText(reference)
    setCopiedReference(reference)
    window.setTimeout(() => {
      setCopiedReference((current) => current === reference ? null : current)
    }, 1500)
  }

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading eyebrow="Secure checkout" title="Buy quota for an API key" body="Select the key that should receive more document generations or hosted credits, then continue into secure Stripe Checkout from the same workspace." />
        <div className="billing-shell mb-6">
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-primary">Target destination</div>
            <select className="surface-console mt-4 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={selectedKey ?? ''} onChange={(event) => setSelectedKey(event.target.value ? Number(event.target.value) : null)}>
              <option value="">{activeKeys.length ? 'Choose an active key' : 'No active key available'}</option>
              {activeKeys.map((key) => <option key={key.id} value={key.id}>{key.key_prefix} / {key.plan_name}</option>)}
            </select>
            <div className="mt-4 text-sm text-outline">
              {activeKey
                ? `${activeKey.key_prefix} has ${activeKey.quota_remaining} document generations and ${activeKey.credit_balance ?? 0} hosted credits remaining.`
                : activeKeys.length
                  ? 'Pick an active production key before starting checkout.'
                  : 'No active API key is available for billing. Re-enable a key in API Keys first.'}
            </div>
          </div>
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-tertiary">Billing flow</div>
            <ol className="mt-4 space-y-3 text-sm text-outline">
              <li>1. Choose the target API key.</li>
              <li>2. Pick a pricing pack below.</li>
              <li>3. Continue to Stripe Checkout and land back in the billing view.</li>
            </ol>
          </div>
        </div>
        {checkoutError ? (
          <div className="mb-6 rounded-2xl border border-rose-400/30 bg-rose-500/10 p-4 text-sm text-rose-100">
            Checkout failed: {checkoutError}
          </div>
        ) : null}
        {pricing.length ? (
          <div className="grid gap-4 lg:grid-cols-3">
            {pricing.map((pack) => (
              <div key={pack.code} className="panel-muted flex flex-col justify-between p-5">
                <div>
                  <div className="info-eyebrow text-outline">{pack.code}</div>
                  <div className="mt-3 text-2xl font-bold text-white">{pack.name}</div>
                  <div className="mt-2 text-sm text-outline">{pack.description}</div>
                  <div className="mt-6 text-4xl font-bold text-primary">{packPrimaryLabel(pack)}</div>
                  <div className="mt-2 text-sm text-outline">{packSecondaryLabel(pack)}</div>
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
                  <CreditCard size={16} /> Continue to Stripe Checkout
                </button>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="Pricing not loaded" body="The marketing site pricing API is currently unavailable for this workspace." />
        )}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Order trail" title="Recent billing activity" body="Every completed, pending, or failed pack purchase remains visible here for reconciliation." />
        {reconcile.isPending ? (
          <div className="mb-4 rounded-2xl border border-primary/20 bg-primary/10 p-4 text-sm text-white">
            Payment returned from Stripe. Syncing the latest checkout status into this workspace now.
          </div>
        ) : null}
        {reconcileError ? (
          <div className="mb-4 rounded-2xl border border-amber-400/30 bg-amber-500/10 p-4 text-sm text-amber-100">
            Stripe payment sync failed: {reconcileError}
          </div>
        ) : null}
        {orders.length ? (
          <div className="space-y-3">
            {orders.map((order) => (
              <div key={order.id} className="panel-muted flex flex-wrap items-center justify-between gap-4 p-5">
                <div>
                  <div className="text-lg font-semibold text-white">{order.pack_name}</div>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-outline">
                    <span>Stripe order</span>
                    <span className="rounded-md border border-outline-variant/20 bg-surface-container-high/50 px-2 py-1 font-mono text-xs text-white">
                      {stripeOrderReference(order) || 'pending assignment'}
                    </span>
                    {stripeOrderReference(order) ? (
                      <button type="button" className="ghost-button" onClick={() => copyReference(stripeOrderReference(order)!)}>
                        <Copy size={14} /> {copiedReference === stripeOrderReference(order) ? 'Copied' : 'Copy'}
                      </button>
                    ) : null}
                  </div>
                  <div className="mt-2 text-sm text-outline">{targetAPIKeyLabel(order, keyByID)}</div>
                  <div className="mt-1 text-xs text-outline">Internal order #{order.id} / created {formatDate(order.created_at)}</div>
                </div>
                <div className="flex items-center gap-4">
                  <div className="text-sm text-outline">{orderPrimaryLabel(order)} <ArrowRight size={14} className="inline" /> {orderSecondaryLabel(order)}</div>
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

function stripeOrderReference(order: Order) {
  return order.stripe_payment_intent_id || order.stripe_checkout_session_id || ''
}

function packPrimaryLabel(pack: { pack_kind?: string; amount_total: number; currency: string; credit_amount?: number }) {
  if (pack.pack_kind === 'hosted_credits') {
    return `${pack.credit_amount ?? 0} credits`
  }
  return `${(pack.amount_total / 100).toFixed(2)} ${pack.currency.toUpperCase()}`
}

function packSecondaryLabel(pack: { pack_kind?: string; amount_total: number; currency: string; quota_amount?: number; credit_amount?: number }) {
  if (pack.pack_kind === 'hosted_credits') {
    return `≈ ${usdLabel(pack.amount_total, pack.currency)} at 100 credits = $1 USD`
  }
  return `${pack.quota_amount ?? 0} document generations per purchase`
}

function orderPrimaryLabel(order: Order) {
  if (order.pack_kind === 'hosted_credits') {
    return `${order.credit_amount ?? 0} credits`
  }
  return `${(order.amount_total / 100).toFixed(2)} ${order.currency.toUpperCase()}`
}

function orderSecondaryLabel(order: Order) {
  if (order.pack_kind === 'hosted_credits') {
    return `≈ ${usdLabel(order.amount_total, order.currency)}`
  }
  return `${order.quota_amount} document generations`
}

function usdLabel(amountCents: number, currency: string) {
  return `${(amountCents / 100).toLocaleString('en-US', { style: 'currency', currency: currency.toUpperCase() })} USD`
}

function targetAPIKeyLabel(order: Order, keyByID: Map<number, ApiKey>) {
  if (!order.target_api_key_id) {
    return 'API key not recorded for this order.'
  }
  const key = keyByID.get(order.target_api_key_id)
  if (!key) {
    return `API key #${order.target_api_key_id}`
  }
  return `API key ${key.key_prefix} / #${key.id} / ${key.plan_name}`
}
