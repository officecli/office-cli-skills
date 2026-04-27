import { Link } from 'react-router-dom'
import AgentSkillsBreadcrumbs from '../components/AgentSkillsBreadcrumbs'
import AgentSkillsSecondaryNav from '../components/AgentSkillsSecondaryNav'
import { agentSkillsFAQs, getAgentSkillsBreadcrumbs, githubRepoURL } from '../agentSkillsData'

export default function AgentSkillsFAQPage() {
  return (
    <main className="overflow-x-hidden px-8 pt-24 pb-18 md:px-16 md:pt-28 md:pb-24">
      <div className="mx-auto max-w-[1440px]">
        <AgentSkillsBreadcrumbs items={getAgentSkillsBreadcrumbs('/officecli-skills/faq')} />
        <section className="rounded-[36px] border border-outline-variant/10 bg-surface-low px-8 py-10 shadow-[0_24px_70px_rgba(0,0,0,0.24)] md:px-10">
          <span className="inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-4 py-2 font-headline text-xs uppercase tracking-[0.28em] text-primary">
            officecli-skills FAQ
          </span>
          <h1 className="mt-6 max-w-4xl font-headline text-5xl font-bold tracking-tight text-white md:text-6xl">
            FAQ for public repo and runtime questions
          </h1>
          <p className="mt-5 max-w-4xl text-lg leading-relaxed text-outline-variant">
            This page captures the queries people repeatedly ask before they decide whether `officecli-skills` is the right entrypoint for local AI Office document workflows.
          </p>
          <div className="mt-8">
            <AgentSkillsSecondaryNav currentPath="/officecli-skills/faq" />
          </div>
        </section>

        <section className="mt-10 grid gap-6 md:grid-cols-2">
          {agentSkillsFAQs.map((faq) => (
            <article key={faq.q} className="rounded-3xl border border-outline-variant/10 bg-surface-low p-7">
              <h2 className="font-headline text-2xl font-bold text-white">{faq.q}</h2>
              <p className="mt-4 text-sm leading-relaxed text-outline-variant">{faq.a}</p>
            </article>
          ))}
        </section>

        <section className="mt-10 grid gap-6 md:grid-cols-2">
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Need install details instead?</h2>
            <Link className="mt-4 block text-sm leading-relaxed text-primary transition-colors hover:text-tertiary" to="/officecli-skills/install">
              Open the install page
            </Link>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Need the public repo?</h2>
            <a className="mt-4 block text-sm leading-relaxed text-primary transition-colors hover:text-tertiary" href={githubRepoURL}>
              Open officecli/officecli-skills on GitHub
            </a>
          </article>
        </section>
      </div>
    </main>
  )
}
