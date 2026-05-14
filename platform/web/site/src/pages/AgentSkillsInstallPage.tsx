import { Link } from 'react-router-dom'
import CodeBlock from '../components/CodeBlock'
import AgentSkillsBreadcrumbs from '../components/AgentSkillsBreadcrumbs'
import AgentSkillsSecondaryNav from '../components/AgentSkillsSecondaryNav'
import {
  agentSkillsHubPath,
  downloadPath,
  getAgentSkillsBreadcrumbs,
  githubRepoURL,
  installPaths,
  productDocsPath,
  verificationCommands,
} from '../agentSkillsData'

export default function AgentSkillsInstallPage() {
  return (
    <main className="overflow-x-hidden px-8 pt-24 pb-18 md:px-16 md:pt-28 md:pb-24">
      <div className="mx-auto max-w-[1440px]">
        <AgentSkillsBreadcrumbs items={getAgentSkillsBreadcrumbs('/officecli/install')} />
        <section className="rounded-[36px] border border-outline-variant/10 bg-[radial-gradient(circle_at_top_left,rgba(174,198,255,0.18),transparent_34%),radial-gradient(circle_at_78%_18%,rgba(0,220,229,0.12),transparent_22%),#0d1117] px-8 py-10 shadow-[0_30px_90px_rgba(0,0,0,0.28)] md:px-10">
          <span className="inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-4 py-2 font-headline text-xs uppercase tracking-[0.28em] text-primary">
            Install officecli
          </span>
          <h1 className="mt-6 max-w-4xl font-headline text-5xl font-bold tracking-tight text-white md:text-6xl">
            Choose the install path that matches the agent runtime
          </h1>
          <p className="mt-5 max-w-4xl text-lg leading-relaxed text-outline-variant">
            `officecli` has three public install surfaces: Claude Code marketplace install, direct local install for Codex-style agents, and the OpenClaw-oriented installer. The right page should be one click away instead of buried in a long README.
          </p>
          <div className="mt-8">
            <AgentSkillsSecondaryNav currentPath="/officecli/install" />
          </div>
        </section>

        <section className="mt-10 grid gap-8 xl:grid-cols-3">
          {installPaths.map((item) => (
            <article key={item.title} className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
              <h2 className="font-headline text-2xl font-bold text-white">{item.title}</h2>
              <p className="mt-4 text-sm leading-relaxed text-outline-variant">{item.intro}</p>
              <div className="mt-6">
                <CodeBlock command={item.command} />
              </div>
            </article>
          ))}
        </section>

        <section className="mt-10 grid gap-6 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Verify the local runtime after install</h2>
            <p className="mt-4 text-sm leading-relaxed text-outline-variant">
              The public repo solves discovery, packaging, and routing. The final Office artifact still depends on a working local OfficeCLI runtime.
            </p>
            <div className="mt-6">
              <CodeBlock command={verificationCommands} />
            </div>
          </article>
          <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
            <h2 className="font-headline text-2xl font-bold text-white">Next entrypoints</h2>
            <div className="mt-5 space-y-4 text-sm leading-relaxed">
              <Link className="block text-primary transition-colors hover:text-tertiary" to={`${agentSkillsHubPath}/claude-code`}>
                Claude Code marketplace install details
              </Link>
              <Link className="block text-primary transition-colors hover:text-tertiary" to={`${agentSkillsHubPath}/codex`}>
                Codex direct local install details
              </Link>
              <Link className="block text-primary transition-colors hover:text-tertiary" to={`${agentSkillsHubPath}/openclaw`}>
                OpenClaw bridge install details
              </Link>
              <Link className="block text-primary transition-colors hover:text-tertiary" to={productDocsPath}>
                Product docs for OfficeCLI formats and bridge behavior
              </Link>
              <Link className="block text-primary transition-colors hover:text-tertiary" to={downloadPath}>
                Download and install OfficeCLI itself
              </Link>
              <a className="block text-primary transition-colors hover:text-tertiary" href={githubRepoURL}>
                GitHub: officecli/officecli
              </a>
            </div>
          </article>
        </section>
      </div>
    </main>
  )
}
