export interface PricingPack {
  code: string
  name: string
  description: string
  currency: string
  amount_total: number
  quota_amount: number
}

const pricingDescriptionByCode: Record<string, string> = {
  'starter-100': '100 document credits for lightweight evaluation and individual workflows.',
  'growth-500': '500 document credits for shared team workflows and recurring automation.',
  'scale-2000': '2,000 document credits for high-frequency batch generation and platform traffic.',
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
