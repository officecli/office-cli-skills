export interface PricingPack {
  code: string
  name: string
  description: string
  currency: string
  amount_total: number
  quota_amount: number
  credit_amount?: number
  pack_kind?: string
}

export async function fetchPricing(): Promise<PricingPack[]> {
  const response = await fetch('/api/pricing')
  if (!response.ok) {
    throw new Error(`pricing request failed: ${response.status}`)
  }

  const payload = await response.json() as { data?: PricingPack[] } | PricingPack[]
  const packs = Array.isArray(payload) ? payload : (payload.data ?? [])
  return packs.sort((left, right) => packPrimaryAmount(left) - packPrimaryAmount(right))
}

export function formatPrice(pack: PricingPack) {
  const dollars = pack.amount_total / 100
  const fractionDigits = Number.isInteger(dollars) ? 0 : 2
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: pack.currency.toUpperCase(),
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(dollars)
}

export function formatCreditCount(pack: PricingPack): string {
  if (pack.pack_kind !== 'hosted_credits') return ''
  return `${(pack.credit_amount ?? 0).toLocaleString('en-US')} credits`
}

export function pricePerCreditLabel(pack: PricingPack): string {
  const credits = pack.credit_amount ?? 0
  if (credits <= 0) return ''
  const perCredit = pack.amount_total / 100 / credits
  return `${new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: pack.currency.toUpperCase(),
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(perCredit)} / credit`
}

export function imagesPerPack(pack: PricingPack, creditsPerImage = 10): string {
  const credits = pack.credit_amount ?? 0
  if (credits <= 0) return ''
  return `≈ ${Math.floor(credits / creditsPerImage)} images @ ${creditsPerImage} credits each`
}

export function bestValueCode(packs: PricingPack[]): string | undefined {
  let best: { code: string; rate: number; credits: number } | undefined
  for (const p of packs) {
    if (p.pack_kind !== 'hosted_credits') continue
    const credits = p.credit_amount ?? 0
    if (credits <= 0) continue
    const rate = p.amount_total / credits
    if (!best || rate < best.rate || (rate === best.rate && credits > best.credits)) {
      best = { code: p.code, rate, credits }
    }
  }
  return best?.code
}

function packPrimaryAmount(pack: PricingPack) {
  return pack.pack_kind === 'hosted_credits' ? (pack.credit_amount ?? 0) : pack.quota_amount
}
