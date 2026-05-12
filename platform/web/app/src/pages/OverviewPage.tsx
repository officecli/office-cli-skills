import { useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { useLocation } from 'react-router-dom'
import { api } from '../api'
import { APP_ANALYTICS_EVENTS } from '../analytics-events'
import { trackEvent } from '../analytics'
import { EmptyState, KeyStat, MetricCard, Panel, SectionHeading, StatusPill, Skeleton, SkeletonMetricCard, formatDate, formatNumber } from '../components/ui'

const inviteRewardGuideHref = 'https://officecli.io/docs#invite-rewards'

export default function OverviewPage() {
  const location = useLocation()
  const { data: overview, isLoading: isLoadingOverview } = useQuery({ queryKey: ['app-overview'], queryFn: api.overview })
  const { data: growth, isLoading: isLoadingGrowth } = useQuery({ queryKey: ['app-growth'], queryFn: api.growth })
  const { data: apiKeys = [], isLoading: isLoadingApiKeys } = useQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })

  const featuredKey = apiKeys[0]
  const rewardGrants = growth?.reward_grants ?? []
  const referrals = growth?.referrals ?? []
  const discordConnection = growth?.discord_connection
  const inviteCode = overview?.invite_code ?? growth?.invite_code
  const inviteLimit = overview?.invite_limit ?? growth?.invite_limit ?? 5
  const referralCount = overview?.referral_count ?? referrals.length
  const activatedReferralCount = overview?.activated_referral_count ?? referrals.filter((referral) => referral.activated_at).length
  const inviteRemaining = overview?.invite_remaining ?? growth?.invite_remaining ?? Math.max(inviteLimit - referrals.length, 0)
  const rewardPerInvite = overview?.reward_per_invite ?? growth?.reward_per_invite ?? 20
  const hostedCredits = overview?.hosted_credit_balance ?? apiKeys.reduce((sum, key) => sum + (key.credit_balance ?? 0), 0)
  const signupBonus = overview?.signup_credit_bonus ?? 30
  const discordParams = useMemo(() => new URLSearchParams(location.search), [location.search])
  const discordResult = discordParams.get('discord')
  const discordMessage = discordParams.get('discord_message')
  const discordReward = discordParams.get('discord_reward')
  const discordLoginHref = `/api/app/discord/login?return_to=${encodeURIComponent('/app')}`
  const growthStatusValue = useMemo(() => {
    if (!discordConnection) return 'NOT LINKED'
    if (discordConnection.verification_status === 'verified') return 'VERIFIED'
    return 'BLOCKED'
  }, [discordConnection])

  useEffect(() => {
    if (!growth) return
    trackEvent(APP_ANALYTICS_EVENTS.growthStatusView, {
      surface: 'app',
      reward_grants_count: rewardGrants.length,
      referrals_count: referrals.length,
      discord_status: discordConnection?.verification_status ?? 'not_linked',
    })
  }, [growth, rewardGrants.length, referrals.length, discordConnection?.verification_status])

  return (
    <div className="space-y-8">
      {discordResult ? (
        <Panel className={discordResult === 'verified' ? 'border border-secondary/20 bg-secondary/10' : 'border border-tertiary/20 bg-tertiary/10'}>
          <div className="text-sm text-white">
            Discord OAuth result: <span className="font-semibold">{discordResult}</span>
          </div>
          <div className="mt-2 text-sm text-outline">
            {discordMessage ?? (discordResult === 'verified' ? 'Discord account linked and guild membership verified.' : 'Discord account linked, but guild verification is still blocked until production configuration is provided.')}
          </div>
          {discordReward === 'granted' ? <div className="mt-2 text-sm text-secondary">Discord reward granted successfully.</div> : null}
        </Panel>
      ) : null}

      <Panel className="relative overflow-hidden">
        <div className="overview-hero-glow absolute inset-y-0 right-0 hidden w-1/2 lg:block" />
        <SectionHeading
          eyebrow="Runtime overview"
          title="Remaining Quota"
          body="Keep an eye on account quota, recent document traffic, and the key carrying the most production load."
        />
        <div className="overview-shell">
          <div className="panel-muted grid gap-4 p-6 md:grid-cols-3">
            {isLoadingOverview ? (
              <>
                <SkeletonMetricCard />
                <SkeletonMetricCard />
                <SkeletonMetricCard />
              </>
            ) : (
              <>
                <MetricCard label="API Keys" value={formatNumber(overview?.api_key_count)} detail="Active production and staging credentials" />
                <MetricCard label="Hosted Credits" value={formatNumber(hostedCredits)} detail={`${formatNumber(signupBonus)} credits granted to new users; hosted runtime spends credits`} />
                <MetricCard label="Recent Usage" value={formatNumber(overview?.recent_usage_count)} detail="External and hosted requests recorded recently" />
              </>
            )}
          </div>
          <div className="panel-muted p-6">
            <div className="info-eyebrow text-tertiary">Lead key</div>
            {isLoadingApiKeys ? (
              <div className="mt-4 space-y-4">
                <Skeleton className="h-6 w-48" />
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-24 w-full" />
              </div>
            ) : featuredKey ? (
              <div className="mt-4 space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <div className="text-lg font-semibold text-white">{featuredKey.key_prefix}</div>
                    <div className="mt-1 text-sm text-outline">{featuredKey.plan_name}</div>
                  </div>
                  <StatusPill value={featuredKey.status} />
                </div>
                <KeyStat label="Remaining Quota" value={featuredKey.quota_remaining ?? featuredKey.quota_total} meta={`Last used ${formatDate(featuredKey.last_used_at)}`} />
              </div>
            ) : (
              <EmptyState title="No keys provisioned yet" body="Create your first API key to start using paid quota in a real workflow." />
            )}
          </div>
        </div>
      </Panel>

      <div className="grid gap-4 xl:grid-cols-4">
        {isLoadingOverview || isLoadingGrowth ? (
          <>
            <SkeletonMetricCard />
            <SkeletonMetricCard />
            <SkeletonMetricCard />
            <SkeletonMetricCard />
          </>
        ) : (
          <>
            <MetricCard label="Orders" value={formatNumber(overview?.recent_orders_count)} detail="Recent billing events that landed in this workspace" />
            <MetricCard
              label="Invite Credits"
              value={formatNumber(overview?.reward_remaining)}
              detail={(
                <div>
                  <div>{inviteCode ? `Invite code: ${inviteCode} · ${formatNumber(rewardPerInvite)} hosted credits per activated invite` : 'No invite code available yet'}</div>
                  <a href={inviteRewardGuideHref} target="_blank" rel="noreferrer" className="mt-3 inline-flex text-xs font-semibold text-primary transition-colors hover:text-white">
                    How invite rewards work
                  </a>
                </div>
              )}
            />
            <MetricCard label="Referral Progress" value={`${formatNumber(referralCount)}/${formatNumber(inviteLimit)}`} detail={`${formatNumber(activatedReferralCount)} activated · ${formatNumber(inviteRemaining)} slots left`} />
            <MetricCard label="Discord Status" value={growthStatusValue} detail={discordConnection?.verification_blocked_reason ?? 'Guild membership determines Discord reward eligibility'} />
          </>
        )}
      </div>

      <Panel>
        <SectionHeading eyebrow="Key inventory" title="Current API key fleet" body="The newest or most active credentials stay visible here so you can spot quota pressure before it blocks document generation." />
        {apiKeys.length ? (
          <div className="grid gap-4 lg:grid-cols-2">
            {apiKeys.slice(0, 4).map((key) => (
              <div key={key.id} className="panel-muted p-5">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="flex items-center gap-2 text-white"><KeyRound size={16} className="text-primary" /> {key.key_prefix}</div>
                    <div className="mt-2 text-sm text-outline">{key.plan_name}</div>
                  </div>
                  <StatusPill value={key.status} />
                </div>
                <div className="mt-5 grid gap-3 sm:grid-cols-3">
                  <KeyStat label="Hosted" value={key.credit_balance ?? 0} meta="Credits" />
                  <KeyStat label="External" value={0} meta="Free unlimited" />
                  <KeyStat label="Remaining" value={key.quota_remaining} meta="Legacy quota" />
                </div>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No usage surfaces yet" body="As soon as a key is created, this overview becomes your live quota board." />
        )}
      </Panel>

      <div className="grid gap-6 xl:grid-cols-[1.2fr_1fr]">
        <Panel>
          <SectionHeading
            eyebrow="Rewards ledger"
            title="Reward grants and referral progress"
            body={`Each account can invite up to ${formatNumber(inviteLimit)} users, and every activated referral adds ${formatNumber(rewardPerInvite)} hosted credits.`}
            action={(
              <a href={inviteRewardGuideHref} target="_blank" rel="noreferrer" className="ghost-button self-start text-xs">
                Full invite guide
              </a>
            )}
          />
          {rewardGrants.length ? (
            <div className="space-y-3">
              {rewardGrants.map((grant) => (
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
            <EmptyState title="No reward grants yet" body="Invite activation rewards and future Discord rewards will appear here as soon as the backend writes them." />
          )}

          <div className="mt-6">
            <div className="info-eyebrow mb-3 text-tertiary">Referral timeline</div>
            {referrals.length ? (
              <div className="space-y-3">
                {referrals.map((referral) => (
                  <div key={`${referral.invite_code}-${referral.registered_at}`} className="panel-muted flex flex-wrap items-center justify-between gap-4 p-5">
                    <div>
                      <div className="text-white">{referral.invite_code}</div>
                      <div className="mt-1 text-sm text-outline">Registered {formatDate(referral.registered_at)}</div>
                    </div>
                    <div className="text-right text-sm text-outline">
                      <div>Activated: {formatDate(referral.activated_at)}</div>
                      <div>Reward granted: {formatDate(referral.reward_granted_at)}</div>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyState title="No referrals captured" body={`Invite registrations appear here after a new user completes the Google login flow through your invite link. Each account can capture up to ${formatNumber(inviteLimit)} invited users.`} />
            )}
          </div>
        </Panel>

        <Panel>
          <SectionHeading eyebrow="Discord status" title="Link Discord for growth rewards" body="The connect API is live, but this build still blocks guild verification until a trusted Discord membership checker is configured." />
          {discordConnection ? (
            <div className="panel-muted p-5">
              <div className="flex items-center justify-between gap-4">
                <div>
                  <div className="text-lg font-semibold text-white">{discordConnection.username}</div>
                  <div className="mt-1 text-sm text-outline">Connected {formatDate(discordConnection.connected_at)}</div>
                </div>
                <StatusPill value={discordConnection.verification_status} />
              </div>
              <div className="mt-4 text-sm text-outline">
                {discordConnection.verification_blocked_reason ?? 'Guild membership verified.'}
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <div className="panel-muted p-4">
                  <div className="info-eyebrow-tight text-outline">Guild member</div>
                  <div className="mt-2 text-2xl font-bold text-white">{discordConnection.guild_member ? 'YES' : 'NO'}</div>
                  <div className="mt-1 text-xs text-outline">Trusted backend verification only</div>
                </div>
                <div className="panel-muted p-4">
                  <div className="info-eyebrow-tight text-outline">Reward granted</div>
                  <div className="mt-2 text-2xl font-bold text-white">{discordConnection.reward_granted_at ? 'YES' : 'NO'}</div>
                  <div className="mt-1 text-xs text-outline">Updated {formatDate(discordConnection.reward_granted_at)}</div>
                </div>
              </div>
            </div>
          ) : (
            <EmptyState title="Discord not linked" body="Add the Discord user ID and username from your community profile to create the first connection record." />
          )}

          <div className="panel-muted mt-6 grid gap-4 p-5">
            <div className="rounded-2xl border border-outline-variant/20 bg-surface-container-low/60 p-4 text-sm text-outline">
              OAuth-based Discord linking is now the primary path. With empty production config, the backend will keep redirecting back with an explicit blocker instead of pretending guild verification succeeded.
            </div>
            <a className="tonal-button w-full justify-center" href={discordLoginHref}>
              Continue with Discord OAuth
            </a>
          </div>
        </Panel>
      </div>
    </div>
  )
}
