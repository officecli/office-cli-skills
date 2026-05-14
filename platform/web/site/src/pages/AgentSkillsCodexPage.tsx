import { Link } from 'react-router-dom'
import CodeBlock from '../components/CodeBlock'
import AgentSkillsBreadcrumbs from '../components/AgentSkillsBreadcrumbs'
import AgentSkillsSecondaryNav from '../components/AgentSkillsSecondaryNav'
import { getAgentSkillsBreadcrumbs, githubRepoURL, verificationCommands } from '../agentSkillsData'

const codexInstallCommand =
  'curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-skill.sh | bash -s -- officecli'

export default function AgentSkillsCodexPage() {
  return (
    <main className="overflow-x-hidden px-8 pt-24 pb-18 md:px-16 md:pt-28 md:pb-24">
      <div className="mx-auto max-w-[1440px]">
        <AgentSkillsBreadcrumbs items={getAgentSkillsBreadcrumbs('/officecli/codex')} />
        <section className="rounded-[36px] border border-outline-variant/10 bg-surface-low px-8 py-10 shadow-[0_24px_70px_rgba(0,0,0,0.24)] md:px-10">
          <span className="inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-4 py-2 font-headline text-xs uppercase tracking-[0.28em] text-primary">
            officecli for Codex
          </span>
          <h1 className="mt-6 max-w-4xl font-headline text-5xl font-bold tracking-tight text-white md:text-6xl">
            Direct local skill install without a marketplace layer
          </h1>
          <p className="mt-5 max-w-4xl text-lg leading-relaxed text-outline-variant">
            For Codex-style local agents, `officecli` should resolve to a direct installer and a refreshable local skill bundle. This page separates that path from Claude Code marketplace instructions so the intent is unambiguous.
          </p>
          <div className="mt-8">
            <AgentSkillsSecondaryNav currentPath="/officecli/codex" />
          </div>
        </section>

        <section className="mt-10 grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Direct install command</h2>
            <p className="mt-4 text-sm leading-relaxed text-outline-variant">
              Use the public installer when you want the `officecli` skill files on a local Codex host without adding a marketplace source.
            </p>
            <div className="mt-6">
              <CodeBlock command={codexInstallCommand} />
            </div>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Verify the local host</h2>
            <p className="mt-4 text-sm leading-relaxed text-outline-variant">
              The skill bundle is only the routing layer. A working OfficeCLI runtime is still required for final file generation.
            </p>
            <div className="mt-6">
              <CodeBlock command={verificationCommands} />
            </div>
          </article>
        </section>

        <section className="mt-10 grid gap-6 md:grid-cols-3">
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Runtime difference</h2>
            <p className="mt-4 text-sm leading-relaxed text-outline-variant">
              Codex uses direct local skill files. Claude Code can use the marketplace wrapper. The OfficeCLI runtime behind both paths is still local-first.
            </p>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">GitHub repository</h2>
            <a className="mt-4 block text-sm leading-relaxed text-primary transition-colors hover:text-tertiary" href={githubRepoURL}>
              officecli/officecli on GitHub
            </a>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Related page</h2>
            <Link className="mt-4 block text-sm leading-relaxed text-primary transition-colors hover:text-tertiary" to="/officecli/install">
              Compare all install routes
            </Link>
          </article>
        </section>
      </div>
    </main>
  )
}
