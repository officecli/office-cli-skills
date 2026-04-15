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
        <h1 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">What OfficeCLI handles today, and what comes next</h1>
        <p className="text-outline-variant text-lg leading-relaxed">
          OfficeCLI is a local-first CLI for document operations. Today it is strongest at creating files and reviewing PPT decks. Hosted platform workflows remain optional for paid access, billing, and preview publishing.
        </p>
      </section>

      <section className="mt-16 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-8">
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">Current release</h2>
          <p className="text-outline-variant leading-relaxed">Create PPTX, DOCX, XLSX, and workbook-backed Report outputs. Review and score PPTX decks. Keep files local by default, or publish online previews when configured.</p>
        </article>
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">External vs hosted</h2>
          <p className="text-outline-variant leading-relaxed mb-6">External mode uses your own LLM endpoint for local workflows. Hosted mode is optional when you need paid access, managed runtime behavior, or billing-aware usage.</p>
          <div className="flex flex-wrap gap-4">
            <Link className="border border-outline-variant/30 text-white px-5 py-3 rounded-md font-bold hover:bg-white/5 transition-all" to="/download">Install CLI</Link>
            <a className="bg-gradient-to-br from-primary to-primary-container text-[#002e6b] px-5 py-3 rounded-md font-bold" href={platformAppHref} onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.consoleOpen, { surface: 'site', placement: 'docs-user-center', ...extractAttributionParams(location.search) })}>Open platform</a>
          </div>
        </article>
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">Platform endpoints</h2>
          <ul className="space-y-3 text-sm text-outline-variant leading-relaxed">
            <li><span className="text-white">Pricing API</span>: <code>/api/pricing</code></li>
            <li><span className="text-white">License check</span>: <code>{platformLicenseAPIURL}</code></li>
            <li><span className="text-white">Billing console</span>: <a className="text-tertiary hover:underline" href={platformBillingHref} onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.checkoutStart, { surface: 'site', placement: 'docs-billing-link', ...extractAttributionParams(location.search) })}>{platformBillingHref}</a></li>
          </ul>
        </article>
      </section>

      <section className="mt-12 bg-surface-low rounded-3xl border border-outline-variant/10 p-8 md:p-10">
        <div className="text-xs font-headline uppercase tracking-widest text-tertiary mb-4">Roadmap boundary</div>
        <h2 className="font-headline text-3xl font-bold text-white mb-4">What is planned, but not in the current release</h2>
        <p className="text-outline-variant leading-relaxed max-w-3xl">
          Broader document operations are on the roadmap: format conversion, content modification, summarization, extraction, and richer layout or formatting workflows across more document families.
        </p>
      </section>
    </main>
  )
}
