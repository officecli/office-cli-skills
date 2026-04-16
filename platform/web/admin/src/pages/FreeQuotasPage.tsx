import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Search, ShieldPlus } from 'lucide-react'
import { useState } from 'react'
import { api } from '../api'
import { EmptyState, Panel, SectionHeading, formatDate, formatNumber } from '../components/ui'

export default function FreeQuotasPage() {
  const queryClient = useQueryClient()
  const [fingerprint, setFingerprint] = useState('')
  const [usageDate, setUsageDate] = useState('')
  const { data: quotas = [] } = useQuery({ queryKey: ['admin-free-quotas', fingerprint, usageDate], queryFn: () => api.freeQuotas(fingerprint, usageDate) })
  const update = useMutation({
    mutationFn: ({ id, free_limit }: { id: number; free_limit: number }) => api.updateFreeQuota(id, free_limit),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['admin-free-quotas'] })
    },
  })

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading
          eyebrow="Abuse control"
          title="Free trial devices"
          body="Inspect daily anonymous trial state by CLI machine fingerprint and raise the daily limit only when onboarding or abuse review justifies it."
        />
        <div className="mb-6 grid gap-4 md:grid-cols-2">
          <label className="surface-console soft-panel flex items-center gap-3 border border-outline-variant/20 px-4 py-3 text-sm text-outline">
            <Search size={16} className="text-primary" />
            <input className="w-full bg-transparent outline-none" placeholder="Filter by fingerprint hash" value={fingerprint} onChange={(event) => setFingerprint(event.target.value)} />
          </label>
          <label className="surface-console soft-panel flex items-center gap-3 border border-outline-variant/20 px-4 py-3 text-sm text-outline">
            <input className="w-full bg-transparent outline-none" placeholder="Usage date (YYYY-MM-DD)" value={usageDate} onChange={(event) => setUsageDate(event.target.value)} />
          </label>
        </div>
        {quotas.length ? (
          <div className="grid gap-4 xl:grid-cols-2">
            {quotas.map((quota) => (
              <div key={quota.id} className="panel-muted p-5">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <div className="info-eyebrow text-outline">Fingerprint</div>
                    <code className="mt-2 block break-all font-mono text-sm text-white">{quota.fingerprint_hash}</code>
                  </div>
                  <button type="button" className="ghost-button" onClick={() => update.mutate({ id: quota.id, free_limit: quota.daily_limit + 1 })}>
                    <ShieldPlus size={14} /> Add 1 trial generation
                  </button>
                </div>
                <div className="mt-4 text-xs text-outline">Usage date {quota.usage_date}</div>
                <div className="mt-5 grid gap-3 sm:grid-cols-3">
                  <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Limit</div><div className="mt-2 text-white">{formatNumber(quota.daily_limit)}</div></div>
                  <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Used</div><div className="mt-2 text-white">{formatNumber(quota.daily_used)}</div></div>
                  <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Remaining</div><div className="mt-2 text-white">{formatNumber(quota.remaining)}</div></div>
                </div>
                <div className="mt-5 flex flex-wrap gap-6 text-xs text-outline">
                  <span>Created {formatDate(quota.created_at)}</span>
                  <span>Updated {formatDate(quota.updated_at)}</span>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No matching free quota records" body="Try a different fingerprint filter or wait for free-tier traffic to be recorded." />
        )}
      </Panel>
    </div>
  )
}
