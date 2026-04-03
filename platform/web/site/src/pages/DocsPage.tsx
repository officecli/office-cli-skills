import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import { platformAppURL, platformBillingURL, platformLicenseAPIURL } from '../siteData'

export default function DocsPage() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)
  const platformBillingHref = buildTrackedURL(platformBillingURL, location.search)

  return (
    <main className="overflow-x-hidden pt-28 px-8 md:px-16 max-w-[1440px] mx-auto pb-24">
      <section className="max-w-3xl">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Docs</span>
        <h1 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">Install the CLI, authenticate once, then hand off to platform</h1>
        <p className="text-outline-variant text-lg leading-relaxed">
          This marketing site explains the product model. The authenticated experience lives in platform, where users manage API keys, quota packs, and billing events.
        </p>
      </section>

      <section className="mt-16 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-8">
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">What the public site handles</h2>
          <p className="text-outline-variant leading-relaxed">Product positioning, download onboarding, pricing understanding, and FAQ-level trust content live here.</p>
        </article>
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">What platform handles</h2>
          <p className="text-outline-variant leading-relaxed mb-6">Authenticated API keys, orders, credit packs, usage history, and billing operations all live in the platform console.</p>
          <div className="flex flex-wrap gap-4">
            <Link className="border border-outline-variant/30 text-white px-5 py-3 rounded-md font-bold hover:bg-white/5 transition-all" to="/download">Download CLI</Link>
            <a className="bg-gradient-to-br from-primary to-primary-container text-[#002e6b] px-5 py-3 rounded-md font-bold" href={platformAppHref} onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.consoleOpen, { surface: 'site', placement: 'docs-user-center', ...extractAttributionParams(location.search) })}>Open user center</a>
          </div>
        </article>
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">Integration endpoints</h2>
          <ul className="space-y-3 text-sm text-outline-variant leading-relaxed">
            <li><span className="text-white">Pricing API</span>: <code>/api/pricing</code></li>
            <li><span className="text-white">License check</span>: <code>{platformLicenseAPIURL}</code></li>
            <li><span className="text-white">Billing console</span>: <a className="text-tertiary hover:underline" href={platformBillingHref} onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.checkoutStart, { surface: 'site', placement: 'docs-billing-link', ...extractAttributionParams(location.search) })}>{platformBillingHref}</a></li>
          </ul>
        </article>
      </section>
    </main>
  )
}
