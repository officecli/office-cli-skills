import { FormEvent, useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { EmptyState, Panel, SectionHeading, StatusPill, formatNumber } from '../components/ui'
import type { HostedCreditPack, HostedModelPricingConfig, HostedPricingRule } from '../types'

const blankRule: HostedPricingRule = {
  document_profile: 'text',
  provider: 'aigateway',
  model: '',
  text_model_key: 'text_default',
  image_model_key: '',
  prompt_per_1k_cost_microusd: 0,
  output_per_1k_cost_microusd: 0,
  reasoning_per_1k_cost_microusd: 0,
  image_per_asset_cost_microusd: 0,
  reservation_credits: 16,
  minimum_charge_credits: 2,
  enabled: true,
}

const profileOptions = [
  { value: 'text', label: 'Text generation' },
  { value: 'image', label: 'Image generation' },
]

const blankPack: HostedCreditPack = {
  code: 'hosted-300',
  name: 'Hosted 300',
  description: '300 hosted credits for platform-managed generation.',
  currency: 'usd',
  amount_total: 300,
  credit_amount: 300,
  enabled: true,
}

const defaultModelConfigs: HostedModelPricingConfig[] = [
  {
    key: 'text_default',
    kind: 'text',
    provider: 'openai',
    model: 'gpt-4.1',
    prompt_per_1m_cost_microusd: 0,
    output_per_1m_cost_microusd: 0,
    reasoning_per_1m_cost_microusd: 0,
    prompt_per_1m_cost_credits: 0,
    output_per_1m_cost_credits: 0,
    reasoning_per_1m_cost_credits: 0,
    enabled: true,
  },
  {
    key: 'image_default',
    kind: 'image',
    provider: 'openai',
    model: 'gpt-image-2',
    prompt_per_1m_cost_microusd: 0,
    output_per_1m_cost_microusd: 0,
    reasoning_per_1m_cost_microusd: 0,
    prompt_per_1m_cost_credits: 0,
    output_per_1m_cost_credits: 0,
    reasoning_per_1m_cost_credits: 0,
    enabled: true,
  },
]

export default function HostedPricingRulesPage() {
  const queryClient = useQueryClient()
  const { data } = useQuery({ queryKey: ['admin-hosted-billing'], queryFn: api.hostedBilling })
  const [markupPercent, setMarkupPercent] = useState('')
  const [creditsPerUSDDraft, setCreditsPerUSDDraft] = useState('100')
  const [creditRatioUnlocked, setCreditRatioUnlocked] = useState(false)
  const [ruleDraft, setRuleDraft] = useState<HostedPricingRule>(blankRule)
  const [packDraft, setPackDraft] = useState<HostedCreditPack>(blankPack)
  const creditsPerUSD = data?.settings.credits_per_usd || 100

  const modelConfigs = useMemo(() => {
    const byKey = new Map((data?.model_configs ?? []).map((config) => [config.key, config]))
    return defaultModelConfigs.map((config) => modelConfigWithCreditDefaults(byKey.get(config.key) ?? config, creditsPerUSD))
  }, [creditsPerUSD, data?.model_configs])

  const settingsMarkupPercent = useMemo(() => markupPercent || bpsToPercentValue(data?.settings.markup_bps ?? 3000), [data?.settings.markup_bps, markupPercent])
  const textModelConfigs = useMemo(() => modelConfigs.filter((config) => config.kind === 'text'), [modelConfigs])
  const imageModelConfigs = useMemo(() => modelConfigs.filter((config) => config.kind === 'image'), [modelConfigs])
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['admin-hosted-billing'] })
  const updateSettings = useMutation({ mutationFn: api.updateHostedPricingSettings, onSuccess: invalidate })
  const createModelConfig = useMutation({ mutationFn: api.createHostedModelPricingConfig, onSuccess: invalidate })
  const updateModelConfig = useMutation({ mutationFn: ({ id, payload }: { id: number; payload: HostedModelPricingConfig }) => api.updateHostedModelPricingConfig(id, payload), onSuccess: invalidate })
  const createRule = useMutation({ mutationFn: api.createHostedPricingRule, onSuccess: async () => { setRuleDraft(blankRule); await invalidate() } })
  const updateRule = useMutation({ mutationFn: ({ id, payload }: { id: number; payload: HostedPricingRule }) => api.updateHostedPricingRule(id, payload), onSuccess: invalidate })
  const createPack = useMutation({ mutationFn: api.createHostedCreditPack, onSuccess: async () => { setPackDraft(blankPack); await invalidate() } })
  const updatePack = useMutation({ mutationFn: ({ id, payload }: { id: number; payload: HostedCreditPack }) => api.updateHostedCreditPack(id, payload), onSuccess: invalidate })

  useEffect(() => {
    setCreditsPerUSDDraft(String(creditsPerUSD))
    setCreditRatioUnlocked(false)
  }, [creditsPerUSD])

  function handleSettingsSubmit(event: FormEvent) {
    event.preventDefault()
    const nextCreditsPerUSD = normalizeCreditsPerUSD(creditsPerUSDDraft, creditsPerUSD)
    if (nextCreditsPerUSD !== creditsPerUSD && !window.confirm('Changing the hosted credit ratio affects how USD model costs convert into hosted credits. Save this change?')) {
      return
    }
    updateSettings.mutate({ markup_bps: percentToBPS(settingsMarkupPercent), currency: data?.settings.currency || 'usd', credits_per_usd: nextCreditsPerUSD })
  }

  function unlockCreditRatio() {
    if (window.confirm('Unlock credit ratio editing? This affects hosted credit calculations.')) {
      setCreditRatioUnlocked(true)
    }
  }

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading eyebrow="Hosted billing" title="Hosted pricing controls" body="Configure aigateway base cost, margin, and prepaid credit packs without restarting the platform." />
        <form className="admin-card-muted flex flex-wrap items-end gap-4 p-5" onSubmit={handleSettingsSubmit}>
          <label className="w-40 text-sm text-outline">Global markup %
            <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={settingsMarkupPercent} onChange={(event) => setMarkupPercent(event.target.value)} />
          </label>
          <label className="w-44 text-sm text-outline">Credits per USD
            <input type="number" min="1" className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none disabled:cursor-not-allowed disabled:opacity-60 focus:border-primary/40" value={creditsPerUSDDraft} disabled={!creditRatioUnlocked} onChange={(event) => setCreditsPerUSDDraft(event.target.value)} />
          </label>
          <div className="pb-3 text-sm text-outline">Credit ratio: {formatNumber(normalizeCreditsPerUSD(creditsPerUSDDraft, creditsPerUSD))} credits = $1 USD</div>
          <button type="button" className="admin-secondary-button self-end" onClick={unlockCreditRatio} disabled={creditRatioUnlocked}>Unlock credit ratio</button>
          <button type="submit" className="admin-primary-button self-end px-5" disabled={updateSettings.isPending} aria-label="Save pricing settings">Save settings</button>
        </form>
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Aigateway cost" title="Model pricing" body="Configure shared text and image model costs once, then reference them from hosted profiles." />
        <div className="space-y-3">
          {modelConfigs.map((config) => (
            <ModelConfigRow
              key={config.key}
              config={config}
              creditsPerUSD={creditsPerUSD}
              onSave={(payload) => config.id ? updateModelConfig.mutate({ id: config.id, payload }) : createModelConfig.mutate(payload)}
            />
          ))}
        </div>
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Hosted profiles" title="Pricing rules" body="Profiles choose shared model pricing and keep only reservation, minimum charge, and markup policy." />
        <form className="admin-card-muted mb-6 grid gap-4 p-5 md:grid-cols-4" onSubmit={(event: FormEvent) => {
          event.preventDefault()
          createRule.mutate(normalizeRule(ruleDraft))
        }}>
          <ProfileSelect label="Profile" value={ruleDraft.document_profile} onChange={(value) => setRuleDraft({ ...ruleDraft, document_profile: value })} />
          <ModelSelect label="Text model" value={ruleDraft.text_model_key ?? ''} options={textModelConfigs} onChange={(value) => setRuleDraft({ ...ruleDraft, text_model_key: value })} />
          <ModelSelect label="Image model" value={ruleDraft.image_model_key ?? ''} options={imageModelConfigs} onChange={(value) => setRuleDraft({ ...ruleDraft, image_model_key: value })} />
          <NumberField label="Markup % override" value={bpsToPercentValue(ruleDraft.markup_bps)} onChange={(value) => setRuleDraft({ ...ruleDraft, markup_bps: value === '' ? undefined : percentToBPS(value) })} />
          <NumberField label="Reservation credits" value={ruleDraft.reservation_credits} onChange={(value) => setRuleDraft({ ...ruleDraft, reservation_credits: Number(value) })} />
          <NumberField label="Minimum credits" value={ruleDraft.minimum_charge_credits} onChange={(value) => setRuleDraft({ ...ruleDraft, minimum_charge_credits: Number(value) })} />
          <label className="flex items-center gap-2 self-end text-sm text-outline"><input type="checkbox" checked={ruleDraft.enabled} onChange={(event) => setRuleDraft({ ...ruleDraft, enabled: event.target.checked })} /> Enabled</label>
          <button type="submit" className="admin-primary-button self-end" disabled={createRule.isPending}>Create rule</button>
        </form>
        {data?.rules.length ? (
          <div className="space-y-3">
            {data.rules.map((rule) => (
              <RuleRow
                key={rule.id}
                rule={rule}
                textModelConfigs={textModelConfigs}
                imageModelConfigs={imageModelConfigs}
                onSave={(payload) => rule.id && updateRule.mutate({ id: rule.id, payload })}
                onToggle={() => rule.id && updateRule.mutate({ id: rule.id, payload: { ...rule, enabled: !rule.enabled } })}
              />
            ))}
          </div>
        ) : <EmptyState title="No hosted pricing rules" body="Create a rule before enabling hosted public billing." />}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Prepaid packs" title="Hosted credit packs" body="Enabled packs are visible to users and can be purchased through Stripe Checkout." />
        <form className="admin-card-muted mb-6 grid gap-4 p-5 md:grid-cols-4" onSubmit={(event: FormEvent) => {
          event.preventDefault()
          createPack.mutate(normalizePack(packDraft))
        }}>
          <TextField label="Code" value={packDraft.code} onChange={(value) => setPackDraft({ ...packDraft, code: value })} />
          <TextField label="Name" value={packDraft.name} onChange={(value) => setPackDraft({ ...packDraft, name: value })} />
          <NumberField label="Credits" value={packDraft.credit_amount} onChange={(value) => setPackDraft({ ...packDraft, credit_amount: Number(value) })} />
          <div className="self-end text-sm text-outline">{creditUSDLabel(packDraft.credit_amount, creditsPerUSD)}</div>
          <label className="md:col-span-3 text-sm text-outline">Description
            <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={packDraft.description} onChange={(event) => setPackDraft({ ...packDraft, description: event.target.value })} />
          </label>
          <button type="submit" className="admin-primary-button self-end" disabled={createPack.isPending}>Create pack</button>
        </form>
        {data?.packs.length ? (
          <div className="space-y-3">
            {data.packs.map((pack) => (
              <PackRow
                key={pack.id}
                pack={pack}
                creditsPerUSD={creditsPerUSD}
                onSave={(payload) => pack.id && updatePack.mutate({ id: pack.id, payload })}
                onToggle={() => pack.id && updatePack.mutate({ id: pack.id, payload: { ...pack, enabled: !pack.enabled } })}
              />
            ))}
          </div>
        ) : <EmptyState title="No hosted credit packs" body="Create an enabled pack to expose hosted billing to users." />}
      </Panel>
    </div>
  )
}

function ModelConfigRow({ config, creditsPerUSD, onSave }: { config: HostedModelPricingConfig; creditsPerUSD: number; onSave: (payload: HostedModelPricingConfig) => void }) {
  const [draft, setDraft] = useState(config)
  const [costDraft, setCostDraft] = useState(modelCostDraftFromConfig(config))

  useEffect(() => {
    setDraft(config)
    setCostDraft(modelCostDraftFromConfig(config))
  }, [config])

  return (
    <div className="admin-card-muted grid gap-4 p-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-3 text-white">{config.key} / {config.model}<StatusPill value={config.enabled ? 'active' : 'disabled'} /></div>
          <div className="mt-1 text-sm text-outline">{config.kind} model; prompt ${microusdToUSDValue(config.prompt_per_1m_cost_microusd)} / output ${microusdToUSDValue(config.output_per_1m_cost_microusd)} / reasoning ${microusdToUSDValue(config.reasoning_per_1m_cost_microusd)} USD per 1M tokens</div>
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-4">
        <TextField label={`Provider ${config.key}`} value={draft.provider} onChange={(value) => setDraft({ ...draft, provider: value })} />
        <TextField label={`Model ${config.key}`} value={draft.model} onChange={(value) => setDraft({ ...draft, model: value })} />
        <NumberField label={`Prompt USD per 1M ${config.key}`} value={costDraft.prompt} type="text" inputMode="decimal" onChange={(value) => setCostDraft({ ...costDraft, prompt: value })} />
        <NumberField label={`Output USD per 1M ${config.key}`} value={costDraft.output} type="text" inputMode="decimal" onChange={(value) => setCostDraft({ ...costDraft, output: value })} />
        <NumberField label={`Reasoning USD per 1M ${config.key}`} value={costDraft.reasoning} type="text" inputMode="decimal" onChange={(value) => setCostDraft({ ...costDraft, reasoning: value })} />
        <label className="flex items-center gap-2 self-end text-sm text-outline"><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} /> Enabled</label>
        <button type="button" className="admin-primary-button self-end" aria-label={`Save hosted model pricing config ${config.key}`} onClick={() => onSave(normalizeModelConfig(draft, costDraft, creditsPerUSD))}>Save model</button>
      </div>
    </div>
  )
}

function RuleRow({ rule, textModelConfigs, imageModelConfigs, onSave, onToggle }: { rule: HostedPricingRule; textModelConfigs: HostedModelPricingConfig[]; imageModelConfigs: HostedModelPricingConfig[]; onSave: (payload: HostedPricingRule) => void; onToggle: () => void }) {
  const [draft, setDraft] = useState(rule)

  useEffect(() => {
    setDraft(rule)
  }, [rule])

  return (
    <div className="admin-card-muted grid gap-4 p-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-3 text-white">{profileLabel(rule.document_profile)} / text {rule.text_model_key || '-'} / image {rule.image_model_key || '-'}<StatusPill value={rule.enabled ? 'active' : 'disabled'} /></div>
          <div className="mt-1 text-sm text-outline">Reservation {rule.reservation_credits} credits; minimum {rule.minimum_charge_credits} credits; markup {markupLabel(rule.markup_bps)}</div>
        </div>
        <button type="button" className="admin-secondary-button" onClick={onToggle}>{rule.enabled ? 'Disable' : 'Enable'}</button>
      </div>
      <div className="grid gap-4 md:grid-cols-4">
        <ProfileSelect label={`Profile ${rule.id ?? rule.document_profile}`} value={draft.document_profile} onChange={(value) => setDraft({ ...draft, document_profile: value })} />
        <ModelSelect label={`Text model ${rule.id ?? rule.document_profile}`} value={draft.text_model_key ?? ''} options={textModelConfigs} onChange={(value) => setDraft({ ...draft, text_model_key: value })} />
        <ModelSelect label={`Image model ${rule.id ?? rule.document_profile}`} value={draft.image_model_key ?? ''} options={imageModelConfigs} onChange={(value) => setDraft({ ...draft, image_model_key: value })} />
        <NumberField label={`Markup % override ${rule.id ?? rule.document_profile}`} value={bpsToPercentValue(draft.markup_bps)} onChange={(value) => setDraft({ ...draft, markup_bps: value === '' ? undefined : percentToBPS(value) })} />
        <NumberField label={`Reservation ${rule.id ?? rule.document_profile}`} value={draft.reservation_credits} onChange={(value) => setDraft({ ...draft, reservation_credits: Number(value) })} />
        <NumberField label={`Minimum ${rule.id ?? rule.document_profile}`} value={draft.minimum_charge_credits} onChange={(value) => setDraft({ ...draft, minimum_charge_credits: Number(value) })} />
        <label className="flex items-center gap-2 self-end text-sm text-outline"><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} /> Enabled</label>
        <button type="button" className="admin-primary-button self-end" aria-label={`Save hosted pricing rule ${rule.id ?? rule.document_profile}`} onClick={() => onSave(normalizeRule(draft))}>Save rule</button>
      </div>
    </div>
  )
}

function PackRow({ pack, creditsPerUSD, onSave, onToggle }: { pack: HostedCreditPack; creditsPerUSD: number; onSave: (payload: HostedCreditPack) => void; onToggle: () => void }) {
  const [draft, setDraft] = useState(pack)

  useEffect(() => {
    setDraft(pack)
  }, [pack])

  return (
    <div className="admin-card-muted grid gap-4 p-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div className="text-white">{pack.name}</div>
          <div className="mt-1 text-sm text-outline">{pack.code} / {formatNumber(pack.credit_amount)} credits / {creditUSDLabel(pack.credit_amount, creditsPerUSD)}</div>
        </div>
        <button type="button" className="admin-secondary-button" onClick={onToggle}>{pack.enabled ? 'Disable' : 'Enable'}</button>
      </div>
      <div className="grid gap-4 md:grid-cols-4">
        <TextField label={`Pack code ${pack.id ?? pack.code}`} value={draft.code} onChange={(value) => setDraft({ ...draft, code: value })} />
        <TextField label={`Pack name ${pack.id ?? pack.code}`} value={draft.name} onChange={(value) => setDraft({ ...draft, name: value })} />
        <NumberField label={`Pack credits ${pack.id ?? pack.code}`} value={draft.credit_amount} onChange={(value) => setDraft({ ...draft, credit_amount: Number(value) })} />
        <div className="self-end text-sm text-outline">{creditUSDLabel(draft.credit_amount, creditsPerUSD)}</div>
        <label className="md:col-span-3 text-sm text-outline">Pack description {pack.id ?? pack.code}
          <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} />
        </label>
        <button type="button" className="admin-primary-button self-end" aria-label={`Save hosted credit pack ${pack.id ?? pack.code}`} onClick={() => onSave(normalizePack(draft))}>Save pack</button>
      </div>
    </div>
  )
}

function TextField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return <label className="text-sm text-outline">{label}<input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={value} onChange={(event) => onChange(event.target.value)} /></label>
}

function ProfileSelect({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label className="text-sm text-outline">{label}
      <select className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={value} onChange={(event) => onChange(event.target.value)}>
        {profileOptions.map((option) => (
          <option key={option.value} value={option.value}>{option.label}</option>
        ))}
      </select>
    </label>
  )
}

function ModelSelect({ label, value, options, onChange }: { label: string; value: string; options: HostedModelPricingConfig[]; onChange: (value: string) => void }) {
  return (
    <label className="text-sm text-outline">{label}
      <select className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="">None</option>
        {options.map((option) => (
          <option key={option.key} value={option.key}>{option.key} / {option.model}</option>
        ))}
      </select>
    </label>
  )
}

function profileLabel(value: string) {
  return profileOptions.find((option) => option.value === value)?.label ?? value
}

function NumberField({ label, value, type = 'number', inputMode, step, onChange }: { label: string; value: number | string; type?: 'number' | 'text'; inputMode?: 'decimal'; step?: string; onChange: (value: string) => void }) {
  return <label className="text-sm text-outline">{label}<input type={type} inputMode={inputMode} step={step} className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={value} onChange={(event) => onChange(event.target.value)} /></label>
}

interface ModelCostDraft {
  prompt: string
  output: string
  reasoning: string
}

function modelCostDraftFromConfig(config: HostedModelPricingConfig): ModelCostDraft {
  return {
    prompt: microusdToUSDValue(config.prompt_per_1m_cost_microusd),
    output: microusdToUSDValue(config.output_per_1m_cost_microusd),
    reasoning: microusdToUSDValue(config.reasoning_per_1m_cost_microusd),
  }
}

function normalizeModelConfig(config: HostedModelPricingConfig, costDraft: ModelCostDraft, creditsPerUSD: number): HostedModelPricingConfig {
  const promptMicrousd = usdToMicrousd(costDraft.prompt)
  const outputMicrousd = usdToMicrousd(costDraft.output)
  const reasoningMicrousd = usdToMicrousd(costDraft.reasoning)
  return {
    ...config,
    prompt_per_1m_cost_microusd: promptMicrousd,
    output_per_1m_cost_microusd: outputMicrousd,
    reasoning_per_1m_cost_microusd: reasoningMicrousd,
    prompt_per_1m_cost_credits: creditsFromMicrousd(promptMicrousd, creditsPerUSD),
    output_per_1m_cost_credits: creditsFromMicrousd(outputMicrousd, creditsPerUSD),
    reasoning_per_1m_cost_credits: creditsFromMicrousd(reasoningMicrousd, creditsPerUSD),
  }
}

function normalizeRule(rule: HostedPricingRule): HostedPricingRule {
  return {
    ...rule,
    provider: rule.provider || '',
    model: rule.model || '',
    text_model_key: rule.text_model_key || '',
    image_model_key: rule.image_model_key || '',
    markup_bps: rule.markup_bps === undefined ? undefined : Number(rule.markup_bps),
  }
}

function normalizePack(pack: HostedCreditPack): HostedCreditPack {
  return { ...pack, currency: pack.currency || 'usd', amount_total: Number(pack.amount_total), credit_amount: Number(pack.credit_amount) }
}

function modelConfigWithCreditDefaults(config: HostedModelPricingConfig, creditsPerUSD: number): HostedModelPricingConfig {
  return {
    ...config,
    prompt_per_1m_cost_credits: config.prompt_per_1m_cost_credits ?? creditsFromMicrousd(config.prompt_per_1m_cost_microusd, creditsPerUSD),
    output_per_1m_cost_credits: config.output_per_1m_cost_credits ?? creditsFromMicrousd(config.output_per_1m_cost_microusd, creditsPerUSD),
    reasoning_per_1m_cost_credits: config.reasoning_per_1m_cost_credits ?? creditsFromMicrousd(config.reasoning_per_1m_cost_microusd, creditsPerUSD),
  }
}

function creditsFromMicrousd(microusd: number, creditsPerUSD: number) {
  if (!microusd || microusd <= 0) return 0
  return Math.ceil((microusd * (creditsPerUSD || 100)) / 1_000_000)
}

function microusdToUSDValue(microusd: number) {
  return (Number(microusd || 0) / 1_000_000).toFixed(2)
}

function usdToMicrousd(value: string | number) {
  return Math.round(Number(value || 0) * 1_000_000)
}

function normalizeCreditsPerUSD(value: string | number, fallback: number) {
  const normalized = Math.round(Number(value))
  return normalized > 0 ? normalized : fallback || 100
}

function bpsToPercentValue(bps?: number) {
  if (bps === undefined || bps === null) return ''
  return String(bps / 100)
}

function percentToBPS(value: string | number) {
  return Math.round(Number(value) * 100)
}

function markupLabel(bps?: number) {
  return bps === undefined || bps === null ? 'global' : `${bps / 100}%`
}

function creditUSDLabel(credits: number, creditsPerUSD: number) {
  const usd = Number(credits || 0) / (creditsPerUSD || 100)
  return `≈ $${usd.toFixed(2)} USD`
}
