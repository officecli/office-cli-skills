import { Link } from 'react-router-dom'
import CodeBlock from '../components/CodeBlock'
import AgentSkillsBreadcrumbs from '../components/AgentSkillsBreadcrumbs'
import AgentSkillsSecondaryNav from '../components/AgentSkillsSecondaryNav'
import { getAgentSkillsBreadcrumbs, githubRepoURL, productDocsPath } from '../agentSkillsData'

const claudeInstallCommand = '/plugin marketplace add officecli/officecli\n/plugin install officecli@officecli'

export default function AgentSkillsClaudeCodePage() {
  return (
    <main className="overflow-x-hidden px-8 pt-24 pb-18 md:px-16 md:pt-28 md:pb-24">
      <div className="mx-auto max-w-[1440px]">
        <AgentSkillsBreadcrumbs items={getAgentSkillsBreadcrumbs('/officecli/claude-code')} />
        <section className="rounded-[36px] border border-outline-variant/10 bg-surface-low px-8 py-10 shadow-[0_24px_70px_rgba(0,0,0,0.24)] md:px-10">
          <span className="inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-4 py-2 font-headline text-xs uppercase tracking-[0.28em] text-primary">
            officecli for Claude Code
          </span>
          <h1 className="mt-6 max-w-4xl font-headline text-5xl font-bold tracking-tight text-white md:text-6xl">
            Marketplace install for same-machine Office workflows
          </h1>
          <p className="mt-5 max-w-4xl text-lg leading-relaxed text-outline-variant">
            This page exists for the query pattern where people search for a Claude Code Office plugin and should land directly on the marketplace install flow for `officecli`, not on an unrelated raw file page.
          </p>
          <div className="mt-8">
            <AgentSkillsSecondaryNav currentPath="/officecli/claude-code" />
          </div>
        </section>

        <section className="mt-10 grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Install from the marketplace source</h2>
            <p className="mt-4 text-sm leading-relaxed text-outline-variant">
              Use the public `officecli/officecli` marketplace repository when Claude Code should discover the OfficeCLI plugin through the normal plugin flow.
            </p>
            <div className="mt-6">
              <CodeBlock command={claudeInstallCommand} />
            </div>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">What this path is for</h2>
            <div className="mt-5 space-y-4 text-sm leading-relaxed text-outline-variant">
              <p>Use it when the agent runtime is Claude Code and the Office request should stay on the local machine through OfficeCLI.</p>
              <p>It is the right entrypoint for PPTX, DOCX, XLSX, and workbook-backed report workflows when the local runtime is already available or will be installed next.</p>
              <p>It is not a hosted plugin backend. The public repo is a wrapper and distribution surface for a local runtime.</p>
            </div>
          </article>
        </section>

        <section className="mt-10 grid gap-6 md:grid-cols-3">
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Repository</h2>
            <a className="mt-4 block text-sm leading-relaxed text-primary transition-colors hover:text-tertiary" href={githubRepoURL}>
              officecli/officecli on GitHub
            </a>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Install guide</h2>
            <Link className="mt-4 block text-sm leading-relaxed text-primary transition-colors hover:text-tertiary" to="/officecli/install">
              Compare Claude Code, Codex, and OpenClaw install paths
            </Link>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Product docs</h2>
            <Link className="mt-4 block text-sm leading-relaxed text-primary transition-colors hover:text-tertiary" to={productDocsPath}>
              OfficeCLI formats, bridge behavior, and workflow docs
            </Link>
          </article>
        </section>
      </div>
    </main>
  )
}
