import { useEffect, useState } from 'react'
import { motion } from 'motion/react'
import { Check } from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import { bestValueCode, fetchPricing, formatCreditCount, formatPrice, imagesPerPack, pricePerCreditLabel, type PricingPack } from '../siteApi'
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

  const hostedPacks = packs.filter(p => p.pack_kind === 'hosted_credits').slice(0, 3)
  const bestCode = bestValueCode(packs)
  const titleClass = 'font-headline text-5xl md:text-6xl font-bold text-white mb-8 tracking-tighter'

  return (
    <section id="pricing" className={`px-8 md:px-16 max-w-[1440px] mx-auto text-center ${standalone ? 'pt-8 pb-24' : 'py-32'}`}>
      <div className="max-w-3xl mx-auto">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Pricing</span>
        <h2 className={titleClass}>Choose External or Hosted Mode</h2>
        <p className="text-xl text-outline-variant mb-12">External Mode is free and unlimited with your own model endpoint. Hosted Mode uses hosted credits for the OfficeCLI-managed runtime.</p>

        {hostedPacks.length > 0 ? (
          <div className={`grid grid-cols-1 ${hostedPacks.length >= 3 ? 'md:grid-cols-3 max-w-6xl' : hostedPacks.length === 2 ? 'md:grid-cols-2 max-w-4xl' : 'max-w-lg'} gap-8 text-left mx-auto mb-16`}>
            {hostedPacks.map((pack, idx) => {
              const isPopular = idx === 1 && hostedPacks.length >= 2
              const isBestValue = pack.code === bestCode && !isPopular
              const packHref = buildTrackedURL(`${platformBillingURL}?pack=${encodeURIComponent(pack.code)}&autostart=1`, location.search)
              return (
                <motion.div
                  key={pack.code}
                  whileHover={{ y: -5 }}
                  className={`flex flex-col p-10 rounded-2xl relative overflow-hidden ${isPopular ? 'bg-surface-high border-2 border-primary shadow-[0_0_40px_rgba(174,198,255,0.1)]' : 'bg-surface-low border border-outline-variant/10'}`}
                >
                  {isPopular && (
                    <div className="absolute top-5 right-5 bg-primary text-[#002e6b] text-[10px] font-bold px-2 py-1 rounded">POPULAR</div>
                  )}
                  {isBestValue && (
                    <div className="absolute top-5 right-5 bg-emerald-500/20 border border-emerald-500/40 text-emerald-200 text-[10px] font-bold px-2 py-1 rounded">Best value</div>
                  )}
                  <h3 className="font-headline text-2xl font-bold text-white mb-2">{pack.name}</h3>
                  <div className="text-5xl font-headline font-black text-primary leading-none mb-1">{formatPrice(pack)}</div>
                  <div className="text-base font-headline font-semibold text-white/80 mb-3">{formatCreditCount(pack) || `${pack.quota_amount ?? 0} document generations`}</div>
                  <div className="mb-6 text-xs text-outline-variant space-y-0.5">
                    <div>{pricePerCreditLabel(pack)}</div>
                    <div>{imagesPerPack(pack)}</div>
                  </div>
                  <ul className="space-y-4 mb-10 text-outline-variant text-sm">
                    <li className="text-outline-variant leading-relaxed">{pack.description}</li>
                    <li className="flex gap-3"><Check className="text-tertiary w-5 h-5 shrink-0" /> External Mode remains free and unlimited</li>
                  </ul>
                  <motion.a
                    whileHover={{ scale: 0.98 }}
                    className={`mt-auto w-full py-4 rounded-md font-bold transition-all inline-flex items-center justify-center ${isPopular ? 'bg-gradient-to-br from-primary to-primary-container text-[#002e6b]' : 'border border-outline-variant/30 hover:bg-white/5'}`}
                    href={packHref}
                    onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.checkoutStart, { surface: 'site', placement: standalone ? 'pricing-page' : 'home-pricing', pack_code: pack.code, ...extractAttributionParams(location.search) })}
                  >
                    Open Secure Checkout
                  </motion.a>
                </motion.div>
              )
            })}
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
                href={buildTrackedURL(platformBillingURL, location.search)}
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
