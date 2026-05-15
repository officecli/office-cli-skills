import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import InstallTabs from '../components/InstallTabs'
import Pricing from '../components/Pricing'
import CodeBlock from '../components/CodeBlock'
import { OfficeCliBrand } from '../components/branding'
import {
  agentSkillHighlights,
  commandGroups,
  docsLinks,
  inviteRewardChecklists,
  inviteRewardRules,
  inviteRewardSteps,
  docsSections,
  promptExamples,
  promptingTips,
  publishGuide,
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
    <div id={id} className="scroll-mt-16 mb-8">
      <h2 className="font-headline text-3xl md:text-4xl font-bold text-white tracking-tight mb-4">{title}</h2>
      {description ? <p className="text-outline-variant text-lg leading-relaxed max-w-3xl">{description}</p> : null}
    </div>
  )
}

export default function DocsPage() {
  const [activeSection, setActiveSection] = useState<string>('')

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        // Find the first intersecting entry
        const visibleEntries = entries.filter((entry) => entry.isIntersecting)
        if (visibleEntries.length > 0) {
          // Sort by intersection ratio to get the most visible one, or just take the first
          setActiveSection(visibleEntries[0].target.id)
        }
      },
      {
        rootMargin: '-100px 0px -60% 0px',
        threshold: 0,
      }
    )

    docsSections.forEach((section) => {
      const el = document.getElementById(section.id)
      if (el) observer.observe(el)
    })

    return () => observer.disconnect()
  }, [])

  return (
    <main className="mx-auto max-w-[1440px] px-8 md:px-16 pt-16 pb-24 flex flex-col md:flex-row gap-12 lg:gap-24">
      {/* Mobile Nav (Dropdown or simple list) */}
      <div className="md:hidden mb-8">
        <div className="text-primary font-headline text-xs uppercase tracking-widest mb-4">Table of Contents</div>
        <div className="flex flex-wrap gap-2">
          {docsSections.map((section) => (
            <a
              key={section.id}
              href={`#${section.id}`}
              className={`rounded-full border px-4 py-2 text-xs font-headline uppercase tracking-widest transition-colors ${
                activeSection === section.id
                  ? 'border-primary/50 bg-primary/10 text-primary'
                  : 'border-outline-variant/20 bg-surface-low text-outline-variant hover:border-primary/30 hover:text-white'
              }`}
            >
              {section.label}
            </a>
          ))}
        </div>
      </div>

      {/* Sidebar Nav (Desktop) */}
      <aside className="hidden md:block w-64 shrink-0">
        <nav className="sticky top-16 flex flex-col gap-1">
          <div className="text-primary font-headline text-xs uppercase tracking-widest mb-4 px-3">On this page</div>
          {docsSections.map((section) => {
            const isActive = activeSection === section.id
            return (
              <a
                key={section.id}
                href={`#${section.id}`}
                className={`px-3 py-2 rounded-lg text-sm font-headline uppercase tracking-widest transition-colors ${
                  isActive
                    ? 'bg-primary/10 text-primary'
                    : 'text-outline-variant hover:bg-surface-low hover:text-white'
                }`}
              >
                {section.label}
              </a>
            )
          })}
        </nav>
      </aside>

      {/* Content */}
      <div className="flex-1 min-w-0 max-w-4xl">
        <section className="mb-16">
          <Link to="/" className="inline-flex">
            <OfficeCliBrand
              className="mb-8"
              markClassName="h-12 w-12"
              titleClassName="text-2xl font-bold tracking-tight text-white"
              subtitle="product docs / local-first runtime"
            />
          </Link>
          <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Docs</span>
          <h1 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">
            OfficeCLI documentation for AI document generation
          </h1>
          <p className="text-outline-variant text-lg leading-relaxed">
            Use this page as the product docs hub for installation, command patterns, prompting guidance, agent integration, OpenClaw setup, troubleshooting, and pricing-aware usage rules.
          </p>
        </section>

        <div className="space-y-24">
          <section>
            <SectionHeading
              id="quickstart"
              title="Quickstart"
              description="Install OfficeCLI, configure access and generation, then create your first PPTX, DOCX, XLSX, or REPORT output."
            />
            <div className="grid gap-6 md:grid-cols-3">
              {quickstartChecklist.map((group) => (
                <article key={group.title} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
                  <h3 className="font-headline text-xl font-bold text-white mb-4">{group.title}</h3>
                  <ul className="space-y-3 text-sm leading-relaxed text-outline-variant">
                    {group.items.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                </article>
              ))}
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="install-update-uninstall"
              title="Install / Update / Uninstall"
              description="Choose the install path that matches your environment, then use the right upgrade or removal command for that channel."
            />
            <div className="mb-10">
              <InstallTabs
                compact
                headline="Install OfficeCLI on macOS or Linux"
                intro="Use Homebrew, npm, the Linux install script, or manual release archives. Pick one channel and keep using it for future updates."
              />
            </div>
            <div className="grid gap-8 lg:grid-cols-2">
              <article>
                <h3 className="font-headline text-2xl font-bold text-white mb-6">Update methods</h3>
                <div className="space-y-6">
                  {updateMethods.map((item) => (
                    <div key={item.command}>
                      <CodeBlock command={item.command} label={item.label} />
                      <p className="mt-3 text-sm leading-relaxed text-outline-variant">{item.detail}</p>
                    </div>
                  ))}
                </div>
              </article>
              <article>
                <h3 className="font-headline text-2xl font-bold text-white mb-6">Uninstall methods</h3>
                <div className="space-y-6">
                  {uninstallMethods.map((item) => (
                    <div key={item.command}>
                      <CodeBlock command={item.command} label={item.label} />
                      <p className="mt-3 text-sm leading-relaxed text-outline-variant">{item.detail}</p>
                    </div>
                  ))}
                </div>
              </article>
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="command-reference"
              title="Command Reference"
              description="The current surface centers on configuration, access checks, generation, review, upgrade, and the agent bridge."
            />
            <div className="space-y-12">
              {commandGroups.map((group) => (
                <article key={group.title}>
                  <h3 className="font-headline text-2xl font-bold text-white mb-4">{group.title}</h3>
                  <CodeBlock command={group.command} />
                  <p className="mt-4 text-outline-variant leading-relaxed">{group.summary}</p>
                  {group.notes?.length ? (
                    <ul className="mt-4 space-y-2 text-sm text-outline-variant list-disc list-inside">
                      {group.notes.map((note) => (
                        <li key={note}>{note}</li>
                      ))}
                    </ul>
                  ) : null}
                  {group.examples?.length ? (
                    <div className="mt-6 grid gap-4 lg:grid-cols-2">
                      {group.examples.map((example) => (
                        <div key={example.command}>
                          <CodeBlock command={example.command} label={example.label} />
                          <p className="mt-3 text-sm leading-relaxed text-outline-variant">{example.detail}</p>
                        </div>
                      ))}
                    </div>
                  ) : null}
                </article>
              ))}
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="online-publish"
              title="Online Publish"
              description="One-command online publish is the differentiating share path of OfficeCLI. After every successful generation, the CLI returns a password-protected `officecli.io/p/<id>` preview URL — no extra hosting, gateway, or upload step. Other terminal-first AI document CLIs stop at a local file."
            />
            <p className="text-outline-variant leading-relaxed max-w-3xl mb-8">{publishGuide.intro}</p>

            <div className="grid gap-6 md:grid-cols-2 mb-12">
              {publishGuide.highlights.map((tip) => (
                <article key={tip.title} className="rounded-2xl border border-primary/15 bg-surface-low p-6">
                  <h3 className="font-headline text-xl font-bold text-white mb-3">{tip.title}</h3>
                  <p className="text-outline-variant leading-relaxed text-sm">{tip.detail}</p>
                </article>
              ))}
            </div>

            <h3 className="font-headline text-2xl font-bold text-white mb-6">Configure publish</h3>
            <div className="grid gap-6 lg:grid-cols-2 mb-12">
              {publishGuide.configCommands.map((item) => (
                <div key={item.command}>
                  <CodeBlock command={item.command} label={item.label} />
                  <p className="mt-3 text-sm leading-relaxed text-outline-variant">{item.detail}</p>
                </div>
              ))}
            </div>

            <h3 className="font-headline text-2xl font-bold text-white mb-4">Expected output</h3>
            <p className="text-outline-variant leading-relaxed mb-4 max-w-3xl">
              When publish is enabled, every successful run prints a Preview URL and an auto-generated access password right after the local file path. Treat the URL plus password as a single share unit:
            </p>
            <div className="rounded-2xl border border-outline-variant/10 bg-surface-low overflow-hidden mb-12">
              <div className="flex items-center justify-between px-5 py-3 border-b border-outline-variant/10 bg-surface">
                <div className="text-xs font-headline uppercase tracking-widest text-tertiary">terminal output</div>
              </div>
              <div className="px-5 py-4">
                <pre className="whitespace-pre-wrap text-sm leading-relaxed text-outline-variant font-mono">{publishGuide.outputShape.join('\n')}</pre>
              </div>
            </div>

            <h3 className="font-headline text-2xl font-bold text-white mb-4">Environment variables</h3>
            <p className="text-outline-variant leading-relaxed mb-6 max-w-3xl">
              Use these to flip publish behavior in CI, batch runs, or self-hosted publish gateways without rerunning <code>set-publish</code>.
            </p>
            <div className="grid gap-4 md:grid-cols-2">
              {publishGuide.envVars.map((item) => (
                <article key={item.name} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-5">
                  <code className="text-primary font-mono text-sm font-bold">{item.name}</code>
                  <p className="mt-2 text-sm leading-relaxed text-outline-variant">{item.detail}</p>
                </article>
              ))}
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="prompting-tips"
              title="Prompting Tips"
              description="Prompt quality is still the strongest lever for predictable AI document generation."
            />
            <div className="grid gap-6 md:grid-cols-2">
              {promptingTips.map((tip) => (
                <article key={tip.title} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
                  <h3 className="font-headline text-xl font-bold text-white mb-3">{tip.title}</h3>
                  <p className="text-outline-variant leading-relaxed text-sm">{tip.detail}</p>
                </article>
              ))}
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="prompt-cookbook"
              title="Prompt Cookbook"
              description="These examples show both direct prompts and prompt-file flows such as --prompt-file for reusable templates."
            />
            <div className="space-y-12">
              {promptExamples.map((example) => (
                <article key={example.title}>
                  <div className="text-xs font-headline uppercase tracking-widest text-primary mb-3">{example.format}</div>
                  <h3 className="font-headline text-2xl font-bold text-white mb-6">{example.title}</h3>
                  <div className="grid gap-6 lg:grid-cols-2 mb-6">
                    <div>
                      <CodeBlock command={example.directCommand} label="Direct prompt" />
                    </div>
                    <div>
                      <CodeBlock command={example.fileCommand} label="Prompt file" />
                    </div>
                  </div>
                  <div className="rounded-2xl border border-outline-variant/10 bg-surface-low overflow-hidden">
                    <div className="flex items-center justify-between px-5 py-3 border-b border-outline-variant/10 bg-surface">
                      <div className="text-xs font-headline uppercase tracking-widest text-tertiary">{example.promptFileName}</div>
                    </div>
                    <div className="px-5 py-4">
                      <pre className="whitespace-pre-wrap text-sm leading-relaxed text-outline-variant font-mono">{example.prompt}</pre>
                    </div>
                  </div>
                </article>
              ))}
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="agents"
              title="Use With Agents"
              description="OfficeCLI already exposes agent-oriented workflows for Codex, Claude Code, and local automation clients."
            />
            <div className="mb-8 rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
              <p className="max-w-3xl text-sm leading-relaxed text-outline-variant">
                Need the public landing page for search-driven installs and GitHub repository context? Visit{' '}
                <Link className="text-primary transition-colors hover:text-tertiary" to="/claude-code-codex-office-skills">
                  OfficeCLI for Claude Code, Codex, and AI agents
                </Link>
                .
              </p>
            </div>
            <div className="grid gap-6 md:grid-cols-2">
              {agentSkillHighlights.map((group) => (
                <article key={group.title} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
                  <h3 className="font-headline text-xl font-bold text-white mb-4">{group.title}</h3>
                  <ul className="space-y-3 text-sm leading-relaxed text-outline-variant">
                    {group.items.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                </article>
              ))}
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="openclaw"
              title="Use With OpenClaw"
              description="OpenClaw clients should install the skill bundle, then use the OfficeCLI bridge instead of parsing human CLI output."
            />
            <div className="grid gap-6 md:grid-cols-2">
              <article>
                <CodeBlock command={docsLinks.openClawInstallCommand} label="Install" />
              </article>
              <article>
                <CodeBlock command={docsLinks.codexBridgeCommand} label="Bridge" />
              </article>
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="pricing-rules"
              title="Pricing & Usage Rules"
              description="External Mode is free and unlimited with your own model endpoint. Hosted Mode uses hosted credits surfaced through the public pricing API and billing workspace."
            />
            <div className="grid gap-6 md:grid-cols-2 mb-10">
              {usageRules.map((rule) => (
                <article key={rule.title} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
                  <h3 className="font-headline text-xl font-bold text-white mb-3">{rule.title}</h3>
                  <p className="text-outline-variant leading-relaxed text-sm">{rule.detail}</p>
                </article>
              ))}
            </div>
            <div className="flex flex-wrap gap-4 mb-12">
              <a className="rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary transition-colors" href={docsLinks.platformAppURL}>
                Open platform app
              </a>
              <a className="rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary transition-colors" href={docsLinks.platformBillingURL}>
                Open billing workspace
              </a>
              <Link className="rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary transition-colors" to="/download">
                Install the CLI
              </Link>
            </div>
            <div className="rounded-3xl border border-outline-variant/10 bg-surface-low p-8">
              <Pricing standalone />
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="invite-rewards"
              title="Invite Rewards"
              description="Use this flow when you want to invite a teammate, understand when hosted credits are granted, and know where to verify each stage in the app."
            />
            <div className="grid gap-6 md:grid-cols-3 mb-10">
              {inviteRewardSteps.map((step) => (
                <article key={step.title} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
                  <h3 className="font-headline text-xl font-bold text-white mb-3">{step.title}</h3>
                  <p className="text-outline-variant leading-relaxed text-sm">{step.detail}</p>
                </article>
              ))}
            </div>
            <div className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr] mb-10">
              <article className="rounded-3xl border border-outline-variant/10 bg-surface-low p-6">
                <h3 className="font-headline text-2xl font-bold text-white mb-4">Invite link template</h3>
                <CodeBlock command={docsLinks.inviteLinkTemplate} label="Share this pattern" />
                <p className="mt-4 text-sm leading-relaxed text-outline-variant">
                  Replace <code>&lt;your-invite-code&gt;</code> with the code shown in the app. The invited user must finish Google login from this invite-bearing URL so the backend can register the referral.
                </p>
              </article>
              <article className="space-y-6">
                {inviteRewardChecklists.map((group) => (
                  <div key={group.title} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
                    <h3 className="font-headline text-xl font-bold text-white mb-4">{group.title}</h3>
                    <ul className="space-y-3 text-sm leading-relaxed text-outline-variant">
                      {group.items.map((item) => (
                        <li key={item}>{item}</li>
                      ))}
                    </ul>
                  </div>
                ))}
              </article>
            </div>
            <div className="grid gap-6 md:grid-cols-2 mb-10">
              {inviteRewardRules.map((rule) => (
                <article key={rule.title} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
                  <h3 className="font-headline text-xl font-bold text-white mb-3">{rule.title}</h3>
                  <p className="text-outline-variant leading-relaxed text-sm">{rule.detail}</p>
                </article>
              ))}
            </div>
            <div className="rounded-3xl border border-outline-variant/10 bg-surface-low p-8 mb-10">
              <div className="text-primary font-headline text-xs uppercase tracking-widest mb-3">Current scope</div>
              <div className="text-white text-lg font-semibold mb-3">What this guide intentionally does and does not promise</div>
              <ul className="space-y-3 text-sm leading-relaxed text-outline-variant">
                <li>Invite rewards currently describe the invite code, registration, activation, and account hosted credits flow only.</li>
                <li>Discord verification grants 100 account hosted credits after a trusted membership check succeeds.</li>
                <li>Invite attribution analytics remain limited, so use the in-app referral timeline and hosted credits ledger as the primary source of truth.</li>
              </ul>
            </div>
            <div className="flex flex-wrap gap-4">
              <a className="rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary transition-colors" href={docsLinks.platformAppURL}>
                Open platform overview
              </a>
              <a className="rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary transition-colors" href={docsLinks.platformQuotaURL}>
                Open quota page
              </a>
            </div>
          </section>

          <div className="h-px bg-outline-variant/10 w-full" />

          <section>
            <SectionHeading
              id="troubleshooting"
              title="Troubleshooting"
              description="Use these checks first when generation output, images, publish links, or access behavior look wrong."
            />
            <div className="grid gap-6 md:grid-cols-2">
              {troubleshootingTips.map((tip) => (
                <article key={tip.title} className="rounded-2xl border border-outline-variant/10 bg-surface-low p-6">
                  <h3 className="font-headline text-xl font-bold text-white mb-3">{tip.title}</h3>
                  <p className="text-outline-variant leading-relaxed text-sm">{tip.detail}</p>
                </article>
              ))}
            </div>
          </section>
        </div>
      </div>
    </main>
  )
}
