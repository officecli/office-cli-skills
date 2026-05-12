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
  if (pack.pack_kind === 'hosted_credits') {
    return `${pack.credit_amount ?? 0} credits`
  }
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: pack.currency.toUpperCase(),
    minimumFractionDigits: 2,
  }).format(pack.amount_total / 100)
}

export function formatAuxiliaryPrice(pack: PricingPack) {
  if (pack.pack_kind !== 'hosted_credits') {
    return ''
  }
  return `≈ ${new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: pack.currency.toUpperCase(),
    minimumFractionDigits: 2,
  }).format(pack.amount_total / 100)} USD at 100 credits = $1 USD`
}

function packPrimaryAmount(pack: PricingPack) {
  return pack.pack_kind === 'hosted_credits' ? (pack.credit_amount ?? 0) : pack.quota_amount
}
