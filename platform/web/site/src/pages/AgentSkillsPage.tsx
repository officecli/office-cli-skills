import { Link } from 'react-router-dom'
import CodeBlock from '../components/CodeBlock'

const githubRepoURL = 'https://github.com/officecli/officecli-skills'

const keywordChips = ['Claude Code', 'Codex', 'AI Agents', 'PPTX', 'DOCX', 'XLSX', 'Report', 'Skills']

const workflowCards = [
  {
    title: 'AI PPTX generation',
    description:
      'Create slide decks, proposal decks, product intros, and executive reviews from natural-language prompts while keeping generation local to the same machine.',
  },
  {
    title: 'AI DOCX drafting',
    description:
      'Draft retrospectives, proposals, memos, and customer-facing documents through an OfficeCLI skill instead of building one-off document automation scripts.',
  },
  {
    title: 'AI XLSX creation',
    description:
      'Generate spreadsheet structures, budget trackers, sales workbooks, and table-heavy outputs through the same skill surface used by agent clients.',
  },
  {
    title: 'Workbook-backed report workflows',
    description:
      'Route report generation through OfficeCLI when the workbook is the source of truth and the agent needs a local report artifact rather than a chat-only summary.',
  },
]

const installPaths = [
  {
    title: 'Claude Code marketplace install',
    intro:
      'Use the public marketplace source when you want Claude Code to discover the OfficeCLI skill through the plugin flow.',
    command: '/plugin marketplace add officecli/officecli-skills\n/plugin install officecli@officecli-skills',
  },
  {
    title: 'Codex and local agent install',
    intro:
      'Use the direct installer when you want the public OfficeCLI skill files on a Codex-style local agent host without a marketplace dependency.',
    command:
      'curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | bash -s -- officecli',
  },
  {
    title: 'OpenClaw install',
    intro:
      'Use the OpenClaw-oriented installer when the agent should generate Office files through `officecli agent-bridge` and return them to chat channels as attachments.',
    command:
      'curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-openclaw-skill.sh | bash',
  },
]

const repoArtifacts = [
  {
    title: 'Public GitHub repository',
    detail:
      'The repo is the public distribution surface for OfficeCLI skills, install scripts, skill docs, and marketplace wrappers.',
  },
  {
    title: 'Claude Code plugin wrappers',
    detail:
      'The `officecli` plugin targets Claude Code workflows where the agent should route Office tasks into the local OfficeCLI runtime.',
  },
  {
    title: 'Codex-compatible skill bundle',
    detail:
      'The public `skills/officecli` bundle is designed for local skill installs where Codex or similar agents can refresh the bundle and validate the host environment.',
  },
  {
    title: 'OpenClaw package',
    detail:
      'The OpenClaw-facing package keeps channel-based Office file generation on the same host through `officecli agent-bridge` instead of scraping human CLI output.',
  },
]

const faqs = [
  {
    q: 'What is the difference between OfficeCLI and officecli-skills?',
    a: 'OfficeCLI is the local document engine. `officecli-skills` is the public repository that distributes the skill definitions, plugin wrappers, and install scripts for agent clients.',
  },
  {
    q: 'Can Claude Code create PPTX, DOCX, XLSX, or report outputs with this repo?',
    a: 'Yes, when the OfficeCLI runtime is installed and configured locally. The public repo tells Claude Code how to route supported Office tasks into OfficeCLI instead of improvising another generation path.',
  },
  {
    q: 'Why mention Codex if this is a GitHub skills repo?',
    a: 'Because the repo is also a direct skill distribution surface for Codex-style local agents. Marketplace install is only one entrypoint; direct skill install is another.',
  },
  {
    q: 'Is this a hosted SaaS plugin backend?',
    a: 'No. The public skills repo distributes local wrappers and setup logic. Document generation still runs through the user-managed OfficeCLI runtime on the local machine.',
  },
  {
    q: 'Which keywords should this page and repo target?',
    a: 'The intended search terms are combinations of Claude Code, Codex, AI agents, Office skills, PPTX, DOCX, XLSX, report generation, and local Office document automation.',
  },
]

export default function AgentSkillsPage() {
  return (
    <main className="overflow-x-hidden">
      <section className="relative isolate overflow-hidden px-8 pt-24 pb-18 md:px-16 md:pt-28 md:pb-24">
        <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(174,198,255,0.22),transparent_36%),radial-gradient(circle_at_82%_18%,rgba(0,220,229,0.14),transparent_22%),radial-gradient(circle_at_50%_100%,rgba(255,177,192,0.12),transparent_34%)]" />
        <div className="relative mx-auto grid max-w-[1440px] gap-10 lg:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.9fr)] lg:items-start">
          <div>
            <span className="mb-5 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-4 py-2 font-headline text-xs uppercase tracking-[0.28em] text-primary">
              AI agent Office document skills
            </span>
            <h1 className="max-w-5xl font-headline text-5xl font-bold tracking-tight text-white md:text-6xl lg:text-7xl">
              OfficeCLI Skills for Claude Code, Codex, and AI Agents
            </h1>
            <p className="mt-6 max-w-4xl text-lg leading-relaxed text-outline-variant md:text-xl">
              <strong className="text-white">OfficeCLI Skills</strong> is the public GitHub repository and install surface for Claude Code,
              Codex, and other AI agents that need local <strong className="text-white">PPTX</strong>,{' '}
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
                to="/docs"
              >
                Read product docs
              </Link>
              <Link
                className="rounded-full border border-outline-variant/20 px-6 py-3 font-semibold text-white transition-colors hover:border-primary/30 hover:text-primary"
                to="/download"
              >
                Install OfficeCLI
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
                <CodeBlock command={'officecli --version\nofficecli config status\nofficecli agent-bridge'} />
              </div>
            </article>
            <article className="rounded-3xl border border-outline-variant/10 bg-background/80 p-6">
              <h3 className="font-headline text-2xl font-bold text-white">Related entrypoints</h3>
              <div className="mt-5 space-y-4 text-sm leading-relaxed">
                <a className="block text-primary transition-colors hover:text-tertiary" href={githubRepoURL}>
                  GitHub: officecli/officecli-skills
                </a>
                <Link className="block text-primary transition-colors hover:text-tertiary" to="/docs">
                  Product docs for OfficeCLI formats and agent bridge
                </Link>
                <Link className="block text-primary transition-colors hover:text-tertiary" to="/download">
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
            {faqs.map((faq) => (
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
