import { useMemo } from 'react'
import { ArrowLeft, Github } from 'lucide-react'
import { useLocation } from 'react-router-dom'
import { trackEvent } from '../analytics'
import { APP_ANALYTICS_EVENTS } from '../analytics-events'
import { api } from '../api'
import { OfficeCliMark } from '../components/branding'

function GoogleIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 48 48" aria-hidden="true">
      <path fill="#FFC107" d="M43.6 20.5H42V20H24v8h11.3c-1.6 4.7-6 8-11.3 8-6.6 0-12-5.4-12-12s5.4-12 12-12c3.1 0 5.8 1.2 7.9 3.1l5.7-5.7C34.1 6.1 29.3 4 24 4 12.9 4 4 12.9 4 24s8.9 20 20 20 20-8.9 20-20c0-1.2-.1-2.3-.4-3.5z" />
      <path fill="#FF3D00" d="M6.3 14.7l6.6 4.8C14.7 15.3 19 12 24 12c3.1 0 5.8 1.2 7.9 3.1l5.7-5.7C34.1 6.1 29.3 4 24 4 16.3 4 9.7 8.3 6.3 14.7z" />
      <path fill="#4CAF50" d="M24 44c5.2 0 9.9-2 13.4-5.2l-6.2-5.2C29.2 35.1 26.7 36 24 36c-5.3 0-9.7-3.3-11.3-8l-6.5 5C9.5 39.6 16.2 44 24 44z" />
      <path fill="#1976D2" d="M43.6 20.5H42V20H24v8h11.3c-.8 2.3-2.3 4.3-4.1 5.6l6.2 5.2C41.1 35.6 44 30.3 44 24c0-1.2-.1-2.3-.4-3.5z" />
    </svg>
  )
}

export default function LoginPage() {
  const location = useLocation()
  const returnTo = useMemo(() => {
    const requested = new URLSearchParams(location.search).get('return_to')?.trim()
    if (!requested || !requested.startsWith('/')) return '/app'
    return requested
  }, [location.search])

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background px-6 py-12 text-on-surface">
      <div className="login-hero-app-glow pointer-events-none absolute inset-0" />
      <div className="relative w-full max-w-sm">
        <div className="flex flex-col items-center text-center">
          <OfficeCliMark className="h-14 w-14" />
          <h1 className="mt-6 text-2xl font-semibold tracking-tight text-white">Sign in to OfficeCLI</h1>
          <p className="mt-2 text-sm leading-6 text-outline">
            Continue to your workspace with your account.
          </p>
        </div>

        <div className="mt-8 flex flex-col gap-3">
          <button
            type="button"
            onClick={() => {
              trackEvent(APP_ANALYTICS_EVENTS.loginStart, { surface: 'app', placement: 'login-page', provider: 'google' })
              api.login(returnTo)
            }}
            style={{ backgroundColor: '#ffffff', color: '#0f172a' }}
            className="flex h-11 w-full items-center justify-center gap-3 rounded-lg text-sm font-medium shadow-sm transition hover:brightness-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-white/60"
          >
            <GoogleIcon />
            <span>Continue with Google</span>
          </button>
          <button
            type="button"
            onClick={() => {
              trackEvent(APP_ANALYTICS_EVENTS.loginStart, { surface: 'app', placement: 'login-page', provider: 'github' })
              api.loginWithGitHub(returnTo)
            }}
            style={{ backgroundColor: '#24292f', color: '#ffffff' }}
            className="flex h-11 w-full items-center justify-center gap-3 rounded-lg text-sm font-medium shadow-sm transition hover:brightness-110 focus:outline-none focus-visible:ring-2 focus-visible:ring-white/60"
          >
            <Github size={18} color="#ffffff" />
            <span>Continue with GitHub</span>
          </button>
        </div>

        <p className="mt-6 text-center text-xs leading-5 text-outline">
          By continuing, you agree to the workspace access policy. Access may still be limited based on the current app policy.
        </p>

        <div className="mt-8 flex justify-center">
          <a
            href="https://officecli.io/"
            style={{ color: 'rgba(255,255,255,0.55)' }}
            className="inline-flex items-center gap-1.5 text-xs no-underline transition hover:!text-white"
          >
            <ArrowLeft size={14} />
            Back to officecli.io
          </a>
        </div>
      </div>
    </div>
  )
}
