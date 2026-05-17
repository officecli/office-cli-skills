import { ArrowLeft, MailWarning, RefreshCcw, ShieldBan } from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'
import { OfficeCliBrand } from '../components/branding'

export default function AccessDeniedPage() {
  const location = useLocation()
  const email = new URLSearchParams(location.search).get('email')

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
              subtitle="admin access / restricted"
            />
            <span className="chip border-error/20 bg-error/10 text-error">Admin access denied</span>
            <h1 className="mt-6 text-5xl font-bold leading-[0.92] text-white md:text-6xl">Access not granted</h1>
            <p className="mt-6 max-w-2xl text-base leading-7 text-outline">This Google account completed authentication, but OfficeCLI did not issue an admin session because the account is not present in the admin allowlist.</p>

            <div className="admin-code-card mt-8 p-5">
              <div className="flex items-start gap-3">
                <MailWarning size={18} className="mt-0.5 text-tertiary" />
                <div>
                  <div className="info-eyebrow text-outline">Authenticated account</div>
                  <div className="mt-2 text-lg font-semibold text-white">{email || 'No email provided by callback'}</div>
                </div>
              </div>
            </div>

            <div className="mt-10 flex flex-wrap gap-3">
              <a href="/api/admin/auth/google/login?return_to=%2Fadmin" className="admin-primary-button">
                <RefreshCcw size={16} />
                Try another Google account
              </a>
              <Link to="/" className="admin-secondary-button">
                <ArrowLeft size={16} />
                Retry admin access
              </Link>
              <a href="mailto:hello@officecli.io?subject=Admin%20allowlist%20request" className="admin-secondary-button">
                <ShieldBan size={16} />
                Contact an administrator
              </a>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
