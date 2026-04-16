import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { EmptyState, KeyStat, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'

export default function QuotaPage() {
  const { data, isLoading } = useQuery({ queryKey: ['app-quota-summary'], queryFn: api.quotaSummary })
  const rewardQuota = data?.reward_quota
  const paidQuota = data?.paid_external_quota
  const trialPolicy = data?.trial_policy
  const keys = paidQuota?.keys ?? []
  const grants = rewardQuota?.grants ?? []

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading
          eyebrow="Account quota"
          title="Reward and paid quota"
          body="Track account-owned quota here. Anonymous trial counts stay inside the local officecli binary and never become account balance."
        />
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-primary">Reward quota</div>
            <div className="mt-3 text-4xl font-bold text-white">{formatNumber(rewardQuota?.remaining)}</div>
            <div className="mt-2 text-sm text-outline">Bonus generations granted by invite activation and future growth programs.</div>
          </div>
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-tertiary">Paid external quota</div>
            <div className="mt-3 text-4xl font-bold text-white">{formatNumber(paidQuota?.total_remaining)}</div>
            <div className="mt-2 text-sm text-outline">Remaining paid document generations across all active and disabled keys in this workspace.</div>
          </div>
        </div>
      </Panel>

      <Panel>
        <SectionHeading
          eyebrow="Trial policy"
          title="CLI trial only"
          body="Anonymous trial counts are tracked on the machine that runs the officecli binary. They do not appear in account quota and are never added to account totals."
        />
        <div className="panel-muted p-5">
          <div className="flex items-center justify-between gap-4">
            <div>
              <div className="text-white">Anonymous trial remains binary-only</div>
              <div className="mt-2 text-sm text-outline">{trialPolicy?.message ?? 'Anonymous trial counts only apply to the local officecli binary and never count as account balance.'}</div>
            </div>
            <StatusPill value={trialPolicy?.cli_binary_only ? 'active' : 'blocked'} />
          </div>
        </div>
      </Panel>

      <Panel>
        <SectionHeading
          eyebrow="Rewards ledger"
          title="Reward grant detail"
          body="Each grant shows the original amount, what has been consumed, and what remains available now."
        />
        {isLoading ? null : grants.length ? (
          <div className="space-y-3">
            {grants.map((grant) => (
              <div key={`${grant.source_type}-${grant.created_at}`} className="panel-muted p-5">
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div>
                    <div className="text-white">{grant.reason}</div>
                    <div className="mt-1 text-xs text-outline">{grant.source_type} / created {formatDate(grant.created_at)}</div>
                  </div>
                  <StatusPill value={grant.remaining > 0 ? 'active' : 'blocked'} />
                </div>
                <div className="mt-4 grid gap-3 sm:grid-cols-3">
                  <KeyStat label="Total" value={grant.amount_total} meta="Granted" />
                  <KeyStat label="Used" value={grant.amount_used} meta="Consumed" />
                  <KeyStat label="Remaining" value={grant.remaining} meta="Available now" />
                </div>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No reward grants yet" body="Invite activation rewards and future growth grants will appear here after the backend issues them." />
        )}
      </Panel>

      <Panel>
        <SectionHeading
          eyebrow="Paid quota"
          title="Paid quota by key"
          body="These rows show the purchased document generations tied to each API key."
        />
        {isLoading ? null : keys.length ? (
          <div className="grid gap-4 lg:grid-cols-2">
            {keys.map((key) => (
              <div key={key.id} className="panel-muted p-5">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="text-white">{key.key_prefix}</div>
                    <div className="mt-1 text-sm text-outline">{key.plan_name}</div>
                  </div>
                  <StatusPill value={key.status} />
                </div>
                <div className="mt-4 grid gap-3 sm:grid-cols-3">
                  <KeyStat label="Total" value={key.quota_total} meta="Paid quota" />
                  <KeyStat label="Used" value={key.quota_used} meta="Consumed" />
                  <KeyStat label="Remaining" value={key.quota_remaining} meta="Available now" />
                </div>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No paid quota yet" body="Create a key and buy a pack from Billing to start building paid quota." />
        )}
      </Panel>
    </div>
  )
}
