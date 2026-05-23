import { useEffect, useMemo, useState } from 'react'
import { motion } from 'motion/react'
import { Check } from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import { fetchPricing, formatAuxiliaryPrice, formatPrice, type PricingPack } from '../siteApi'
import { platformBillingURL } from '../siteData'

interface PricingProps {
  standalone?: boolean
}

export default function Pricing({ standalone = false }: PricingProps) {
  const [packs, setPacks] = useState<PricingPack[]>([])
  const [pricingUnavailable, setPricingUnavailable] = useState(false)
  const location = useLocation()

  useEffect(() => {
    let cancelled = false

    fetchPricing()
      .then((data) => {
        if (!cancelled) {
          setPacks(data)
          setPricingUnavailable(data.length === 0)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setPricingUnavailable(true)
        }
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

  const starterPack = packs[0]
  const productionPack = packs[1] ?? packs[0]
  const bulkPack = packs.length >= 3 ? packs[packs.length - 1] : undefined
  const platformBillingHref = buildTrackedURL(platformBillingURL, location.search)
  const titleClass = 'font-headline text-5xl md:text-6xl font-bold text-white mb-8 tracking-tighter'

  const starterItems = useMemo(() => [
    packUnitItem(starterPack),
    'Entry pack for evaluation',
    'External Mode remains free and unlimited',
  ], [starterPack])

  const productionItems = useMemo(() => [
    'Best value for recurring usage',
    'Shared team automation',
    'API-key based workflows',
  ], [])

  const bulkItems = useMemo(() => bulkPack ? [
    packUnitItem(bulkPack),
    'Largest capacity in a single purchase',
    'Stretch credits across long-running automations',
  ] : [], [bulkPack])

  return (
    <section id="pricing" className={`px-8 md:px-16 max-w-[1440px] mx-auto text-center ${standalone ? 'pt-8 pb-24' : 'py-32'}`}>
      <div className="max-w-3xl mx-auto">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Pricing</span>
        <h2 className={titleClass}>Choose External or Hosted Mode</h2>
        <p className="text-xl text-outline-variant mb-12">External Mode is free and unlimited with your own model endpoint. Hosted Mode uses hosted credits for the OfficeCLI-managed runtime.</p>

        {packs.length > 0 && starterPack && productionPack ? (
          <div className={`grid grid-cols-1 ${bulkPack ? 'md:grid-cols-3 max-w-6xl' : 'md:grid-cols-2 max-w-4xl'} gap-8 text-left mx-auto mb-16`}>
            <motion.div
              whileHover={{ y: -5 }}
              className="p-10 bg-surface-low rounded-2xl border border-outline-variant/10"
            >
              <h3 className="font-headline text-2xl font-bold text-white mb-2">{starterPack.name}</h3>
              <div className="text-4xl font-headline font-black text-primary mb-2">{formatPrice(starterPack)}</div>
              <div className="mb-6 text-sm font-normal text-outline-variant">{priceMeta(starterPack)}</div>
              <ul className="space-y-4 mb-10 text-outline-variant text-sm">
                {starterItems.map((item) => (
                  <li key={item} className="flex gap-3"><Check className="text-tertiary w-5 h-5" /> {item}</li>
                ))}
              </ul>
              <Link
                className="w-full py-4 rounded-md border border-outline-variant/30 font-bold hover:bg-white/5 transition-all inline-flex items-center justify-center"
                to="/download"
              >
                Install the CLI
              </Link>
            </motion.div>

            <motion.div
              whileHover={{ y: -5 }}
              className="p-10 bg-surface-high rounded-2xl border-2 border-primary shadow-[0_0_40px_rgba(174,198,255,0.1)] relative overflow-hidden"
            >
              <div className="absolute top-5 right-5 bg-primary text-[#002e6b] text-[10px] font-bold px-2 py-1 rounded">POPULAR</div>
              <h3 className="font-headline text-2xl font-bold text-white mb-2">{productionPack.name}</h3>
              <div className="text-4xl font-headline font-black text-primary mb-2">{formatPrice(productionPack)}</div>
              <div className="mb-6 text-sm font-normal text-outline-variant">{priceMeta(productionPack)}</div>
              <ul className="space-y-4 mb-10 text-outline-variant text-sm">
                <li className="text-outline-variant leading-relaxed">{productionPack.description}</li>
                {productionItems.map((item) => (
                  <li key={item} className="flex gap-3"><Check className="text-tertiary w-5 h-5" /> {item}</li>
                ))}
              </ul>
              <div className="mb-4 text-sm text-outline-variant">
                Secure Stripe checkout starts in the billing workspace after sign-in and account selection.
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

            {bulkPack ? (
              <motion.div
                whileHover={{ y: -5 }}
                className="p-10 bg-surface-low rounded-2xl border border-outline-variant/10"
              >
                <h3 className="font-headline text-2xl font-bold text-white mb-2">{bulkPack.name}</h3>
                <div className="text-4xl font-headline font-black text-primary mb-2">{formatPrice(bulkPack)}</div>
                <div className="mb-6 text-sm font-normal text-outline-variant">{priceMeta(bulkPack)}</div>
                <ul className="space-y-4 mb-10 text-outline-variant text-sm">
                  <li className="text-outline-variant leading-relaxed">{bulkPack.description}</li>
                  {bulkItems.map((item) => (
                    <li key={item} className="flex gap-3"><Check className="text-tertiary w-5 h-5" /> {item}</li>
                  ))}
                </ul>
                <motion.a
                  whileHover={{ scale: 0.98 }}
                  className="w-full py-4 rounded-md border border-outline-variant/30 font-bold hover:bg-white/5 transition-all inline-flex items-center justify-center"
                  href={platformBillingHref}
                  onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.checkoutStart, { surface: 'site', placement: standalone ? 'pricing-page' : 'home-pricing', pack_code: bulkPack.code, ...extractAttributionParams(location.search) })}
                >
                  Open Secure Checkout
                </motion.a>
              </motion.div>
            ) : null}
          </div>
        ) : pricingUnavailable ? (
          <div className="max-w-3xl mx-auto mb-16 rounded-2xl border border-outline-variant/10 bg-surface-low p-10 text-left">
            <div className="text-xs font-headline uppercase tracking-widest text-primary">Pricing temporarily unavailable</div>
            <h3 className="mt-4 font-headline text-3xl font-bold text-white">Live pricing is currently unavailable</h3>
            <p className="mt-4 text-base leading-relaxed text-outline-variant">
              Pricing is served from the platform API only. Retry in a moment, or open the billing workspace after sign-in to view the current pack totals.
            </p>
            <div className="mt-8 flex flex-col gap-4 sm:flex-row">
              <motion.a
                whileHover={{ scale: 0.98 }}
                className="inline-flex items-center justify-center rounded-md bg-gradient-to-br from-primary to-primary-container px-6 py-4 font-bold text-[#002e6b] transition-all"
                href={platformBillingHref}
                onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.checkoutStart, { surface: 'site', placement: standalone ? 'pricing-page-unavailable' : 'home-pricing-unavailable', pack_code: 'unavailable', ...extractAttributionParams(location.search) })}
              >
                Open Billing Workspace
              </motion.a>
              <Link
                className="inline-flex items-center justify-center rounded-md border border-outline-variant/30 px-6 py-4 font-bold transition-all hover:bg-white/5"
                to="/download"
              >
                Install the CLI
              </Link>
            </div>
          </div>
        ) : null}

        <div className="bg-surface-low p-8 rounded-xl border border-outline-variant/10 inline-flex flex-col items-center">
          <p className="text-outline-variant text-sm mb-4">Enterprise scale or custom requirements?</p>
          <a className="text-tertiary font-bold hover:underline" href="mailto:support@officecli.io">Contact Us</a>
        </div>
      </div>
    </section>
  )
}

function packUnitItem(pack?: PricingPack) {
  if (!pack) return '0 hosted credits'
  if (pack.pack_kind === 'hosted_credits') {
    return `${pack.credit_amount ?? 0} hosted credits`
  }
  return `${pack.quota_amount ?? 0} legacy document generations`
}

function priceMeta(pack: PricingPack) {
  if (pack.pack_kind === 'hosted_credits') {
    return formatAuxiliaryPrice(pack)
  }
  return `/ ${pack.quota_amount} generations`
}
