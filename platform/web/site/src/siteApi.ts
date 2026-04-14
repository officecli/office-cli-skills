export interface PricingPack {
  code: string
  name: string
  description: string
  currency: string
  amount_total: number
  quota_amount: number
}

const pricingDescriptionByCode: Record<string, string> = {
  'external-100': '100 paid document operations for lightweight evaluation and individual workflows.',
  'external-500': '500 paid document operations for shared team workflows and recurring automation.',
}

export async function fetchPricing(): Promise<PricingPack[]> {
  const response = await fetch('/api/pricing')
  if (!response.ok) {
    throw new Error(`pricing request failed: ${response.status}`)
  }

  const payload = await response.json() as { data?: PricingPack[] } | PricingPack[]
  const packs = Array.isArray(payload) ? payload : (payload.data ?? [])
  return packs.map(normalizePricingPack).sort((left, right) => left.quota_amount - right.quota_amount)
}

export function formatPrice(pack: PricingPack) {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: pack.currency.toUpperCase(),
    minimumFractionDigits: 2,
  }).format(pack.amount_total / 100)
}

function normalizePricingPack(pack: PricingPack): PricingPack {
  const fallbackDescription = pricingDescriptionByCode[pack.code]
  const containsChinese = /[\u4e00-\u9fff]/.test(pack.description)

  return {
    ...pack,
    description: containsChinese && fallbackDescription ? fallbackDescription : pack.description,
  }
}
