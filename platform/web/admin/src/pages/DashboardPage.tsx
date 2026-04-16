import { useQuery } from '@tanstack/react-query'
import { Activity, ShieldAlert, Waypoints } from 'lucide-react'
import { api } from '../api'
import { MetricCard, Panel, SectionHeading, formatNumber } from '../components/ui'

const guardrails = [
  'Admin sessions are issued only after Google auth plus an exact allowlist match.',
  'Free quota edits should be used for abuse control and onboarding exceptions only.',
  'Usage event review is the fastest way to verify blocked traffic before touching billing-backed keys.',
]

export default function DashboardPage() {
  const { data: overview } = useQuery({ queryKey: ['admin-overview'], queryFn: api.overview })

  return (
    <div className="space-y-8">
      <Panel className="relative overflow-hidden">
        <div className="overview-hero-glow absolute inset-y-0 right-0 hidden w-1/2 lg:block" />
        <SectionHeading
          eyebrow="Platform overview"
          title="Governance metrics in one pass"
          body="Review key posture, free-machine pressure, and blocked traffic before taking any action on the platform."
        />
        <div className="grid gap-4 xl:grid-cols-4">
          <MetricCard label="Total API Keys" value={formatNumber(overview?.total_api_keys)} detail="All managed credentials across the platform" />
          <MetricCard label="Active Keys" value={formatNumber(overview?.active_api_keys)} detail="Keys currently allowed to process traffic" />
          <MetricCard label="Free Machines" value={formatNumber(overview?.free_machines)} detail="Fingerprints still operating inside the free tier" tone="warning" />
          <MetricCard label="Blocked 24H" value={formatNumber(overview?.blocked_last_24h)} detail="Requests denied by policy in the last 24 hours" tone="critical" />
        </div>
      </Panel>

      <div className="dashboard-shell">
        <Panel>
          <SectionHeading eyebrow="Traffic pressure" title="Policy-facing activity snapshot" body="These counters help operators decide whether the platform is seeing healthy document traffic or something that needs intervention." />
          <div className="grid gap-4 md:grid-cols-3">
            <MetricCard label="Checks 24H" value={formatNumber(overview?.checks_last_24h)} detail="Requests that reached policy evaluation" />
            <MetricCard label="Consumes 24H" value={formatNumber(overview?.consumes_last_24h)} detail="Requests that consumed free/reward/paid quota or hosted credits" />
            <MetricCard label="Expired Keys" value={formatNumber(overview?.expired_api_keys)} detail="Credentials that need attention or archival" />
          </div>
        </Panel>

        <Panel>
          <SectionHeading eyebrow="Operator brief" title="Current stance" />
          <div className="space-y-4">
            <div className="panel-muted flex items-start gap-3 p-4"><ShieldAlert size={18} className="mt-0.5 text-tertiary" /><p className="text-sm text-outline">Use API key edits for metadata, status flips, and corrective quota work. Avoid treating the admin plane as a billing replacement.</p></div>
            <div className="panel-muted flex items-start gap-3 p-4"><Activity size={18} className="mt-0.5 text-primary" /><p className="text-sm text-outline">Blocked traffic spikes should be reviewed in Usage Events before relaxing any quota control.</p></div>
            <div className="panel-muted flex items-start gap-3 p-4"><Waypoints size={18} className="mt-0.5 text-secondary" /><p className="text-sm text-outline">All admin interactions assume an English-first operator experience and OfficeCLI brand naming.</p></div>
          </div>
        </Panel>
      </div>

      <Panel>
        <SectionHeading eyebrow="Guardrails" title="What operators should verify before changing state" />
        <div className="grid gap-4 md:grid-cols-3">
          {guardrails.map((item) => (
            <div key={item} className="panel-muted p-5 text-sm text-outline">{item}</div>
          ))}
        </div>
      </Panel>
    </div>
  )
}
