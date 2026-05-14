import { useEffect, useState } from 'react'
import { CheckCheck, Copy, Mail, MessageSquare, Megaphone } from 'lucide-react'

const supportEmail = 'support@officecli.io'
const discordInviteURL = 'https://discord.gg/ezAHMkdG'
const xProfileURL = 'https://x.com/officecli'

const socialPlaceholders = [
  {
    label: 'Discord',
    description: 'Community room and product discussion.',
    icon: MessageSquare,
    href: discordInviteURL,
  },
  {
    label: 'X',
    description: 'Product updates and release highlights.',
    icon: Megaphone,
    href: xProfileURL,
  },
]

export default function Contact() {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 1500)
    return () => window.clearTimeout(timer)
  }, [copied])

  async function copyEmail() {
    try {
      await navigator.clipboard?.writeText(supportEmail)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  return (
    <section className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="rounded-[32px] border border-outline-variant/10 bg-surface-low p-8 md:p-12">
        <div className="grid grid-cols-1 xl:grid-cols-[1.2fr_0.8fr] gap-8">
          <div>
            <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Contact</span>
            <h2 className="font-headline text-4xl md:text-5xl font-bold text-white tracking-tight mb-5">Need help evaluating OfficeCLI for your workflow?</h2>
            <p className="text-outline-variant text-lg leading-relaxed max-w-2xl mb-8">
              Reach out for setup questions, roadmap feedback, enterprise evaluation, or help deciding how OfficeCLI should fit into your document operations stack.
            </p>

            <div className="bg-background border border-outline-variant/10 rounded-3xl p-6">
              <div className="flex items-center gap-3 text-primary mb-4">
                <Mail size={18} />
                <span className="text-xs font-headline uppercase tracking-widest">Email support</span>
              </div>
              <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-5">
                <div>
                  <div className="font-mono text-lg md:text-2xl text-white break-all">{supportEmail}</div>
                  <div className="text-sm text-outline-variant mt-2">Click to copy the address and contact the team directly.</div>
                </div>
                <div className="flex flex-wrap gap-3">
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-full bg-gradient-to-br from-primary to-primary-container px-5 py-3 font-bold text-[#002e6b]"
                    onClick={copyEmail}
                  >
                    {copied ? <CheckCheck size={16} /> : <Copy size={16} />}
                    {copied ? 'Copied' : 'Copy email'}
                  </button>
                  <a
                    className="inline-flex items-center gap-2 rounded-full border border-outline-variant/20 px-5 py-3 font-semibold text-white hover:border-primary/30 hover:text-primary"
                    href={`mailto:${supportEmail}`}
                  >
                    Email us
                  </a>
                </div>
              </div>
            </div>
          </div>

          <div className="grid gap-4">
            {socialPlaceholders.map((item) => {
              const Icon = item.icon
              const cardClassName =
                'rounded-3xl border border-outline-variant/10 bg-background/70 p-6 transition-colors'
              const cardContent = (
                <>
                  <div className="flex items-center justify-between gap-4 mb-4">
                    <div className="inline-flex items-center gap-3 text-white">
                      <Icon size={18} className="text-tertiary" />
                      <span className="font-semibold">{item.label}</span>
                    </div>
                    <span className="rounded-full border border-outline-variant/15 px-3 py-1 text-[10px] font-headline uppercase tracking-widest text-outline-variant">
                      {item.href ? 'Join now' : 'Coming soon'}
                    </span>
                  </div>
                  <p className="text-sm leading-relaxed text-outline-variant">{item.description}</p>
                </>
              )

              if (item.href) {
                return (
                  <a
                    key={item.label}
                    className={`${cardClassName} block hover:border-primary/30 hover:bg-background`}
                    href={item.href}
                    aria-label={item.label}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {cardContent}
                  </a>
                )
              }

              return (
                <div key={item.label} className={cardClassName}>
                  {cardContent}
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </section>
  )
}
