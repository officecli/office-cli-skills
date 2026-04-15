import { Link } from 'react-router-dom'
import InstallTabs from '../components/InstallTabs'
import Pricing from '../components/Pricing'
import {
  agentSkillHighlights,
  commandGroups,
  docsLinks,
  docsSections,
  promptExamples,
  promptingTips,
  quickstartChecklist,
  troubleshootingTips,
  uninstallMethods,
  updateMethods,
  usageRules,
} from '../docsData'

function SectionHeading({
  id,
  title,
  description,
}: {
  id: string
  title: string
  description?: string
}) {
  return (
    <div id={id} className="scroll-mt-28">
      <h2 className="font-headline text-3xl md:text-4xl font-bold text-white tracking-tight mb-4">{title}</h2>
      {description ? <p className="text-outline-variant text-lg leading-relaxed max-w-3xl">{description}</p> : null}
    </div>
  )
}

export default function DocsPage() {
  return (
    <main className="overflow-x-hidden pt-28 px-8 md:px-16 max-w-[1440px] mx-auto pb-24">
      <section className="max-w-4xl">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Docs</span>
        <h1 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">
          OfficeCLI documentation for AI document generation and REPORT workflows
        </h1>
        <p className="text-outline-variant text-lg leading-relaxed max-w-3xl">
          Use this page as the product docs hub for installation, command patterns, prompting guidance, agent integration, OpenClaw setup, troubleshooting, and pricing-aware usage rules.
        </p>
      </section>

      <section className="mt-12 grid gap-4 md:grid-cols-3 xl:grid-cols-5">
        {docsSections.map((section) => (
          <a
            key={section.id}
            className="rounded-2xl border border-outline-variant/10 bg-surface-low px-5 py-4 text-sm font-headline uppercase tracking-widest text-outline-variant hover:border-primary/30 hover:text-white"
            href={`#${section.id}`}
          >
            {section.label}
          </a>
        ))}
      </section>

      <section className="mt-16 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="quickstart"
          title="Quickstart"
          description="Install OfficeCLI, configure access and generation, then create your first PPTX, DOCX, XLSX, or REPORT output."
        />
        <div className="mt-10 grid gap-6 md:grid-cols-3">
          {quickstartChecklist.map((group) => (
            <article key={group.title} className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
              <h3 className="font-headline text-2xl font-bold text-white mb-4">{group.title}</h3>
              <ul className="space-y-3 text-sm leading-relaxed text-outline-variant">
                {group.items.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </article>
          ))}
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="install-update-uninstall"
          title="Install / Update / Uninstall"
          description="Choose the install path that matches your environment, then use the right upgrade or removal command for that channel."
        />
        <div className="mt-10">
          <InstallTabs
            compact
            headline="Install OfficeCLI on macOS or Linux"
            intro="Use Homebrew, npm, the Linux install script, or manual release archives. Pick the channel you want to keep using for future updates."
          />
        </div>
        <div className="mt-10 grid gap-6 lg:grid-cols-2">
          <article className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
            <h3 className="font-headline text-2xl font-bold text-white mb-4">Update methods</h3>
            <div className="space-y-4">
              {updateMethods.map((item) => (
                <div key={item.command} className="rounded-2xl border border-outline-variant/10 bg-background px-5 py-4">
                  <div className="text-xs font-headline uppercase tracking-widest text-primary mb-2">{item.label}</div>
                  <code className="block text-sm text-white break-all">{item.command}</code>
                  <p className="mt-3 text-sm leading-relaxed text-outline-variant">{item.detail}</p>
                </div>
              ))}
            </div>
          </article>
          <article className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
            <h3 className="font-headline text-2xl font-bold text-white mb-4">Uninstall methods</h3>
            <div className="space-y-4">
              {uninstallMethods.map((item) => (
                <div key={item.command} className="rounded-2xl border border-outline-variant/10 bg-background px-5 py-4">
                  <div className="text-xs font-headline uppercase tracking-widest text-primary mb-2">{item.label}</div>
                  <code className="block text-sm text-white break-all">{item.command}</code>
                  <p className="mt-3 text-sm leading-relaxed text-outline-variant">{item.detail}</p>
                </div>
              ))}
            </div>
          </article>
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="command-reference"
          title="Command Reference"
          description="The current surface centers on configuration, access checks, generation, review, upgrade, and the agent bridge."
        />
        <div className="mt-10 space-y-6">
          {commandGroups.map((group) => (
            <article key={group.title} className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
              <h3 className="font-headline text-2xl font-bold text-white mb-4">{group.title}</h3>
              <code className="block rounded-2xl border border-outline-variant/10 bg-background px-5 py-4 text-sm text-white break-all">
                {group.command}
              </code>
              <p className="mt-4 text-outline-variant leading-relaxed">{group.summary}</p>
              {group.notes?.length ? (
                <ul className="mt-4 space-y-2 text-sm text-outline-variant">
                  {group.notes.map((note) => (
                    <li key={note}>{note}</li>
                  ))}
                </ul>
              ) : null}
              {group.examples?.length ? (
                <div className="mt-6 grid gap-4 lg:grid-cols-2">
                  {group.examples.map((example) => (
                    <div key={example.command} className="rounded-2xl border border-outline-variant/10 bg-background px-5 py-4">
                      <div className="text-xs font-headline uppercase tracking-widest text-tertiary mb-2">{example.label}</div>
                      <code className="block text-sm text-white break-all">{example.command}</code>
                      <p className="mt-3 text-sm leading-relaxed text-outline-variant">{example.detail}</p>
                    </div>
                  ))}
                </div>
              ) : null}
            </article>
          ))}
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="prompting-tips"
          title="Prompting Tips"
          description="Prompt quality is still the strongest lever for predictable AI document generation."
        />
        <div className="mt-10 grid gap-5 md:grid-cols-2">
          {promptingTips.map((tip) => (
            <article key={tip.title} className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
              <h3 className="font-headline text-2xl font-bold text-white mb-3">{tip.title}</h3>
              <p className="text-outline-variant leading-relaxed">{tip.detail}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="prompt-cookbook"
          title="Prompt Cookbook"
          description="These examples show both direct prompts and prompt-file flows such as --prompt-file for reusable templates."
        />
        <div className="mt-10 grid gap-6">
          {promptExamples.map((example) => (
            <article key={example.title} className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
              <div className="text-xs font-headline uppercase tracking-widest text-primary mb-3">{example.format}</div>
              <h3 className="font-headline text-2xl font-bold text-white mb-4">{example.title}</h3>
              <div className="grid gap-4 lg:grid-cols-2">
                <div className="rounded-2xl border border-outline-variant/10 bg-background px-5 py-4">
                  <div className="text-xs font-headline uppercase tracking-widest text-tertiary mb-2">Direct prompt</div>
                  <code className="block text-sm text-white break-all">{example.directCommand}</code>
                </div>
                <div className="rounded-2xl border border-outline-variant/10 bg-background px-5 py-4">
                  <div className="text-xs font-headline uppercase tracking-widest text-tertiary mb-2">Prompt file</div>
                  <code className="block text-sm text-white break-all">{example.fileCommand}</code>
                </div>
              </div>
              <div className="mt-4 rounded-2xl border border-outline-variant/10 bg-background px-5 py-4">
                <div className="text-xs font-headline uppercase tracking-widest text-tertiary mb-2">{example.promptFileName}</div>
                <pre className="whitespace-pre-wrap text-sm leading-relaxed text-outline-variant">{example.prompt}</pre>
              </div>
            </article>
          ))}
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="agents"
          title="Use With Agents"
          description="OfficeCLI already exposes agent-oriented workflows for Codex, Claude Code, and local automation clients."
        />
        <div className="mt-10 grid gap-6 md:grid-cols-2">
          {agentSkillHighlights.map((group) => (
            <article key={group.title} className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
              <h3 className="font-headline text-2xl font-bold text-white mb-4">{group.title}</h3>
              <ul className="space-y-3 text-sm leading-relaxed text-outline-variant">
                {group.items.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </article>
          ))}
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="openclaw"
          title="Use With OpenClaw"
          description="OpenClaw clients should install the skill bundle, then use the OfficeCLI bridge instead of parsing human CLI output."
        />
        <div className="mt-10 grid gap-5 md:grid-cols-2">
          <article className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
            <div className="text-xs font-headline uppercase tracking-widest text-primary mb-2">Install</div>
            <code className="block text-sm text-white break-all">{docsLinks.openClawInstallCommand}</code>
          </article>
          <article className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
            <div className="text-xs font-headline uppercase tracking-widest text-primary mb-2">Bridge</div>
            <code className="block text-sm text-white break-all">{docsLinks.codexBridgeCommand}</code>
          </article>
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="pricing-rules"
          title="Pricing & Usage Rules"
          description="Pricing and quota behavior are surfaced through the platform and can be validated from the public pricing API and billing workspace."
        />
        <div className="mt-10 grid gap-5 md:grid-cols-2">
          {usageRules.map((rule) => (
            <article key={rule.title} className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
              <h3 className="font-headline text-2xl font-bold text-white mb-3">{rule.title}</h3>
              <p className="text-outline-variant leading-relaxed">{rule.detail}</p>
            </article>
          ))}
        </div>
        <div className="mt-8 flex flex-wrap gap-4">
          <a className="rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary" href={docsLinks.platformAppURL}>
            Open platform app
          </a>
          <a className="rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary" href={docsLinks.platformBillingURL}>
            Open billing workspace
          </a>
          <Link className="rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary" to="/download">
            Install the CLI
          </Link>
        </div>
        <div className="mt-8">
          <Pricing standalone />
        </div>
      </section>

      <section className="mt-12 rounded-3xl border border-outline-variant/10 bg-surface-low p-8 md:p-10">
        <SectionHeading
          id="troubleshooting"
          title="Troubleshooting"
          description="Use these checks first when generation output, images, publish links, or access behavior look wrong."
        />
        <div className="mt-10 grid gap-5 md:grid-cols-2">
          {troubleshootingTips.map((tip) => (
            <article key={tip.title} className="rounded-2xl border border-outline-variant/10 bg-background/60 p-6">
              <h3 className="font-headline text-2xl font-bold text-white mb-3">{tip.title}</h3>
              <p className="text-outline-variant leading-relaxed">{tip.detail}</p>
            </article>
          ))}
        </div>
      </section>
    </main>
  )
}
