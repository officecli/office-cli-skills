import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { EmptyState, KeyStat, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'

const inviteRewardGuideHref = 'https://officecli.io/docs#invite-rewards'

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
          eyebrow="Legacy quota"
          title="Historical reward and external quota"
          body="External Mode is free and unlimited now. This page preserves legacy quota visibility for older grants and historical keys; Hosted credits are shown on Overview and Billing."
        />
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="app-card-muted p-5">
            <div className="info-eyebrow text-primary">Legacy reward quota</div>
            <div className="mt-3 text-4xl font-bold text-white">{formatNumber(rewardQuota?.remaining)}</div>
            <div className="mt-2 text-sm text-outline">
              <div>Historical bonus-generation grants. New signup and invite rewards are issued as hosted credits.</div>
              <a href={inviteRewardGuideHref} target="_blank" rel="noreferrer" className="mt-3 inline-flex text-xs font-semibold text-primary transition-colors hover:text-white">
                How invite rewards work
              </a>
            </div>
          </div>
          <div className="app-card-muted p-5">
            <div className="info-eyebrow text-tertiary">Legacy paid external quota</div>
            <div className="mt-3 text-4xl font-bold text-white">{formatNumber(paidQuota?.total_remaining)}</div>
            <div className="mt-2 text-sm text-outline">Historical paid document-generation quota across active and disabled keys. New external packs are no longer sold.</div>
          </div>
        </div>
      </Panel>

      <Panel>
        <SectionHeading
          eyebrow="Trial policy"
          title="External Mode free unlimited"
          body="External Mode generation no longer depends on anonymous trial buckets. Hosted Mode continues to use hosted credits."
        />
        <div className="app-card-muted p-5">
          <div className="flex items-center justify-between gap-4">
            <div>
              <div className="text-white">External generation is free and unlimited</div>
              <div className="mt-2 text-sm text-outline">{trialPolicy?.message ?? 'External Mode uses your configured model endpoint and does not consume OfficeCLI quota.'}</div>
            </div>
            <StatusPill value={trialPolicy?.cli_binary_only ? 'active' : 'blocked'} />
          </div>
        </div>
      </Panel>

      <Panel>
        <SectionHeading
          eyebrow="Rewards ledger"
          title="Legacy reward grant detail"
          body="Each legacy grant shows the original amount, what has been consumed, and what remains available now."
          action={(
            <a href={inviteRewardGuideHref} target="_blank" rel="noreferrer" className="app-secondary-button self-start text-xs">
              Referral rules
            </a>
          )}
        />
        {isLoading ? null : grants.length ? (
          <div className="space-y-3">
            {grants.map((grant) => (
              <div key={`${grant.source_type}-${grant.created_at}`} className="app-card-muted p-5">
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
          <EmptyState title="No legacy reward grants yet" body="New signup and invite benefits are issued as hosted credits and appear in hosted credit grant records." />
        )}
      </Panel>

      <Panel>
        <SectionHeading
          eyebrow="Legacy paid quota"
          title="Legacy paid quota by key"
          body="These rows preserve purchased external generations tied to each API key. New external purchases are disabled because External Mode is free and unlimited."
        />
        {isLoading ? null : keys.length ? (
          <div className="grid gap-4 lg:grid-cols-2">
            {keys.map((key) => (
              <div key={key.id} className="app-card-muted p-5">
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
          <EmptyState title="No legacy paid quota" body="Billing now sells hosted credits only. External Mode does not need a paid quota pack." />
        )}
      </Panel>
    </div>
  )
}
