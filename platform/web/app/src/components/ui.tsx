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

export function MetricCard({ label, value, detail }: { label: string; value: string; detail: ReactNode }) {
  return (
    <div className="panel-muted p-5">
      <div className="info-eyebrow-mid text-outline">{label}</div>
      <div className="mt-3 text-3xl font-bold text-white">{value}</div>
      <div className="mt-2 text-sm text-outline">{detail}</div>
    </div>
  )
}

export function StatusPill({ value }: { value: string }) {
  const styles = value === 'active' || value === 'allowed' ? 'bg-secondary/15 text-secondary border-secondary/20' : value === 'disabled' || value === 'blocked' ? 'bg-error/15 text-error border-error/20' : 'bg-tertiary/15 text-tertiary border-tertiary/20'
  return <span className={cn('info-eyebrow-tight inline-flex rounded-full border px-3 py-1', styles)}>{value}</span>
}

export function DataTable({ headers, rows }: { headers: string[]; rows: ReactNode[][] }) {
  return (
    <div className="soft-panel overflow-hidden border border-outline-variant/15">
      <div className="info-eyebrow-tight grid bg-surface-container-high/70 text-outline" style={{ gridTemplateColumns: `repeat(${headers.length}, minmax(0, 1fr))` }}>
        {headers.map((header) => (
          <div key={header} className="px-4 py-3">{header}</div>
        ))}
      </div>
      <div className="divide-y divide-outline-variant/10">
        {rows.map((row, index) => (
          <div key={index} className="grid items-center bg-surface-container-low/40 text-sm" style={{ gridTemplateColumns: `repeat(${headers.length}, minmax(0, 1fr))` }}>
            {row.map((cell, cellIndex) => <div key={cellIndex} className="px-4 py-4 text-outline">{cell}</div>)}
          </div>
        ))}
      </div>
    </div>
  )
}

export function KeyStat({ label, value, meta }: { label: string; value?: number; meta: string }) {
  return (
    <div className="panel-muted p-4">
      <div className="info-eyebrow-tight text-outline">{label}</div>
      <div className="mt-2 text-2xl font-bold text-white">{formatNumber(value)}</div>
      <div className="mt-1 text-xs text-outline">{meta}</div>
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

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-surface-container-high/50', className)} />
}

export function SkeletonMetricCard() {
  return (
    <div className="panel-muted p-5">
      <Skeleton className="h-4 w-24" />
      <Skeleton className="mt-3 h-9 w-16" />
      <Skeleton className="mt-2 h-4 w-48" />
    </div>
  )
}

export function SkeletonDataTable({ columns, rows = 3 }: { columns: number; rows?: number }) {
  return (
    <div className="soft-panel overflow-hidden border border-outline-variant/15">
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
