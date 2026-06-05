import type { PropsWithChildren, ReactNode } from 'react'
import { Card, Empty, Skeleton as AntSkeleton, Spin, Statistic, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { cn, formatDate, formatNumber } from '../lib/utils'

export function Panel({ className, children }: PropsWithChildren<{ className?: string }>) {
  return <Card className={cn('admin-panel', className)}>{children}</Card>
}

export function SectionHeading({ eyebrow, title, body, action }: { eyebrow: string; title: string; body?: string; action?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
      <div>
        <Typography.Text className="admin-eyebrow" type="secondary">{eyebrow}</Typography.Text>
        <Typography.Title level={2} className="!mb-0 !mt-2 !text-2xl">{title}</Typography.Title>
        {body ? <Typography.Paragraph className="!mb-0 !mt-2 max-w-2xl" type="secondary">{body}</Typography.Paragraph> : null}
      </div>
      {action}
    </div>
  )
}

export function MetricCard({ label, value, detail, tone = 'default' }: { label: string; value: string; detail: string; tone?: 'default' | 'critical' | 'warning' }) {
  const toneClass = tone === 'critical' ? 'admin-metric-critical' : tone === 'warning' ? 'admin-metric-warning' : undefined
  return (
    <Card className={cn('admin-metric-card', toneClass)} size="small">
      <Statistic title={label} value={value} />
      <Typography.Text type="secondary" className="mt-2 block text-sm">{detail}</Typography.Text>
    </Card>
  )
}

export function StatusPill({ value }: { value: string }) {
  const normalized = value.trim().toLowerCase().replace(/[\s-]+/g, '_')
  const color = matchesStatus(normalized, ['active', 'allowed', 'paid', 'complete', 'completed', 'success', 'succeeded', 'verified'])
    ? 'success'
    : matchesStatus(normalized, ['disabled', 'blocked', 'failed', 'failure', 'error', 'denied', 'expired'])
      ? 'error'
      : matchesStatus(normalized, ['pending', 'processing', 'queued', 'reconciling', 'incomplete', 'requires_action', 'action_required'])
        ? 'warning'
        : 'default'
  return <Tag color={color}>{value}</Tag>
}

function matchesStatus(value: string, exactValues: string[]) {
  if (exactValues.includes(value)) {
    return true
  }

  return exactValues.some((candidate) => value.endsWith(`_${candidate}`))
}

export function DataTable({ headers, rows, columns }: { headers: string[]; rows: ReactNode[][]; columns?: string }) {
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
  const minWidth = columns ? Math.max(headers.length * 160, 720) : undefined
  return (
    <div className="admin-table-scroll">
      <Table
        className="admin-table"
        columns={tableColumns}
        dataSource={dataSource}
        pagination={false}
        size="small"
        scroll={{ x: minWidth }}
      />
    </div>
  )
}

export function EmptyState({ title, body }: { title: string; body: string }) {
  return (
    <Empty
      className="admin-empty"
      description={(
        <div>
          <Typography.Text strong>{title}</Typography.Text>
          <Typography.Paragraph className="!mb-0 !mt-2" type="secondary">{body}</Typography.Paragraph>
        </div>
      )}
    />
  )
}

export function LoadingState({ label, className }: { label: string; className?: string }) {
  return (
    <div role="status" aria-label={label} className={cn('flex min-h-64 items-center justify-center', className)}>
      <Spin size="large" description={<Typography.Text type="secondary">{label}</Typography.Text>} />
    </div>
  )
}

export function Skeleton({ className }: { className?: string }) {
  return <AntSkeleton.Input active className={cn('admin-skeleton', className)} />
}

export { formatDate, formatNumber }
