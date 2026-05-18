import { Link, Navigate } from 'react-router-dom'
import type { SEOLandingPage as SEOLandingPageData } from '../seoLandingPages'
import CodeBlock from '../components/CodeBlock'

export default function SEOLandingPage({ page }: { page?: SEOLandingPageData }) {
  if (!page) return <Navigate to="/" replace />

  return (
    <main className="overflow-x-hidden px-8 md:px-16 pt-28 pb-24 max-w-[1440px] mx-auto">
      <section className="max-w-5xl">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">{page.eyebrow}</span>
        <h1 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">{page.title}</h1>
        <p className="text-outline-variant text-lg md:text-xl leading-relaxed max-w-4xl">{page.intro}</p>
        <div className="mt-8 max-w-4xl">
          <CodeBlock command={page.command} label="Example command" />
        </div>
      </section>

      <section className="mt-16 grid gap-10 lg:grid-cols-[1fr_22rem]">
        <div className="space-y-12">
          {page.sections.map((section) => (
            <article key={section.title}>
              <h2 className="font-headline text-3xl md:text-4xl font-bold text-white tracking-tight mb-5">{section.title}</h2>
              <div className="space-y-5">
                {section.body.map((paragraph) => (
                  <p key={paragraph} className="text-outline-variant text-base md:text-lg leading-relaxed">{paragraph}</p>
                ))}
              </div>
            </article>
          ))}
        </div>

        <aside className="space-y-6">
          <div className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white mb-4">Best for</h2>
            <ul className="space-y-3 text-sm leading-relaxed text-outline-variant">
              {page.bestFor.map((item) => <li key={item}>{item}</li>)}
            </ul>
          </div>
          <div className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white mb-4">Not for</h2>
            <ul className="space-y-3 text-sm leading-relaxed text-outline-variant">
              {page.notFor.map((item) => <li key={item}>{item}</li>)}
            </ul>
          </div>
          <div className="rounded-2xl border border-primary/15 bg-surface-high p-6">
            <h2 className="font-headline text-2xl font-bold text-white mb-4">Related pages</h2>
            <div className="space-y-3 text-sm">
              <Link className="block text-primary hover:text-tertiary" to="/download">Install OfficeCLI</Link>
              <Link className="block text-primary hover:text-tertiary" to="/docs">Product docs</Link>
              <Link className="block text-primary hover:text-tertiary" to="/officecli">Agent skills</Link>
            </div>
          </div>
        </aside>
      </section>

      <section className="mt-16">
        <h2 className="font-headline text-3xl md:text-4xl font-bold text-white tracking-tight mb-6">FAQ</h2>
        <div className="grid gap-6 md:grid-cols-3">
          {page.faqs.map((faq) => (
            <article key={faq.q} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
              <h3 className="font-headline text-xl font-bold text-white mb-3">{faq.q}</h3>
              <p className="text-sm leading-relaxed text-outline-variant">{faq.a}</p>
            </article>
          ))}
        </div>
      </section>
    </main>
  )
}
