import { Link } from 'react-router-dom'
import CodeBlock from '../components/CodeBlock'
import AgentSkillsSecondaryNav from '../components/AgentSkillsSecondaryNav'
import {
  agentSkillsFAQs,
  agentSkillsHubPath,
  downloadPath,
  githubRepoURL,
  installPaths,
  keywordChips,
  productDocsPath,
  repoArtifacts,
  runtimeQuickLinks,
  verificationCommands,
  workflowCards,
} from '../agentSkillsData'

export default function AgentSkillsPage() {
  return (
    <main className="overflow-x-hidden">
      <section className="relative isolate overflow-hidden px-8 pt-24 pb-18 md:px-16 md:pt-28 md:pb-24">
        <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(174,198,255,0.22),transparent_36%),radial-gradient(circle_at_82%_18%,rgba(0,220,229,0.14),transparent_22%),radial-gradient(circle_at_50%_100%,rgba(255,177,192,0.12),transparent_34%)]" />
        <div className="relative mx-auto grid max-w-[1440px] gap-10 lg:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.9fr)] lg:items-start">
          <div>
            <span className="mb-5 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-4 py-2 font-headline text-xs uppercase tracking-[0.28em] text-primary">
              officecli-skills public repository
            </span>
            <h1 className="max-w-5xl font-headline text-5xl font-bold tracking-tight text-white md:text-6xl lg:text-7xl">
              officecli-skills for Claude Code, Codex, and AI Agents
            </h1>
            <p className="mt-6 max-w-4xl text-lg leading-relaxed text-outline-variant md:text-xl">
              <strong className="text-white">officecli-skills</strong> is the public GitHub repository and install surface for Claude Code,
              Codex, OpenClaw, and other AI agents that need local <strong className="text-white">PPTX</strong>,{' '}
              <strong className="text-white">DOCX</strong>, <strong className="text-white">XLSX</strong>, and workbook-backed{' '}
              <strong className="text-white">report</strong> workflows. It packages marketplace metadata, public skill bundles, install scripts,
              and agent-facing guidance so Office document automation stays on the same machine through OfficeCLI instead of a hosted plugin backend.
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              {keywordChips.map((chip) => (
                <span
                  key={chip}
                  className="rounded-full border border-outline-variant/15 bg-surface-low/90 px-4 py-2 font-headline text-xs uppercase tracking-[0.22em] text-outline-variant"
                >
                  {chip}
                </span>
              ))}
            </div>
            <div className="mt-10 flex flex-wrap gap-4">
              <a
                className="rounded-full bg-gradient-to-r from-primary to-tertiary px-6 py-3 font-semibold text-[#002e6b] transition-transform hover:-translate-y-0.5"
                href={githubRepoURL}
              >
                Open GitHub repository
              </a>
              <Link
                className="rounded-full border border-outline-variant/20 px-6 py-3 font-semibold text-white transition-colors hover:border-primary/30 hover:text-primary"
                to={agentSkillsHubPath + '/install'}
              >
                Open install page
              </Link>
              <Link
                className="rounded-full border border-outline-variant/20 px-6 py-3 font-semibold text-white transition-colors hover:border-primary/30 hover:text-primary"
                to={productDocsPath}
              >
                Read product docs
              </Link>
            </div>
          </div>

          <aside className="glass-panel rounded-[32px] border border-outline-variant/10 p-8 shadow-[0_30px_90px_rgba(0,0,0,0.28)]">
            <div className="mb-6 font-headline text-xs uppercase tracking-[0.24em] text-tertiary">What this repo helps agents do</div>
            <div className="space-y-5">
              <div>
                <div className="text-sm uppercase tracking-[0.2em] text-outline-variant">Detect</div>
                <p className="mt-2 text-sm leading-relaxed text-white">
                  Check whether an Office request should route into OfficeCLI before the agent starts generating files.
                </p>
              </div>
              <div>
                <div className="text-sm uppercase tracking-[0.2em] text-outline-variant">Install</div>
                <p className="mt-2 text-sm leading-relaxed text-white">
                  Support both marketplace install and direct skill install without exposing closed-source OfficeCLI implementation code.
                </p>
              </div>
              <div>
                <div className="text-sm uppercase tracking-[0.2em] text-outline-variant">Generate</div>
                <p className="mt-2 text-sm leading-relaxed text-white">
                  Produce local PPTX, DOCX, XLSX, and report outputs through a consistent Office document skill surface.
                </p>
              </div>
              <div>
                <div className="text-sm uppercase tracking-[0.2em] text-outline-variant">Bridge</div>
                <p className="mt-2 text-sm leading-relaxed text-white">
                  Use `officecli agent-bridge` for agent-first flows instead of scraping human CLI output when a structured protocol exists.
                </p>
              </div>
            </div>
          </aside>
        </div>
      </section>

      <section className="px-8 py-4 md:px-16 md:py-6">
        <div className="mx-auto max-w-[1440px]">
          <AgentSkillsSecondaryNav currentPath={agentSkillsHubPath} />
        </div>
      </section>

      <section className="px-8 py-8 md:px-16 md:py-12">
        <div className="mx-auto grid max-w-[1440px] gap-6 lg:grid-cols-4">
          {repoArtifacts.map((item) => (
            <article key={item.title} className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
              <h2 className="font-headline text-2xl font-bold text-white">{item.title}</h2>
              <p className="mt-4 text-sm leading-relaxed text-outline-variant">{item.detail}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="px-8 py-12 md:px-16 md:py-16">
        <div className="mx-auto max-w-[1440px]">
          <div className="mb-10 max-w-3xl">
            <span className="mb-4 block font-headline text-xs uppercase tracking-[0.24em] text-primary">Primary entrypoints</span>
            <h2 className="font-headline text-4xl font-bold tracking-tight text-white md:text-5xl">
              Give Google stable child pages instead of random file links
            </h2>
            <p className="mt-4 text-lg leading-relaxed text-outline-variant">
              These pages map the main search intents around the public repo: install, Claude Code, Codex, OpenClaw, and FAQ.
            </p>
          </div>
          <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-4">
            {runtimeQuickLinks.map((item) => (
              <Link key={item.href} className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6 transition-colors hover:border-primary/20" to={item.href}>
                <h3 className="font-headline text-2xl font-bold text-white">{item.title}</h3>
                <p className="mt-4 text-sm leading-relaxed text-outline-variant">{item.description}</p>
              </Link>
            ))}
          </div>
        </div>
      </section>

      <section className="px-8 py-18 md:px-16 md:py-24">
        <div className="mx-auto max-w-[1440px]">
          <div className="mb-10 max-w-3xl">
            <span className="mb-4 block font-headline text-xs uppercase tracking-[0.24em] text-primary">Supported workflows</span>
            <h2 className="font-headline text-4xl font-bold tracking-tight text-white md:text-5xl">
              One skill surface for PPTX, DOCX, XLSX, and report tasks
            </h2>
            <p className="mt-4 text-lg leading-relaxed text-outline-variant">
              The repo should read clearly to both search engines and developers: it is about Office document automation for agent clients, not about a generic plugin shell.
            </p>
          </div>
          <div className="grid gap-6 md:grid-cols-2">
            {workflowCards.map((item) => (
              <article key={item.title} className="rounded-3xl border border-outline-variant/10 bg-surface-low p-7">
                <h3 className="font-headline text-2xl font-bold text-white">{item.title}</h3>
                <p className="mt-4 text-sm leading-relaxed text-outline-variant">{item.description}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className="bg-surface-low px-8 py-18 md:px-16 md:py-24">
        <div className="mx-auto max-w-[1440px]">
          <div className="mb-10 max-w-3xl">
            <span className="mb-4 block font-headline text-xs uppercase tracking-[0.24em] text-tertiary">Install paths</span>
            <h2 className="font-headline text-4xl font-bold tracking-tight text-white md:text-5xl">
              Choose the path that matches the agent runtime
            </h2>
            <p className="mt-4 text-lg leading-relaxed text-outline-variant">
              Search traffic should land on a page that reaches installation in one or two steps. These are the three install surfaces that matter most for the public repo.
            </p>
          </div>
          <div className="grid gap-8 xl:grid-cols-3">
            {installPaths.map((item) => (
              <article key={item.title} className="rounded-3xl border border-outline-variant/10 bg-background/80 p-6">
                <h3 className="font-headline text-2xl font-bold text-white">{item.title}</h3>
                <p className="mt-4 text-sm leading-relaxed text-outline-variant">{item.intro}</p>
                <div className="mt-6">
                  <CodeBlock command={item.command} />
                </div>
              </article>
            ))}
          </div>
          <div className="mt-8 grid gap-6 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]">
            <article className="rounded-3xl border border-outline-variant/10 bg-background/80 p-6">
              <h3 className="font-headline text-2xl font-bold text-white">Verify the local runtime</h3>
              <p className="mt-4 text-sm leading-relaxed text-outline-variant">
                The public repo only solves distribution and routing. The final Office artifact still depends on a working local OfficeCLI runtime.
              </p>
              <div className="mt-6">
                <CodeBlock command={verificationCommands} />
              </div>
            </article>
            <article className="rounded-3xl border border-outline-variant/10 bg-background/80 p-6">
              <h3 className="font-headline text-2xl font-bold text-white">Related entrypoints</h3>
              <div className="mt-5 space-y-4 text-sm leading-relaxed">
                <a className="block text-primary transition-colors hover:text-tertiary" href={githubRepoURL}>
                  GitHub: officecli/officecli-skills
                </a>
                <Link className="block text-primary transition-colors hover:text-tertiary" to={`${agentSkillsHubPath}/claude-code`}>
                  Claude Code marketplace install
                </Link>
                <Link className="block text-primary transition-colors hover:text-tertiary" to={`${agentSkillsHubPath}/codex`}>
                  Codex direct local install
                </Link>
                <Link className="block text-primary transition-colors hover:text-tertiary" to={`${agentSkillsHubPath}/openclaw`}>
                  OpenClaw bridge install
                </Link>
                <Link className="block text-primary transition-colors hover:text-tertiary" to={productDocsPath}>
                  Product docs for OfficeCLI formats and agent bridge
                </Link>
                <Link className="block text-primary transition-colors hover:text-tertiary" to={downloadPath}>
                  OfficeCLI install methods for macOS and Linux
                </Link>
              </div>
            </article>
          </div>
        </div>
      </section>

      <section className="px-8 py-18 md:px-16 md:py-24">
        <div className="mx-auto max-w-[1440px]">
          <div className="mb-10 max-w-3xl">
            <span className="mb-4 block font-headline text-xs uppercase tracking-[0.24em] text-secondary">FAQ</span>
            <h2 className="font-headline text-4xl font-bold tracking-tight text-white md:text-5xl">
              Frequently asked questions for search and onboarding
            </h2>
            <p className="mt-4 text-lg leading-relaxed text-outline-variant">
              These answers deliberately cover the search phrases people use when they are trying to understand whether this repository is the right entrypoint for AI Office document workflows.
            </p>
          </div>
          <div className="grid gap-6 md:grid-cols-2">
            {agentSkillsFAQs.map((faq) => (
              <article key={faq.q} className="rounded-3xl border border-outline-variant/10 bg-surface-low p-7">
                <h3 className="font-headline text-2xl font-bold text-white">{faq.q}</h3>
                <p className="mt-4 text-sm leading-relaxed text-outline-variant">{faq.a}</p>
              </article>
            ))}
          </div>
        </div>
      </section>
    </main>
  )
}
