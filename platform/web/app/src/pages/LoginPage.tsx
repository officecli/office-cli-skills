import { useMemo } from 'react'
import { Button } from 'antd'
import { ArrowLeft, ShieldCheck } from 'lucide-react'
import { useLocation } from 'react-router-dom'
import { api } from '../api'
import { OfficeCliBrand } from '../components/branding'

export default function LoginPage() {
  const location = useLocation()
  const returnTo = useMemo(() => {
    const requested = new URLSearchParams(location.search).get('return_to')?.trim()
    if (!requested || !requested.startsWith('/')) return '/app'
    return requested
  }, [location.search])

  return (
    <div className="min-h-screen bg-background px-6 py-8 text-on-surface">
      <div className="centered-auth-shell mx-auto flex max-w-4xl items-center">
        <section className="app-panel relative w-full overflow-hidden p-8 md:p-12">
          <div className="login-hero-app-glow absolute inset-0" />
          <div className="relative">
            <OfficeCliBrand
              markClassName="h-11 w-11"
              titleClassName="text-xl font-bold text-white"
              subtitle="sign in required"
            />

            <span className="chip mt-8">Google sign-in required</span>
            <h1 className="mt-6 text-5xl font-bold leading-[0.92] text-white md:text-6xl">Continue to OfficeCLI</h1>
            <p className="mt-6 max-w-2xl text-base leading-7 text-outline">Sign in with your Google account to continue to the workspace. Access may still be limited by the current app policy.</p>

            <div className="app-card-muted mt-8 p-6">
              <div className="text-sm text-outline">You will be redirected to Google authentication and then returned to your requested workspace page if the account is permitted and still active.</div>
            </div>

            <div className="mt-10 flex flex-wrap gap-3">
              <Button type="primary" icon={<ShieldCheck size={16} />} onClick={() => api.login(returnTo)}>
                Continue with Google
              </Button>
              <Button href="https://officecli.io/" icon={<ArrowLeft size={16} />}>
                Back to site
              </Button>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
