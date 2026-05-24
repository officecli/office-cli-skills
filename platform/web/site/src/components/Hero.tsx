import { motion } from 'motion/react'
import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import { githubRepoURL, officedexRepoURL, platformAppURL } from '../siteData'
import { DiscordMark, GithubMark, XMark } from './branding'
import HeroOutputFormats from './HeroOutputFormats'
import HeroProductTabs from './HeroProductTabs'

function InlineCode({ children }: { children: string }) {
  return (
    <code className="rounded border border-outline-variant/15 bg-black/40 px-1.5 py-0.5 font-mono text-[0.95em] text-primary">
      {children}
    </code>
  )
}

const HERO_SOCIAL_LINKS = [
  {
    label: 'officecli on GitHub',
    title: 'officecli',
    description: 'CLI source repo',
    href: githubRepoURL,
    icon: GithubMark,
    className: 'border-outline-variant/25 bg-surface-high/40 hover:border-outline-variant/45 hover:bg-surface-high/60',
    iconClassName: 'text-white',
  },
  {
    label: 'officedex on GitHub',
    title: 'officedex',
    description: 'Desktop app repo',
    href: officedexRepoURL,
    icon: GithubMark,
    className: 'border-outline-variant/25 bg-surface-high/40 hover:border-outline-variant/45 hover:bg-surface-high/60',
    iconClassName: 'text-white',
  },
  {
    label: 'Join Discord',
    title: 'Discord',
    description: 'Community room',
    href: 'https://discord.gg/ezAHMkdG',
    icon: DiscordMark,
    className: 'border-tertiary/25 bg-tertiary/10 hover:border-tertiary/40 hover:bg-tertiary/15',
    iconClassName: 'text-tertiary',
  },
  {
    label: 'Follow on X',
    title: 'X / Twitter',
    description: 'Release updates',
    href: 'https://x.com/officecli',
    icon: XMark,
    className: 'border-primary/25 bg-primary/10 hover:border-primary/40 hover:bg-primary/15',
    iconClassName: 'text-primary',
  },
]

export default function Hero() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)

  return (
    <section className="relative px-6 pt-24 pb-12 md:px-10 lg:px-16 max-w-[1440px] mx-auto">
      <motion.div
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5 }}
        className="grid grid-cols-1 gap-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end"
      >
        <div className="min-w-0">
          <span className="mb-4 inline-flex max-w-full items-center gap-2 rounded-full border border-[#0d4c54]/60 bg-[#0d4c54]/40 px-3 py-1 text-[11px] font-headline uppercase tracking-widest text-[#8af3f7]">
            <span className="h-2 w-2 rounded-full bg-tertiary terminal-pulse shadow-[0_0_8px_#00dce5]" />
            One binary · Three surfaces · Plus a desktop app
          </span>
          <div className="flex flex-wrap items-baseline gap-x-5 gap-y-2">
            <h1 className="font-mono text-4xl font-extrabold tracking-tight text-white sm:text-5xl lg:text-6xl">
              OfficeCLI
            </h1>
            <p className="font-headline text-sm uppercase tracking-[0.18em] text-outline-variant sm:text-base">
              AI document generation, on every surface
            </p>
          </div>
          <p className="mt-4 max-w-3xl text-base leading-relaxed text-outline-variant sm:text-lg">
            Run <InlineCode>officecli new pptx …</InlineCode> from scripts; just{' '}
            <InlineCode>officecli</InlineCode> to enter the interactive terminal—same binary. For non-terminal users, the{' '}
            <InlineCode>officedex</InlineCode> desktop app is here.
          </p>

          <div className="mt-6 flex flex-wrap gap-3">
            <motion.a
              whileHover={{ scale: 0.97 }}
              className="rounded-md bg-gradient-to-br from-primary to-primary-container px-6 py-3 text-base font-bold text-[#002e6b] transition-all"
              href={platformAppHref}
              onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.consoleOpen, { surface: 'site', placement: 'hero', ...extractAttributionParams(location.search) })}
            >
              Start Building for Free
            </motion.a>
            <Link
              className="inline-flex rounded-md border border-outline-variant/30 px-6 py-3 text-base font-bold text-white transition-all hover:bg-white/5"
              to="/download"
            >
              Install the AI CLI
            </Link>
          </div>
        </div>

        <div className="flex w-full flex-col gap-2 lg:w-72 lg:items-stretch">
          {HERO_SOCIAL_LINKS.map((item) => {
            const Icon = item.icon
            return (
              <a
                key={item.label}
                className={`group/social flex items-center justify-between gap-3 rounded-lg border px-3 py-2.5 transition-colors ${item.className}`}
                href={item.href}
                aria-label={item.label}
                target="_blank"
                rel="noreferrer"
              >
                <span className="inline-flex min-w-0 items-center gap-2">
                  <Icon className={`h-4 w-4 shrink-0 ${item.iconClassName}`} />
                  <span className="min-w-0">
                    <span className="block font-headline text-[11px] uppercase tracking-widest text-white">
                      {item.title}
                    </span>
                    <span className="block truncate text-xs text-outline-variant">{item.description}</span>
                  </span>
                </span>
                <span className="shrink-0 rounded border border-outline-variant/15 px-2 py-0.5 text-[9px] font-headline uppercase tracking-wider text-outline-variant transition-colors group-hover/social:text-white">
                  Open
                </span>
              </a>
            )
          })}
        </div>
      </motion.div>

      <HeroProductTabs />
      <HeroOutputFormats />
    </section>
  )
}
