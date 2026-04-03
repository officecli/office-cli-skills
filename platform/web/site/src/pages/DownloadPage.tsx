import { useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import { platformAppURL } from '../siteData'

export default function DownloadPage() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)

  return (
    <main className="overflow-x-hidden pt-28 px-8 md:px-16 max-w-[1440px] mx-auto pb-24">
      <section className="grid grid-cols-1 lg:grid-cols-2 gap-10 items-start">
        <div>
          <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Download</span>
          <h1 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">Download OfficeCLI and wire it into your production workflow</h1>
          <p className="text-outline-variant text-lg leading-relaxed max-w-2xl">
            Install locally, verify authentication, then route quota purchases and API key management through platform.officecli.io.
          </p>
        </div>
        <div className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <div className="text-xs font-headline uppercase tracking-widest text-tertiary mb-4">Recommended sequence</div>
          <ol className="space-y-4 text-outline-variant text-sm list-decimal list-inside">
            <li>Install OfficeCLI and verify the runtime environment</li>
            <li>Create or manage your API key on platform</li>
            <li>Run `auth status` and `generate` to validate the end-to-end path</li>
          </ol>
          <pre className="mt-8 bg-background rounded-xl p-6 overflow-x-auto text-sm text-white border border-outline-variant/10"><code>{`brew install officecli\nofficecli auth status\nofficecli generate ./brief.md`}</code></pre>
        </div>
      </section>

      <section className="mt-16 grid grid-cols-1 md:grid-cols-2 gap-8">
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">Integration boundary</h2>
          <p className="text-outline-variant leading-relaxed">
            The public site handles discovery and onboarding. Authenticated entitlement checks resolve through <code>platform.officecli.io/api/license/*</code>.
          </p>
        </article>
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">Platform entry</h2>
          <p className="text-outline-variant leading-relaxed mb-6">
            Use platform to create API keys, inspect credit balances, and complete checkout for production quota packs.
          </p>
          <a className="inline-flex bg-gradient-to-br from-primary to-primary-container text-[#002e6b] px-6 py-3 rounded-md font-bold" href={platformAppHref} onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.downloadClick, { surface: 'site', placement: 'download-page', ...extractAttributionParams(location.search) })}>Open platform</a>
        </article>
      </section>
    </main>
  )
}
