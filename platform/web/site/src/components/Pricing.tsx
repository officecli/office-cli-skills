import { useEffect, useMemo, useState } from 'react'
import { motion } from 'motion/react'
import { Check } from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import { fetchPricing, formatPrice, type PricingPack } from '../siteApi'
import { platformBillingURL } from '../siteData'

const fallbackPacks: PricingPack[] = [
  {
    code: 'external-100',
    name: 'External 100',
    description: '100 external generations for lightweight evaluation and individual workflows.',
    currency: 'usd',
    amount_total: 990,
    quota_amount: 100,
  },
  {
    code: 'external-500',
    name: 'External 500',
    description: '500 external generations for shared team workflows and recurring automation.',
    currency: 'usd',
    amount_total: 4490,
    quota_amount: 500,
  },
]

interface PricingProps {
  standalone?: boolean
}

export default function Pricing({ standalone = false }: PricingProps) {
  const [packs, setPacks] = useState<PricingPack[]>(fallbackPacks)
  const location = useLocation()

  useEffect(() => {
    let cancelled = false

    fetchPricing()
      .then((data) => {
        if (!cancelled && data.length > 0) {
          setPacks(data)
        }
      })
      .catch(() => {
        // Fallback pricing keeps the landing page usable when the API is unavailable.
      })

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    trackEvent(SITE_ANALYTICS_EVENTS.pricingView, {
      surface: 'site',
      placement: standalone ? 'pricing-page' : 'home-pricing',
    })
  }, [standalone])

  const starterPack = packs[0] ?? fallbackPacks[0]
  const productionPack = packs[1] ?? packs[0] ?? fallbackPacks[1]
  const platformBillingHref = buildTrackedURL(platformBillingURL, location.search)
  const titleClass = 'font-headline text-5xl md:text-6xl font-bold text-white mb-8 tracking-tighter'

  const starterItems = useMemo(() => [
    `${starterPack.quota_amount} external generations`,
    'Entry pack for evaluation',
    'CLI access',
  ], [starterPack.quota_amount])

  const productionItems = useMemo(() => [
    'Best value for recurring usage',
    'Shared team automation',
    'API webhook support',
  ], [])

  return (
    <section className={`px-8 md:px-16 max-w-[1440px] mx-auto text-center ${standalone ? 'pt-8 pb-24' : 'py-32'}`}>
      <div className="max-w-3xl mx-auto">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Pricing Architecture</span>
        <h2 className={titleClass}>Simple Kinetic Billing</h2>
        <p className="text-xl text-outline-variant mb-12">Start with a small pack, then scale external document generation with a discounted team bundle.</p>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-8 text-left max-w-4xl mx-auto mb-16">
          <motion.div
            whileHover={{ y: -5 }}
            className="p-10 bg-surface-low rounded-2xl border border-outline-variant/10"
          >
            <h3 className="font-headline text-2xl font-bold text-white mb-2">{starterPack.name}</h3>
            <div className="text-4xl font-headline font-black text-primary mb-6">{formatPrice(starterPack)} <span className="text-sm font-normal text-outline-variant">/ {starterPack.quota_amount} generations</span></div>
            <ul className="space-y-4 mb-10 text-outline-variant text-sm">
              {starterItems.map((item) => (
                <li key={item} className="flex gap-3"><Check className="text-tertiary w-5 h-5" /> {item}</li>
              ))}
            </ul>
            <Link
              className="w-full py-4 rounded-md border border-outline-variant/30 font-bold hover:bg-white/5 transition-all inline-flex items-center justify-center"
              to="/download"
            >
              Download CLI
            </Link>
          </motion.div>

          <motion.div
            whileHover={{ y: -5 }}
            className="p-10 bg-surface-high rounded-2xl border-2 border-primary shadow-[0_0_40px_rgba(174,198,255,0.1)] relative overflow-hidden"
          >
            <div className="absolute top-5 right-5 bg-primary text-[#002e6b] text-[10px] font-bold px-2 py-1 rounded">POPULAR</div>
            <h3 className="font-headline text-2xl font-bold text-white mb-2">{productionPack.name}</h3>
            <div className="text-4xl font-headline font-black text-primary mb-6">{formatPrice(productionPack)} <span className="text-sm font-normal text-outline-variant">/ {productionPack.quota_amount} generations</span></div>
            <ul className="space-y-4 mb-10 text-outline-variant text-sm">
              <li className="text-outline-variant leading-relaxed">{productionPack.description}</li>
              {productionItems.map((item) => (
                <li key={item} className="flex gap-3"><Check className="text-tertiary w-5 h-5" /> {item}</li>
              ))}
            </ul>
            <div className="mb-4 text-sm text-outline-variant">
              Secure Stripe checkout starts in the billing workspace after sign-in and API key selection.
            </div>
            <motion.a
              whileHover={{ scale: 0.98 }}
              className="w-full py-4 rounded-md bg-gradient-to-br from-primary to-primary-container text-[#002e6b] font-bold transition-all inline-flex items-center justify-center"
              href={platformBillingHref}
              onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.checkoutStart, { surface: 'site', placement: standalone ? 'pricing-page' : 'home-pricing', pack_code: productionPack.code, ...extractAttributionParams(location.search) })}
            >
              Open Secure Checkout
            </motion.a>
          </motion.div>
        </div>

        <div className="bg-surface-low p-8 rounded-xl border border-outline-variant/10 inline-flex flex-col items-center">
          <p className="text-outline-variant text-sm mb-4">Enterprise scale or custom requirements?</p>
          <Link className="text-tertiary font-bold hover:underline" to="/docs">Contact Infrastructure Support</Link>
        </div>
      </div>
    </section>
  )
}
