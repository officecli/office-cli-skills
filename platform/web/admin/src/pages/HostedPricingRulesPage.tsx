import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { EmptyState, Panel, SectionHeading } from '../components/ui'

export default function HostedPricingRulesPage() {
  const { data: rules = [] } = useQuery({ queryKey: ['admin-hosted-pricing-rules'], queryFn: api.hostedPricingRules })

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading
          eyebrow="Hosted billing"
          title="Hosted pricing rules"
          body="Current hosted pricing is config-managed. Update `HOSTED_PRICING_RULES_JSON` or platform defaults, then restart the service to apply changes."
        />

        <div className="panel-muted mb-6 p-5 text-sm text-outline">
          Source of truth is server config, not the admin database. This page is intentionally read-only for now so operator changes do not vanish after a restart.
        </div>

        {rules.length ? (
          <div className="space-y-4">
            {rules.map((rule, index) => (
              <div key={`${rule.document_profile}-${index}`} className="panel-muted grid gap-4 p-5 md:grid-cols-3">
                <ReadField label="Document profile" value={rule.document_profile} />
                <ReadField label="Provider" value={rule.provider} />
                <ReadField label="Model" value={rule.model} />
                <ReadField label="Prompt / 1K" value={rule.prompt_per_1k_credits} />
                <ReadField label="Output / 1K" value={rule.output_per_1k_credits} />
                <ReadField label="Reasoning / 1K" value={rule.reasoning_per_1k_credits} />
                <ReadField label="Image / asset" value={rule.image_per_asset_credits} />
                <ReadField label="Reservation" value={rule.reservation_credits} />
                <ReadField label="Minimum charge" value={rule.minimum_charge_credits} />
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No hosted pricing rules" body="Add default hosted pricing rules in platform config before editing them here." />
        )}
      </Panel>
    </div>
  )
}

function ReadField({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="text-sm text-outline">
      {label}
      <div className="surface-console mt-2 rounded-2xl border border-outline-variant/20 px-4 py-3 text-white">{value}</div>
    </div>
  )
}
