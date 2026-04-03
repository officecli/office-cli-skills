import type { PropsWithChildren, ReactNode } from 'react'
import { cn, formatDate, formatNumber } from '../lib/utils'

export function Panel({ className, children }: PropsWithChildren<{ className?: string }>) {
  return <section className={cn('panel p-6', className)}>{children}</section>
}

export function SectionHeading({ eyebrow, title, body, action }: { eyebrow: string; title: string; body?: string; action?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
      <div>
        <div className="info-eyebrow text-primary">{eyebrow}</div>
        <h2 className="mt-2 text-3xl font-bold text-white">{title}</h2>
        {body ? <p className="mt-2 max-w-2xl text-sm text-outline">{body}</p> : null}
      </div>
      {action}
    </div>
  )
}

export function MetricCard({ label, value, detail, tone = 'default' }: { label: string; value: string; detail: string; tone?: 'default' | 'critical' | 'warning' }) {
  const toneClass = tone === 'critical' ? 'border-error/20' : tone === 'warning' ? 'border-tertiary/20' : 'border-outline-variant/10'
  return (
    <div className={cn('panel-muted border p-5', toneClass)}>
      <div className="info-eyebrow-mid text-outline">{label}</div>
      <div className="mt-3 text-3xl font-bold text-white">{value}</div>
      <div className="mt-2 text-sm text-outline">{detail}</div>
    </div>
  )
}

export function StatusPill({ value }: { value: string }) {
  const styles = value === 'active' || value === 'allowed'
    ? 'bg-secondary/15 text-secondary border-secondary/20'
    : value === 'disabled' || value === 'blocked'
      ? 'bg-error/15 text-error border-error/20'
      : 'bg-tertiary/15 text-tertiary border-tertiary/20'
  return <span className={cn('info-eyebrow-tight inline-flex rounded-full border px-3 py-1', styles)}>{value}</span>
}

export function DataTable({ headers, rows, columns }: { headers: string[]; rows: ReactNode[][]; columns?: string }) {
  return (
    <div className="soft-panel overflow-hidden border border-outline-variant/15">
      <div className="info-eyebrow-tight grid bg-surface-container-high/70 text-outline" style={{ gridTemplateColumns: columns ?? `repeat(${headers.length}, minmax(0, 1fr))` }}>
        {headers.map((header) => (
          <div key={header} className="px-4 py-3">{header}</div>
        ))}
      </div>
      <div className="divide-y divide-outline-variant/10">
        {rows.map((row, index) => (
          <div key={index} className="grid items-center bg-surface-container-low/40 text-sm" style={{ gridTemplateColumns: columns ?? `repeat(${headers.length}, minmax(0, 1fr))` }}>
            {row.map((cell, cellIndex) => <div key={cellIndex} className="px-4 py-4 text-outline">{cell}</div>)}
          </div>
        ))}
      </div>
    </div>
  )
}

export function EmptyState({ title, body }: { title: string; body: string }) {
  return (
    <div className="panel-muted p-8 text-center">
      <div className="text-lg font-semibold text-white">{title}</div>
      <div className="mt-2 text-sm text-outline">{body}</div>
    </div>
  )
}

export { formatDate, formatNumber }
