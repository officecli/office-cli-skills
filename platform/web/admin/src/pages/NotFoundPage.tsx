import { ArrowLeft, FileSearch } from 'lucide-react'
import { OfficeCliBrand } from '../components/branding'

export default function NotFoundPage() {
  return (
    <div className="min-h-screen bg-background px-6 py-8 text-on-surface">
      <div className="centered-auth-shell mx-auto flex max-w-4xl items-center">
        <section className="panel relative w-full overflow-hidden p-8 md:p-12">
          <div className="access-denied-glow absolute inset-0" />
          <div className="relative">
            <OfficeCliBrand
              className="mb-8"
              markClassName="h-11 w-11"
              title="OfficeCLI admin"
              titleClassName="text-xl font-bold text-white"
              subtitle="governance console"
            />
            <span className="chip">404</span>
            <h1 className="mt-6 text-5xl font-bold leading-[0.92] text-white md:text-6xl">Page not found</h1>
            <p className="mt-6 max-w-2xl text-base leading-7 text-outline">The page you requested could not be found.</p>

            <div className="admin-code-card mt-8 flex items-start gap-3 p-5">
              <FileSearch size={18} className="mt-0.5 text-tertiary" />
              <div>
                <div className="info-eyebrow text-outline">Not Found</div>
                <div className="mt-2 text-sm text-outline">Check the URL or return to the main site.</div>
              </div>
            </div>

            <div className="mt-10 flex flex-wrap gap-3">
              <a href="https://officecli.io/" className="admin-secondary-button">
                <ArrowLeft size={16} />
                Back to site
              </a>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
