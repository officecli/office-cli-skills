import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'

export default function GrowthPage() {
  const { data: growth } = useQuery({ queryKey: ['admin-growth'], queryFn: api.growth })

  const rewardRows = (growth?.reward_grants ?? []).map((grant) => {
    const remaining = Math.max(grant.amount_total - grant.amount_used, 0)
    return [
      <span key="user" className="text-white">{grant.user_id}</span>,
      <span key="source" className="text-white">{grant.source_type}</span>,
      <span key="remaining" className="text-white">{formatNumber(remaining)}</span>,
      <code key="key" className="font-mono text-xs text-outline">{grant.idempotency_key}</code>,
      <span key="created">{formatDate(grant.created_at)}</span>,
    ]
  })

  const hostedCreditRows = (growth?.hosted_credit_grants ?? []).map((grant) => [
    <span key="user" className="text-white">{grant.user_id}</span>,
    <span key="key" className="text-white">{grant.api_key_id}</span>,
    <span key="source" className="text-white">{grant.source_type}</span>,
    <span key="credits" className="text-white">{formatNumber(grant.credit_amount)}</span>,
    <code key="idempotency" className="font-mono text-xs text-outline">{grant.idempotency_key}</code>,
    <span key="created">{formatDate(grant.created_at)}</span>,
  ])

  const referralRows = (growth?.referrals ?? []).map((referral) => [
    <span key="inviter" className="text-white">{referral.inviter_user_id}</span>,
    <span key="invited" className="text-white">{referral.invited_user_id}</span>,
    <code key="invite" className="font-mono text-xs text-outline">{referral.invite_code}</code>,
    <span key="activated">{formatDate(referral.activated_at)}</span>,
    <span key="reward">{formatDate(referral.reward_granted_at)}</span>,
  ])

  const discordRows = (growth?.discord_connections ?? []).map((connection) => [
    <span key="user" className="text-white">{connection.user_id}</span>,
    <code key="discord" className="font-mono text-xs text-outline">{connection.discord_user_id}</code>,
    <span key="username" className="text-white">{connection.username}</span>,
    <StatusPill key="guild" value={connection.guild_member ? 'verified' : 'verification_blocked'} />,
    <span key="reward">{formatDate(connection.reward_granted_at)}</span>,
  ])

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading
          eyebrow="Growth operations"
          title="Reward grants, referrals, and Discord connections"
          body="This panel consumes the real `/api/admin/growth` ledger so operations can trace account hosted credit grants, referrals, and Discord records."
        />
        <div className="grid gap-4 md:grid-cols-3">
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-outline">Hosted credit grants</div>
            <div className="mt-3 text-3xl font-bold text-white">{formatNumber(growth?.hosted_credit_grants?.length)}</div>
          </div>
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-outline">Referrals</div>
            <div className="mt-3 text-3xl font-bold text-white">{formatNumber(growth?.referrals.length)}</div>
          </div>
          <div className="panel-muted p-5">
            <div className="info-eyebrow text-outline">Discord connections</div>
            <div className="mt-3 text-3xl font-bold text-white">{formatNumber(growth?.discord_connections.length)}</div>
          </div>
        </div>
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Hosted credits ledger" title="Signup, invite, and Discord credits" body="New users receive signup hosted credits; each activated referral grants 100 hosted credits to the inviter, and each verified Discord connection grants 100 hosted credits." />
        {hostedCreditRows.length ? (
          <DataTable headers={['User', 'API key', 'Source', 'Credits', 'Idempotency key', 'Created']} rows={hostedCreditRows} columns="0.6fr 0.7fr 1fr 0.6fr 1.4fr 1fr" />
        ) : (
          <EmptyState title="No hosted credit grants yet" body="Signup and invite hosted credit grants appear here after users enter the app or referrals activate." />
        )}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Reward ledger" title="Reward grants" body="Remaining is computed from the persisted grant totals and usage counters." />
        {rewardRows.length ? (
          <DataTable headers={['User', 'Source', 'Remaining', 'Idempotency key', 'Created']} rows={rewardRows} columns="0.7fr 1fr 0.7fr 1.4fr 1fr" />
        ) : (
          <EmptyState title="No reward grants yet" body="Reward grant rows will appear here after invite activation or future verified Discord rewards land." />
        )}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Referral ledger" title="Referrals" body="Use this list to track registration, activation, and reward timestamps per invited account." />
        {referralRows.length ? (
          <DataTable headers={['Inviter', 'Invited', 'Invite code', 'Activated', 'Reward granted']} rows={referralRows} columns="0.7fr 0.7fr 1fr 1fr 1fr" />
        ) : (
          <EmptyState title="No referrals yet" body="Referral rows are written after invite-bearing login callbacks complete." />
        )}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Discord ledger" title="Discord connections" body="Guild verification remains explicitly blocked until a trusted Discord membership checker is configured in production." />
        {discordRows.length ? (
          <DataTable headers={['User', 'Discord user ID', 'Username', 'Guild status', 'Reward granted']} rows={discordRows} columns="0.7fr 1.2fr 1fr 1fr 1fr" />
        ) : (
          <EmptyState title="No Discord connections yet" body="Once users link Discord through the app, the records appear here for operations review." />
        )}
      </Panel>
    </div>
  )
}
