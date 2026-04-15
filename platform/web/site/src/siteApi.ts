export interface PricingPack {
  code: string
  name: string
  description: string
  currency: string
  amount_total: number
  quota_amount: number
}

export async function fetchPricing(): Promise<PricingPack[]> {
  const response = await fetch('/api/pricing')
  if (!response.ok) {
    throw new Error(`pricing request failed: ${response.status}`)
  }

  const payload = await response.json() as { data?: PricingPack[] } | PricingPack[]
  const packs = Array.isArray(payload) ? payload : (payload.data ?? [])
  return packs.sort((left, right) => left.quota_amount - right.quota_amount)
}

export function formatPrice(pack: PricingPack) {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: pack.currency.toUpperCase(),
    minimumFractionDigits: 2,
  }).format(pack.amount_total / 100)
}
