import { Link } from 'react-router-dom'
import CodeBlock from '../components/CodeBlock'
import AgentSkillsBreadcrumbs from '../components/AgentSkillsBreadcrumbs'
import AgentSkillsSecondaryNav from '../components/AgentSkillsSecondaryNav'
import { getAgentSkillsBreadcrumbs, githubRepoURL } from '../agentSkillsData'

const openClawInstallCommand =
  'curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-openclaw-skill.sh | bash'
const openClawConfigSnippet = `agents:
  office-bot:
    model: openai/gpt-4o
    channels: [telegram]
    skills: [openclaw-officecli]
    tools: [shell, file_read]`

export default function AgentSkillsOpenClawPage() {
  return (
    <main className="overflow-x-hidden px-8 pt-24 pb-18 md:px-16 md:pt-28 md:pb-24">
      <div className="mx-auto max-w-[1440px]">
        <AgentSkillsBreadcrumbs items={getAgentSkillsBreadcrumbs('/officecli/openclaw')} />
        <section className="rounded-[36px] border border-outline-variant/10 bg-surface-low px-8 py-10 shadow-[0_24px_70px_rgba(0,0,0,0.24)] md:px-10">
          <span className="inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-4 py-2 font-headline text-xs uppercase tracking-[0.28em] text-primary">
            officecli for OpenClaw
          </span>
          <h1 className="mt-6 max-w-4xl font-headline text-5xl font-bold tracking-tight text-white md:text-6xl">
            Structured Office workflows through officecli agent-bridge
          </h1>
          <p className="mt-5 max-w-4xl text-lg leading-relaxed text-outline-variant">
            This path is for OpenClaw users who need channel-based Office generation and should land on the OpenClaw-specific installer instead of a generic skill page or random bridge file.
          </p>
          <div className="mt-8">
            <AgentSkillsSecondaryNav currentPath="/officecli/openclaw" />
          </div>
        </section>

        <section className="mt-10 grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Install the OpenClaw package</h2>
            <p className="mt-4 text-sm leading-relaxed text-outline-variant">
              Use the OpenClaw-oriented installer when the skill should generate Office files through `officecli agent-bridge` and return them back to Telegram, Discord, Slack, or similar chat channels.
            </p>
            <div className="mt-6">
              <CodeBlock command={openClawInstallCommand} />
            </div>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Attach the skill to an agent</h2>
            <p className="mt-4 text-sm leading-relaxed text-outline-variant">
              After install, wire the skill into the OpenClaw agent config and keep `shell` plus `file_read` available on the same machine.
            </p>
            <div className="mt-6">
              <CodeBlock command={openClawConfigSnippet} />
            </div>
          </article>
        </section>

        <section className="mt-10 grid gap-6 md:grid-cols-3">
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Bridge check</h2>
            <div className="mt-4">
              <CodeBlock command={'officecli agent-bridge'} />
            </div>
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
              Compare install routes and verification
            </Link>
          </article>
        </section>
      </div>
    </main>
  )
}
