import { ArrowLeft, ShieldCheck, Sparkles } from 'lucide-react'
import { api } from '../api'

export default function LoginPage() {
  return (
    <div className="min-h-screen bg-background px-6 py-8 text-on-surface">
      <div className="centered-auth-shell mx-auto flex max-w-4xl items-center">
        <section className="panel relative w-full overflow-hidden p-8 md:p-12">
          <div className="login-hero-app-glow absolute inset-0" />
          <div className="relative">
            <div className="flex items-center gap-3">
              <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-primary/20 bg-primary/10 text-primary">
                <Sparkles size={18} />
              </div>
              <div>
                <div className="font-headline text-xl font-bold text-white">OfficeCLI</div>
                <div className="info-eyebrow text-outline">sign in required</div>
              </div>
            </div>

            <span className="chip mt-8">Google sign-in</span>
            <h1 className="mt-6 text-5xl font-bold leading-[0.92] text-white md:text-6xl">Sign in to continue</h1>
            <p className="mt-6 max-w-2xl text-base leading-7 text-outline">Use your Google account to continue to the OfficeCLI workspace.</p>

            <div className="panel-muted mt-8 p-6">
              <div className="text-sm text-outline">You will be redirected to Google authentication and then returned to the app.</div>
            </div>

            <div className="mt-10 flex flex-wrap gap-3">
              <button type="button" className="tonal-button" onClick={() => api.login('/app')}>
                <ShieldCheck size={16} />
                Continue with Google
              </button>
              <a href="https://officecli.io/" className="ghost-button">
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
