import type { PropsWithChildren, ReactNode } from 'react'
import { Card, Empty, Skeleton as AntSkeleton, Statistic, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { cn, formatDate, formatNumber } from '../lib/utils'

export function Panel({ className, children }: PropsWithChildren<{ className?: string }>) {
  return <Card className={cn('app-panel', className)}>{children}</Card>
}

export function SectionHeading({ eyebrow, title, body, action }: { eyebrow: string; title: string; body?: string; action?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
      <div>
        <Typography.Text className="app-eyebrow" type="secondary">{eyebrow}</Typography.Text>
        <Typography.Title level={2} className="!mb-0 !mt-2 !text-2xl">{title}</Typography.Title>
        {body ? <Typography.Paragraph className="!mb-0 !mt-2 max-w-2xl" type="secondary">{body}</Typography.Paragraph> : null}
      </div>
      {action}
    </div>
  )
}

export function MetricCard({ label, value, detail }: { label: string; value: string; detail: ReactNode }) {
  return (
    <Card className="app-metric-card" size="small">
      <Statistic title={label} value={value} />
      <Typography.Text type="secondary" className="mt-2 block text-sm">{detail}</Typography.Text>
    </Card>
  )
}

export function StatusPill({ value }: { value: string }) {
  const normalized = value.trim().toLowerCase().replace(/[\s-]+/g, '_')
  return <Tag color={statusPillColor(normalized)}>{value}</Tag>
}

function statusPillColor(value: string) {
  if (matchesStatus(value, ['active', 'allowed', 'paid', 'complete', 'completed', 'success', 'succeeded', 'verified'])) {
    return 'success'
  }
  if (matchesStatus(value, ['pending', 'processing', 'queued', 'reconciling', 'incomplete', 'requires_action', 'action_required', 'trialing', 'awaiting_payment'])) {
    return 'warning'
  }
  if (matchesStatus(value, ['disabled', 'blocked', 'failed', 'failure', 'error', 'denied', 'expired'])) {
    return 'error'
  }
  return 'default'
}

function matchesStatus(value: string, exactValues: string[]) {
  if (exactValues.includes(value)) {
    return true
  }

  return exactValues.some((candidate) => value.endsWith(`_${candidate}`))
}

export function DataTable({ headers, rows }: { headers: string[]; rows: ReactNode[][] }) {
  const tableColumns: ColumnsType<Record<string, ReactNode>> = headers.map((header, index) => ({
    title: header,
    dataIndex: `col_${index}`,
    key: `col_${index}`,
    render: (value: ReactNode) => value,
  }))
  const dataSource = rows.map((row, rowIndex) => Object.fromEntries([
    ['key', rowIndex],
    ...row.map((cell, cellIndex) => [`col_${cellIndex}`, cell]),
  ]))
  const minWidth = Math.max(headers.length * 160, 720)
  return (
    <div className="app-table-scroll">
      <Table
        className="app-table"
        columns={tableColumns}
        dataSource={dataSource}
        pagination={false}
        size="small"
        scroll={{ x: minWidth }}
      />
    </div>
  )
}

export function KeyStat({ label, value, meta }: { label: string; value?: number; meta: string }) {
  return (
    <Card className="app-key-stat" size="small">
      <Statistic title={label} value={formatNumber(value)} />
      <Typography.Text type="secondary" className="mt-1 block text-xs">{meta}</Typography.Text>
    </Card>
  )
}

export function EmptyState({ title, body }: { title: string; body: string }) {
  return (
    <Empty
      className="app-empty"
      description={(
        <div>
          <Typography.Text strong>{title}</Typography.Text>
          <Typography.Paragraph className="!mb-0 !mt-2" type="secondary">{body}</Typography.Paragraph>
        </div>
      )}
    />
  )
}

export function Skeleton({ className }: { className?: string }) {
  return <AntSkeleton.Input active className={cn('app-skeleton', className)} />
}

export function SkeletonMetricCard() {
  return (
    <div className="app-card-muted p-5">
      <Skeleton className="h-4 w-24" />
      <Skeleton className="mt-3 h-9 w-16" />
      <Skeleton className="mt-2 h-4 w-48" />
    </div>
  )
}

export function SkeletonDataTable({ columns, rows = 3 }: { columns: number; rows?: number }) {
  return (
    <div className="app-surface-panel overflow-hidden border border-outline-variant/15">
      <div className="info-eyebrow-tight grid bg-surface-container-high/70 text-outline" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
        {Array.from({ length: columns }).map((_, i) => (
          <div key={i} className="px-4 py-3"><Skeleton className="h-4 w-full" /></div>
        ))}
      </div>
      <div className="divide-y divide-outline-variant/10">
        {Array.from({ length: rows }).map((_, i) => (
          <div key={i} className="grid items-center bg-surface-container-low/40 text-sm" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
            {Array.from({ length: columns }).map((_, j) => (
              <div key={j} className="px-4 py-4 text-outline"><Skeleton className="h-4 w-full" /></div>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

export { formatDate, formatNumber }
