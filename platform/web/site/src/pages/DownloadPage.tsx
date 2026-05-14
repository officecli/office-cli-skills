import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import InstallTabs from '../components/InstallTabs'
import { platformAppURL } from '../siteData'

export default function DownloadPage() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)

  return (
    <main className="overflow-x-hidden pt-28 px-8 md:px-16 max-w-[1440px] mx-auto pb-24">
      <section className="max-w-4xl">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Download</span>
        <h1 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">Install OfficeCLI for AI document generation</h1>
        <p className="text-outline-variant text-lg leading-relaxed max-w-3xl">
          Use Homebrew, npm, the official install script, or manual binaries to get started. Install one dependency-free binary, then generate PPTX, DOCX, XLSX, REPORT, and IMG outputs with hosted trial access by default.
        </p>
      </section>

      <div className="mt-16">
        <InstallTabs compact headline="Choose the install path that matches your machine" intro="OfficeCLI supports macOS and Linux on x64 and arm64. Pick the setup that fits your workstation or CI environment, then copy a first-run command below." />
      </div>

      <section className="mt-16 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-8">
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">What you need</h2>
          <p className="text-outline-variant leading-relaxed">
            OfficeCLI does not require Python, LibreOffice, Microsoft Office, Docker, Kubernetes, or an AI agent. Hosted Mode works first; External Mode remains available when you want to use your own model endpoint.
          </p>
        </article>
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">Recommended sequence</h2>
          <ol className="space-y-3 text-sm text-outline-variant list-decimal list-inside">
            <li>Install OfficeCLI with Homebrew, npm, the script, or a manual binary.</li>
            <li>Run <code>officecli --version</code>, then copy one of the first-run generation commands.</li>
            <li>Use <code>officecli auth status</code> to check trial or hosted key access.</li>
          </ol>
        </article>
        <article className="bg-surface-low rounded-2xl border border-outline-variant/10 p-8">
          <h2 className="font-headline text-2xl font-bold text-white mb-4">Optional platform features</h2>
          <p className="text-outline-variant leading-relaxed mb-6">
            Use platform to manage hosted credits, API keys, billing, and optional online preview publishing. External Mode generation remains free and unlimited.
          </p>
          <div className="flex flex-wrap gap-4">
            <a className="inline-flex bg-gradient-to-br from-primary to-primary-container text-[#002e6b] px-6 py-3 rounded-md font-bold" href={platformAppHref} onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.downloadClick, { surface: 'site', placement: 'download-page', ...extractAttributionParams(location.search) })}>Open platform</a>
            <Link className="inline-flex border border-outline-variant/20 px-6 py-3 rounded-md font-bold text-white hover:border-primary/30 hover:text-primary" to="/docs">Read capability docs</Link>
          </div>
        </article>
      </section>
    </main>
  )
}
