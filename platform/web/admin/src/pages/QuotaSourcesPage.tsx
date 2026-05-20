import { FormEvent, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'

interface FilterState {
  fingerprint: string
  usage_date: string
  key_prefix: string
  user_id: string
}

const defaultFilters: FilterState = {
  fingerprint: '',
  usage_date: '',
  key_prefix: '',
  user_id: '',
}

export default function QuotaSourcesPage() {
  const [draft, setDraft] = useState<FilterState>(defaultFilters)
  const [filters, setFilters] = useState<FilterState>(defaultFilters)

  const { data } = useQuery({
    queryKey: ['admin-quota-sources', filters],
    queryFn: () => {
      const params = new URLSearchParams()
      if (filters.fingerprint) params.set('fingerprint', filters.fingerprint)
      if (filters.usage_date) params.set('usage_date', filters.usage_date)
      if (filters.key_prefix) params.set('key_prefix', filters.key_prefix)
      if (filters.user_id) params.set('user_id', filters.user_id)
      return api.quotaSources(params)
    },
  })

  const freeTrialDevices = data?.free_trial_devices ?? []
  const rewardGrants = data?.reward_grants ?? []
  const paidExternalKeys = data?.paid_external_keys ?? []
  const hostedKeys = data?.hosted_keys ?? []

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading
          eyebrow="Unified quota audit"
          title="Quota sources"
          body="Inspect anonymous trial compatibility views, legacy reward grants, paid external quota keys, and account hosted credits from one operator surface."
        />
        <form className="admin-code-card admin-surface-panel grid gap-4 border border-outline-variant/20 p-5 md:grid-cols-4" onSubmit={(event: FormEvent) => {
          event.preventDefault()
          setFilters(draft)
        }}>
          <label className="text-sm text-outline">Fingerprint
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.fingerprint} onChange={(event) => setDraft((current) => ({ ...current, fingerprint: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Usage date
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" placeholder="YYYY-MM-DD" value={draft.usage_date} onChange={(event) => setDraft((current) => ({ ...current, usage_date: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Key prefix
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.key_prefix} onChange={(event) => setDraft((current) => ({ ...current, key_prefix: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">User ID
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.user_id} onChange={(event) => setDraft((current) => ({ ...current, user_id: event.target.value }))} />
          </label>
          <div className="md:col-span-4 flex gap-3">
            <button type="submit" className="admin-primary-button">Apply filters</button>
            <button type="button" className="admin-secondary-button" onClick={() => {
              setDraft(defaultFilters)
              setFilters(defaultFilters)
            }}>Reset</button>
          </div>
        </form>
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Anonymous trial" title="CLI free trial devices" body="These daily_free_quotas rows are a legacy daily audit and compatibility view. Current anonymous license state is tracked in free_quotas." />
        {freeTrialDevices.length ? (
          <div className="grid gap-4 xl:grid-cols-2">
            {freeTrialDevices.map((quota) => (
              <div key={quota.id} className="admin-card-muted p-5">
                <div className="text-xs text-outline">{quota.usage_date}</div>
                <code className="mt-2 block break-all text-sm text-white">{quota.fingerprint_hash}</code>
                <div className="mt-4 grid gap-3 sm:grid-cols-3">
                  <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Limit</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber(quota.daily_limit)}</div></div>
                  <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Used</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber(quota.daily_used)}</div></div>
                  <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Remaining</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber(quota.remaining)}</div></div>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No free trial devices matched" body="Adjust the filters or wait for legacy daily audit rows to be recorded." />
        )}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Legacy account rewards" title="Reward grants" body="Legacy reward grants remain account-owned historical quota and should never be mixed into anonymous trial counts or account hosted credits." />
        {rewardGrants.length ? (
          <div className="space-y-3">
            {rewardGrants.map((grant) => (
              <div key={grant.id} className="admin-card-muted flex flex-wrap items-center justify-between gap-4 p-5">
                <div>
                  <div className="text-white">{grant.reason}</div>
                  <div className="mt-1 text-sm text-outline">User {grant.user_id} / {grant.source_type} / created {formatDate(grant.created_at)}</div>
                </div>
                <div className="text-right">
                  <div className="text-white">{formatNumber(grant.amount_total - grant.amount_used)} remaining</div>
                  <div className="text-xs text-outline">{formatNumber(grant.amount_used)} used / {formatNumber(grant.amount_total)} total</div>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No reward grants matched" body="Try filtering by user ID if you are tracing a single account." />
        )}
      </Panel>

      <div className="grid gap-6 xl:grid-cols-2">
        <Panel>
          <SectionHeading eyebrow="Paid external" title="Paid external quota keys" body="These keys carry billable document-generation quota." />
          {paidExternalKeys.length ? (
            <div className="space-y-3">
              {paidExternalKeys.map((key) => (
                <div key={key.id} className="admin-card-muted p-5">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <div className="text-white">{key.key_prefix}</div>
                      <div className="mt-1 text-sm text-outline">{key.plan_name}</div>
                    </div>
                    <StatusPill value={key.status} />
                  </div>
                  <div className="mt-4 grid gap-3 sm:grid-cols-3">
                    <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Total</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber(key.quota_total)}</div></div>
                    <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Used</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber(key.quota_used)}</div></div>
                    <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Remaining</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber(key.quota_remaining)}</div></div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState title="No paid keys matched" body="Filter by key prefix or user ID to narrow a paid quota trace." />
          )}
        </Panel>

        <Panel>
          <SectionHeading eyebrow="Account hosted" title="Hosted credit credentials" body="These rows are legacy key-shaped compatibility views; the authoritative hosted credits balance is account-level." />
          {hostedKeys.length ? (
            <div className="space-y-3">
              {hostedKeys.map((key) => (
                <div key={key.id} className="admin-card-muted p-5">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <div className="text-white">{key.key_prefix}</div>
                      <div className="mt-1 text-sm text-outline">{key.plan_name}</div>
                    </div>
                    <StatusPill value={key.status} />
                  </div>
                  <div className="mt-4 grid gap-3 sm:grid-cols-3">
                    <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Balance</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber(key.credit_balance)}</div></div>
                    <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Reserved</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber(key.credit_reserved)}</div></div>
                    <div className="admin-card-muted p-4"><div className="info-eyebrow-tight text-outline">Available</div><div className="mt-2 text-2xl font-bold text-white">{formatNumber((key.credit_balance ?? 0) - (key.credit_reserved ?? 0))}</div></div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState title="No hosted credentials matched" body="Hosted credit compatibility rows will appear here when the current filter set matches hosted-enabled credentials." />
          )}
        </Panel>
      </div>
    </div>
  )
}
