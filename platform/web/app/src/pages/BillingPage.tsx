import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'
import { Alert, Button } from 'antd'
import { ArrowRight, Copy, CreditCard } from 'lucide-react'
import { ApiError, api } from '../api'
import { trackEvent } from '../analytics'
import { APP_ANALYTICS_EVENTS } from '../analytics-events'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'
import { redirectTo } from '../lib/navigation'
import type { Order } from '../types'

export default function BillingPage() {
  const location = useLocation()
  const queryClient = useQueryClient()
  const { data: pricingRaw = [] } = useQuery({ queryKey: ['pricing'], queryFn: api.pricing })
  const pendingPollInterval = 5000
  const { data: orders = [] } = useQuery({
    queryKey: ['app-orders'],
    queryFn: api.orders,
    refetchInterval: (query) => query.state.data?.some((item) => item.status === 'pending') ? pendingPollInterval : false,
  })
  const pricing = useMemo(() => pricingRaw.filter((pack) => pack.pack_kind === 'hosted_credits'), [pricingRaw])
  const searchParams = useMemo(() => new URLSearchParams(location.search), [location.search])
  const checkoutSessionID = searchParams.get('session_id')?.trim() ?? ''
  const shouldAttemptReconcile = searchParams.get('status') === 'success' && checkoutSessionID !== ''
  const [copiedReference, setCopiedReference] = useState<string | null>(null)
  const [reconciledSessionID, setReconciledSessionID] = useState<string | null>(null)
  const hasAutoStartedRef = useRef(false)

  const checkout = useMutation({
    mutationFn: ({ packCode }: { packCode: string }) => api.checkout({ pack_code: packCode }),
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

  const autostartPack = searchParams.get('pack')?.trim() ?? ''
  const shouldAutostart = searchParams.get('autostart') === '1' && autostartPack !== ''

  useEffect(() => {
    if (!shouldAutostart) return
    if (hasAutoStartedRef.current) return
    if (checkout.isPending) return
    const match = pricing.find(p => p.code === autostartPack)
    if (!match) return
    hasAutoStartedRef.current = true
    trackEvent(APP_ANALYTICS_EVENTS.checkoutStart, { surface: 'app', placement: 'deep-link-autostart', pack_code: match.code })
    checkout.mutate({ packCode: match.code })
  }, [shouldAutostart, autostartPack, pricing, checkout])

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
        <SectionHeading eyebrow="Secure checkout" title="Buy account hosted credits" body="External Mode is free and unlimited. Checkout adds hosted credits to your signed-in account, and all CLI sessions or API keys on that account share the same balance." />
        <div className="billing-shell mb-6">
          <div className="app-card-muted p-5">
            <div className="info-eyebrow text-primary">Target destination</div>
            <div className="mt-4 text-sm text-outline">
              Credits are added to your account balance. `officecli login` sessions and `officecli set-key` API key mode consume the same account hosted credits.
            </div>
            <div className="mt-3 text-xs text-outline">
              Usage rate: each AI image generation deducts 10 credits (~$0.10).
            </div>
          </div>
          <div className="app-card-muted p-5">
            <div className="info-eyebrow text-tertiary">Billing flow</div>
            <ol className="mt-4 space-y-3 text-sm text-outline">
              <li>1. Pick a hosted credits pack below.</li>
              <li>2. Continue to Stripe Checkout.</li>
              <li>3. Return to Billing and let the workspace reconcile payment status.</li>
            </ol>
          </div>
        </div>
        {checkoutError ? (
          <Alert className="mb-6" type="error" showIcon title={`Checkout failed: ${checkoutError}`} />
        ) : null}
        <div className="app-card-muted mb-4 p-4 text-sm text-outline">Have a redeem code? <Link to="/redeem" className="text-primary hover:text-white">Redeem here →</Link></div>
        {pricing.length ? (
          <div className="grid gap-4 lg:grid-cols-3">
            {pricing.map((pack) => (
              <div key={pack.code} className="app-card-muted flex flex-col justify-between p-5">
                <div>
                  <div className="info-eyebrow text-outline">{pack.code}</div>
                  <div className="mt-3 text-2xl font-bold text-white">{pack.name}</div>
                  <div className="mt-2 text-sm text-outline">{pack.description}</div>
                  <div className="mt-6 text-4xl font-bold text-primary">{packPrimaryLabel(pack)}</div>
                  <div className="mt-2 text-sm text-outline">{packSecondaryLabel(pack)}</div>
                </div>
                <Button
                  type="primary"
                  className="mt-8 w-full"
                  loading={checkout.isPending}
                  icon={<CreditCard size={16} />}
                  onClick={() => {
                    trackEvent(APP_ANALYTICS_EVENTS.checkoutStart, { surface: 'app', pack_code: pack.code })
                    checkout.mutate({ packCode: pack.code })
                  }}
                >
                  Continue to Stripe Checkout
                </Button>
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
          <Alert className="mb-4" type="info" showIcon title="Syncing payment status" description="Payment returned from Stripe. Syncing the latest checkout status into this workspace now." />
        ) : null}
        {reconcileError ? (
          <Alert className="mb-4" type="warning" showIcon title={`Stripe payment sync failed: ${reconcileError}`} />
        ) : null}
        {orders.length ? (
          <div className="space-y-3">
            {orders.map((order) => (
              <div key={order.id} className="app-card-muted flex flex-wrap items-center justify-between gap-4 p-5">
                <div>
                  <div className="text-lg font-semibold text-white">{order.pack_name}</div>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-outline">
                    <span>Stripe order</span>
                    <span className="rounded-md border border-outline-variant/20 bg-surface-container-high/50 px-2 py-1 font-mono text-xs text-white">
                      {stripeOrderReference(order) || 'pending assignment'}
                    </span>
                    {stripeOrderReference(order) ? (
                      <Button size="small" icon={<Copy size={14} />} onClick={() => copyReference(stripeOrderReference(order)!)}>
                        {copiedReference === stripeOrderReference(order) ? 'Copied' : 'Copy'}
                      </Button>
                    ) : null}
                  </div>
                  <div className="mt-2 text-sm text-outline">{targetAccountLabel(order)}</div>
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
    const creditAmount = pack.credit_amount ?? 0
    const perCredit = creditAmount > 0 ? (pack.amount_total / 100 / creditAmount).toFixed(3) : '0.000'
    const approxImages = Math.floor(creditAmount / 10)
    return `$${perCredit} / credit · ≈ ${approxImages} images at 10 credits each`
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

function targetAccountLabel(order: Order) {
  if (order.target_api_key_id) {
    return `Legacy order target API key #${order.target_api_key_id}; credits are now account-level.`
  }
  return 'Account hosted credits.'
}
