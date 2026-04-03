import { useLocation } from 'react-router-dom'
import { buildTrackedURL } from '../analytics'
import { platformAppURL } from '../siteData'

export default function LoginPage() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)

  return (
    <main className="overflow-x-hidden pt-28 px-8 md:px-16 max-w-[1440px] mx-auto pb-24">
      <section className="max-w-3xl bg-surface-low rounded-2xl border border-outline-variant/10 p-10">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Login</span>
        <h1 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">Login and billing are centralized on platform</h1>
        <p className="text-outline-variant text-lg leading-relaxed mb-8">
          Google OAuth, Stripe checkout, session cookies, and the authenticated console all run from platform.officecli.io so the public site can stay fast and static.
        </p>
        <a className="inline-flex bg-gradient-to-br from-primary to-primary-container text-[#002e6b] px-6 py-3 rounded-md font-bold" href={platformAppHref}>Go to platform.officecli.io/app</a>
      </section>
    </main>
  )
}
