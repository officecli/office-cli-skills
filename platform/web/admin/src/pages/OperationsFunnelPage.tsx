import { useQuery } from '@tanstack/react-query'
import { Segmented, Typography } from 'antd'
import { useState } from 'react'
import { api } from '../api'
import { MetricCard, Panel, SectionHeading, formatDate, formatNumber } from '../components/ui'
import type { OperationsFunnel } from '../types'

type RangeKey = '24h' | '7d' | '30d'

export default function OperationsFunnelPage() {
  const [range, setRange] = useState<RangeKey>('30d')
  const { data: funnel } = useQuery({ queryKey: ['admin-operations-funnel', range], queryFn: () => api.operationsFunnel(range) })

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading
          eyebrow="Operations funnel"
          title="Acquisition, activation, usage, and revenue"
          body={funnel ? `${formatDate(funnel.window_start)} - ${formatDate(funnel.window_end)}` : 'Loading funnel window...'}
          action={<Segmented<RangeKey> options={['24h', '7d', '30d']} value={range} onChange={setRange} />}
        />
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <MetricCard label="Visitors" value={formatNumber(funnel?.visitors)} detail="Distinct visitor_id values from first-party events" />
          <MetricCard label="Pricing Views" value={formatNumber(funnel?.pricing_views)} detail="Pricing page and section view events" />
          <MetricCard label="CTA Clicks" value={formatNumber(funnel?.cta_clicks)} detail="CTA, download, and console-open clicks" />
          <MetricCard label="Login Starts" value={formatNumber(funnel?.login_starts)} detail="User login intent from site and app" />
        </div>
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Activation" title="Registered users to first active use" />
        <div className="grid gap-4 md:grid-cols-3 xl:grid-cols-6">
          <MetricCard label="Registered Users" value={formatNumber(funnel?.registered_users)} detail="users.created_at in the selected window" />
          <MetricCard label="Activated Users" value={formatNumber(funnel?.activated_users)} detail="Users seen in CLI sessions or usage events" />
          <MetricCard label="Activated Registered" value={formatNumber(funnel?.activated_registered_users)} detail="Registered cohort activated in-window" />
          <MetricCard label="Activation Rate" value={formatPercent(funnel?.activation_rate)} detail="Activated registered users divided by registered users" />
          <MetricCard label="CLI Login Started" value={formatNumber(funnel?.cli_login_started)} detail="CLI login challenges created" />
          <MetricCard label="CLI Login Completed" value={formatNumber(funnel?.cli_login_completed)} detail="CLI login challenges completed" />
          <MetricCard label="CLI Sessions" value={formatNumber(funnel?.cli_sessions_created)} detail="New CLI sessions created" />
        </div>
      </Panel>

      <div className="grid gap-4 xl:grid-cols-3">
        <QualityPanel title="Machine quality" items={machineItems(funnel)} />
        <QualityPanel title="Usage quality" items={usageItems(funnel)} />
        <QualityPanel title="Revenue quality" items={revenueItems(funnel)} />
      </div>

      <Panel>
        <SectionHeading eyebrow="Commercial funnel" title="Checkout to paid revenue" />
        <div className="grid gap-4 md:grid-cols-3 xl:grid-cols-6">
          <MetricCard label="Checkout Starts" value={formatNumber(funnel?.checkout_starts)} detail="First-party checkout_start events" />
          <MetricCard label="Orders Created" value={formatNumber(funnel?.orders_created)} detail="orders.created_at in the window" />
          <MetricCard label="Paid Orders" value={formatNumber(funnel?.paid_orders)} detail="Paid orders by updated_at" />
          <MetricCard label="Paid Users" value={formatNumber(funnel?.paid_users)} detail="Distinct paid order users in-window" />
          <MetricCard label="Paid Registered" value={formatNumber(funnel?.paid_registered_users)} detail="Registered cohort paid in-window" />
          <MetricCard label="Revenue" value={formatCurrency(funnel?.revenue)} detail="Paid order amount_total sum" />
          <MetricCard label="Repeat Paid Users" value={formatNumber(funnel?.repeat_paid_users)} detail="Lifetime paid orders >= 2 with a paid order in-window" />
        </div>
      </Panel>
    </div>
  )
}

function QualityPanel({ title, items }: { title: string; items: Array<{ label: string; value: string; detail: string }> }) {
  return (
    <Panel>
      <Typography.Title level={3} className="!mb-4 !text-lg">{title}</Typography.Title>
      <div className="space-y-3">
        {items.map((item) => (
          <div key={item.label} className="admin-card-muted flex items-center justify-between gap-4 p-4">
            <div>
              <div className="text-sm font-semibold text-white">{item.label}</div>
              <div className="mt-1 text-xs text-outline">{item.detail}</div>
            </div>
            <div className="text-xl font-bold text-white">{item.value}</div>
          </div>
        ))}
      </div>
    </Panel>
  )
}

function machineItems(funnel?: OperationsFunnel) {
  return [
    { label: 'Total Machines', value: formatNumber(funnel?.machine_quality?.total_machines), detail: 'Lifetime distinct non-hosted fingerprints' },
    { label: 'Active 24H', value: formatNumber(funnel?.machine_quality?.active_machines_24h), detail: 'Non-hosted fingerprints active in 24 hours' },
    { label: 'Active 7D', value: formatNumber(funnel?.machine_quality?.active_machines_7d), detail: 'Non-hosted fingerprints active in 7 days' },
  ]
}

function usageItems(funnel?: OperationsFunnel) {
  return [
    { label: 'First Usage Users', value: formatNumber(funnel?.usage_quality?.first_usage_users), detail: 'First usage identity landed in the window' },
    { label: 'Repeat Usage Users', value: formatNumber(funnel?.usage_quality?.repeat_usage_users), detail: 'Usage identity with at least two events' },
    { label: 'Blocked Rate', value: formatPercent(funnel?.usage_quality?.blocked_rate), detail: 'Blocked usage events divided by usage total' },
  ]
}

function revenueItems(funnel?: OperationsFunnel) {
  return [
    { label: 'Paid Conversion', value: formatPercent(funnel?.revenue_quality?.paid_conversion_rate), detail: 'Paid registered users divided by registered users' },
    { label: 'Checkout to Paid', value: formatPercent(funnel?.revenue_quality?.checkout_to_paid_rate), detail: 'Paid orders divided by checkout starts' },
    { label: 'Revenue', value: formatCurrency(funnel?.revenue_quality?.revenue), detail: 'Paid order revenue in cents' },
  ]
}

function formatPercent(value?: number | null) {
  if (value == null) return '--'
  return `${(value * 100).toFixed(1)}%`
}

function formatCurrency(value?: number | null) {
  if (value == null) return '--'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(value / 100)
}
