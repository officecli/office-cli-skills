import { useQuery } from '@tanstack/react-query'
import { Line, Pie } from '@ant-design/plots'
import { Button, Checkbox, Empty, Typography } from 'antd'
import { Activity, Download, ShieldAlert, Waypoints } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { DataTable, MetricCard, Panel, SectionHeading, formatNumber } from '../components/ui'
import { buildFingerprintQualityCsv } from '../fingerprintQualityCsv'
import type { FingerprintQualityRow, OverviewBreakdownItem, OverviewUsageTrendPoint } from '../types'

const guardrails = [
  'Admin sessions are issued only after company OAuth2 auth plus an exact allowlist match.',
  'Free quota edits should be used for abuse control and onboarding exceptions only.',
  'Usage event review is the fastest way to verify blocked traffic before touching billing-backed keys.',
]

export default function DashboardPage() {
  const [hideDefaultFiltered, setHideDefaultFiltered] = useState(true)
  const { data: overview } = useQuery({ queryKey: ['admin-overview'], queryFn: api.overview })
  const { data: funnel30d } = useQuery({ queryKey: ['admin-operations-funnel', '30d', 'dashboard'], queryFn: () => api.operationsFunnel('30d') })
  const { data: fingerprintQuality } = useQuery({ queryKey: ['admin-fingerprint-quality'], queryFn: api.fingerprintQuality })
  const funnel = funnel30d ?? overview?.operations_funnel_30d
  const trendData = toTrendChartData(overview?.usage_trend ?? [])
  const hasTrendData = trendData.some((item) => item.value > 0)
  const resultData = positiveBreakdown(overview?.result_breakdown ?? [])
  const modeData = positiveBreakdown(overview?.mode_breakdown ?? [])
  const apiKeyStatusData = positiveBreakdown(overview?.api_key_status_breakdown ?? [])
  const defaultFilteredBuckets = useMemo(() => new Set((fingerprintQuality?.summary ?? []).filter((item) => item.default_filtered).map((item) => item.bucket)), [fingerprintQuality?.summary])
  const fingerprintRows = useMemo(() => {
    const rows = fingerprintQuality?.rows ?? []
    if (!hideDefaultFiltered) return rows
    return rows.filter((row) => !defaultFilteredBuckets.has(row.bucket))
  }, [defaultFilteredBuckets, fingerprintQuality?.rows, hideDefaultFiltered])

  return (
    <div className="space-y-8">
      <Panel className="relative overflow-hidden">
        <div className="overview-hero-glow absolute inset-y-0 right-0 hidden w-1/2 lg:block" />
        <SectionHeading
          eyebrow="Platform overview"
          title="Governance metrics in one pass"
          body="Review key posture, free-machine pressure, and blocked traffic before taking any action on the platform."
          action={<Link to="/operations/funnel"><Button>Operations / Funnel</Button></Link>}
        />
        <div className="grid gap-4 xl:grid-cols-4">
          <MetricCard label="Total API Keys" value={formatNumber(overview?.total_api_keys)} detail="All managed credentials across the platform" />
          <MetricCard label="Active Keys" value={formatNumber(overview?.active_api_keys)} detail="Keys currently allowed to process traffic" />
          <MetricCard label="Total Machines" value={formatNumber(funnel?.machine_quality?.total_machines ?? overview?.free_machines)} detail="Distinct non-hosted fingerprints from usage events" tone="warning" />
          <MetricCard label="Blocked 24H" value={formatNumber(overview?.blocked_last_24h)} detail="Requests denied by policy in the last 24 hours" tone="critical" />
        </div>
      </Panel>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.45fr)_minmax(0,1fr)]">
        <ChartPanel title="7-day usage trend" description="Checks, consuming requests, and blocked traffic by UTC day." legend={['Checks', 'Consumes', 'Blocked']}>
          {hasTrendData ? (
            <Line
              {...baseChartConfig}
              data={trendData}
              xField="date"
              yField="value"
              colorField="metric"
              point={{ sizeField: 3 }}
              axis={{
                x: { labelFill: '#9aa4b2', lineStroke: '#2a3344' },
                y: { labelFill: '#9aa4b2', gridStroke: '#1f2937' },
              }}
              scale={{ color: { range: ['#7dd3fc', '#8b5cf6', '#fb7185'] } }}
            />
          ) : <ChartEmpty />}
        </ChartPanel>
        <div className="grid gap-4 md:grid-cols-3 xl:grid-cols-1">
          <PiePanel title="Result mix" data={resultData} colors={['#34d399', '#fb7185']} />
          <PiePanel title="Mode mix" data={modeData} colors={['#7dd3fc', '#fbbf24', '#a78bfa', '#2dd4bf']} />
          <PiePanel title="API key status" data={apiKeyStatusData} colors={['#34d399', '#f59e0b', '#fb7185', '#94a3b8']} />
        </div>
      </div>

      <div className="dashboard-shell">
        <Panel>
          <SectionHeading eyebrow="Traffic pressure" title="Policy-facing activity snapshot" body="These counters help operators decide whether the platform is seeing healthy document traffic or something that needs intervention." />
          <div className="grid gap-4 md:grid-cols-3">
            <MetricCard label="Checks 24H" value={formatNumber(overview?.checks_last_24h)} detail="Requests that reached policy evaluation" />
            <MetricCard label="Consumes 24H" value={formatNumber(overview?.consumes_last_24h)} detail="Requests that consumed free/reward/paid quota or hosted credits" />
            <MetricCard label="Expired Keys" value={formatNumber(overview?.expired_api_keys)} detail="Credentials that need attention or archival" />
          </div>
        </Panel>

        <Panel>
          <SectionHeading eyebrow="30-day operations funnel" title="Activation and revenue signal" />
          <div className="grid gap-4 md:grid-cols-2">
            <MetricCard label="Activated Users" value={formatNumber(funnel?.activated_users)} detail="CLI sessions or usage users in the last 30 days" />
            <MetricCard label="Paid Users" value={formatNumber(funnel?.paid_users)} detail="Distinct users with paid orders in the last 30 days" />
            <MetricCard label="Activation Rate" value={formatPercent(funnel?.activation_rate)} detail="Registered cohort activated in the selected window" />
            <MetricCard label="Paid Conversion Rate" value={formatPercent(funnel?.paid_conversion_rate)} detail="Registered cohort paid in the selected window" />
          </div>
        </Panel>

        <Panel>
          <SectionHeading eyebrow="Operator brief" title="Current stance" />
          <div className="space-y-4">
            <div className="admin-card-muted flex items-start gap-3 p-4"><ShieldAlert size={18} className="mt-0.5 text-tertiary" /><p className="text-sm text-outline">Use API key edits for metadata, status flips, and corrective quota work. Avoid treating the admin plane as a billing replacement.</p></div>
            <div className="admin-card-muted flex items-start gap-3 p-4"><Activity size={18} className="mt-0.5 text-primary" /><p className="text-sm text-outline">Blocked traffic spikes should be reviewed in Usage Events before relaxing any quota control.</p></div>
            <div className="admin-card-muted flex items-start gap-3 p-4"><Waypoints size={18} className="mt-0.5 text-secondary" /><p className="text-sm text-outline">All admin interactions assume an English-first operator experience and OfficeCLI brand naming.</p></div>
          </div>
        </Panel>
      </div>

      <Panel>
        <SectionHeading
          eyebrow="Fingerprint quality"
          title="Machine and CI fingerprint review"
          body="Filter probable Docker, CI, and internal test fingerprints from the detailed device view while keeping every bucket visible in the summary."
          action={(
            <div className="flex flex-col items-start gap-3 md:items-end">
              <Checkbox checked={hideDefaultFiltered} onChange={(event) => setHideDefaultFiltered(event.target.checked)}>
                Hide default-filtered buckets
              </Checkbox>
              <Button icon={<Download size={16} />} onClick={() => exportFingerprintRows(fingerprintRows)} disabled={!fingerprintRows.length}>
                Export CSV
              </Button>
            </div>
          )}
        />
        <div data-testid="fingerprint-quality-summary" className="mb-5 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {(fingerprintQuality?.summary ?? []).map((item) => (
            <div key={item.bucket} className="admin-card-muted p-4">
              <div className="flex items-start justify-between gap-3">
                <Typography.Text strong>{item.bucket}</Typography.Text>
                <Typography.Text type={item.default_filtered ? 'warning' : 'secondary'}>
                  {item.default_filtered ? 'filtered by default' : 'kept by default'}
                </Typography.Text>
              </div>
              <Typography.Paragraph className="!mb-3 !mt-2 text-sm" type="secondary">{item.reason}</Typography.Paragraph>
              <div className="grid grid-cols-2 gap-3 text-sm">
                <div><Typography.Text type="secondary">Fingerprints</Typography.Text><div>{formatNumber(item.fingerprints)}</div></div>
                <div><Typography.Text type="secondary">Events</Typography.Text><div>{formatNumber(item.events)}</div></div>
              </div>
            </div>
          ))}
        </div>
        {fingerprintRows.length ? (
          <DataTable
            headers={['Bucket', 'Reason', 'Fingerprint', 'Prefix', 'First', 'Last', 'Events', 'Generate', 'Status', 'Blocked', 'User-bound', 'IP count', 'IPs', 'CLI versions', 'Runtime modes', 'Document types', 'User agents']}
            rows={fingerprintRows.map((row) => [
              row.bucket,
              row.reason,
              row.fingerprint_hash,
              row.fingerprint_prefix,
              formatTimestamp(row.first_at),
              formatTimestamp(row.last_at),
              formatNumber(row.events),
              formatNumber(row.generate_events),
              formatNumber(row.status_events),
              formatNumber(row.blocked_events),
              formatNumber(row.user_bound_events),
              formatNumber(row.ip_count),
              joinList(row.ips),
              joinList(row.cli_versions),
              joinList(row.runtime_modes),
              joinList(row.document_types),
              joinList(row.user_agents),
            ])}
            columns="fingerprint-quality"
          />
        ) : (
          <Empty description="No fingerprint rows in the current filter" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        )}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Guardrails" title="What operators should verify before changing state" />
        <div className="grid gap-4 md:grid-cols-3">
          {guardrails.map((item) => (
            <div key={item} className="admin-card-muted p-5 text-sm text-outline">{item}</div>
          ))}
        </div>
      </Panel>
    </div>
  )
}

const baseChartConfig = {
  autoFit: true,
  height: 280,
  padding: 'auto',
  legend: false,
  tooltip: true,
  theme: 'classicDark',
} as const

function ChartPanel({ title, description, legend, children }: { title: string; description?: string; legend?: string[]; children: ReactNode }) {
  return (
    <Panel className="admin-chart-panel">
      <div className="mb-4 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <Typography.Title level={3} className="!mb-0 !text-lg">{title}</Typography.Title>
          {description ? <Typography.Paragraph className="!mb-0 !mt-1 text-sm" type="secondary">{description}</Typography.Paragraph> : null}
        </div>
        {legend?.length ? (
          <div className="admin-chart-legend">
            {legend.map((item) => <span key={item}>{item}</span>)}
          </div>
        ) : null}
      </div>
      <div className="admin-chart-body">{children}</div>
    </Panel>
  )
}

function PiePanel({ title, data, colors }: { title: string; data: OverviewBreakdownItem[]; colors: string[] }) {
  return (
    <ChartPanel title={title} legend={data.map((item) => item.label)}>
      {data.length ? (
        <Pie
          {...baseChartConfig}
          height={190}
          data={data}
          angleField="value"
          colorField="label"
          innerRadius={0.64}
          label={false}
          scale={{ color: { range: colors } }}
        />
      ) : <ChartEmpty compact />}
    </ChartPanel>
  )
}

function ChartEmpty({ compact = false }: { compact?: boolean }) {
  return (
    <div className={compact ? 'admin-chart-empty admin-chart-empty-compact' : 'admin-chart-empty'}>
      <Empty description="No chart data available" image={Empty.PRESENTED_IMAGE_SIMPLE} />
    </div>
  )
}

function toTrendChartData(points: OverviewUsageTrendPoint[]) {
  return points.flatMap((point) => [
    { date: point.date, metric: 'Checks', value: point.checks ?? 0 },
    { date: point.date, metric: 'Consumes', value: point.consumes ?? 0 },
    { date: point.date, metric: 'Blocked', value: point.blocked ?? 0 },
  ])
}

function positiveBreakdown(items: OverviewBreakdownItem[]) {
  return items.filter((item) => item.value > 0)
}

function joinList(values: string[]) {
  return values.length ? values.join('; ') : '--'
}

function formatTimestamp(value?: string) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function exportFingerprintRows(rows: FingerprintQualityRow[]) {
  const csv = buildFingerprintQualityCsv(rows)
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `fingerprint-quality-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function formatPercent(value?: number | null) {
  if (value == null) return '--'
  return `${(value * 100).toFixed(1)}%`
}
