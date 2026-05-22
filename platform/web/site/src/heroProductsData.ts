import { installOfficeCliPath, officedexNotifyPath } from './siteData'

export type SurfaceId = 'officedex' | 'officecli-tui' | 'officecli'

export type SurfaceCtaVariant = 'primary' | 'ghost' | 'comingSoon'

export interface SurfaceCta {
  label: string
  href: string | null
  variant: SurfaceCtaVariant
  external?: boolean
}

export interface SurfaceData {
  id: SurfaceId
  eyebrow: string
  name: string
  tabSub: string
  lede: string
  features: string[]
  ctas: SurfaceCta[]
  sameBinaryNote?: string
  preview: 'desktop' | 'tui' | 'cli'
}

export const surfaces: SurfaceData[] = [
  {
    id: 'officedex',
    eyebrow: 'Desktop GUI · Coming soon',
    name: 'officedex',
    tabSub: 'Native workspace for non-terminal users',
    lede: 'A native desktop workspace for designers, PMs, and ops teams—zero terminal, what-you-see-is-what-you-get. Shares the same document generation engine as officecli, shipped as a standalone app.',
    features: [
      'Drag a brief or template into the workspace and preview PPTX, DOCX, XLSX, and Report live',
      'Multi-document project space with version history rollback',
      'One-click switch between External Mode (bring-your-own model) and Hosted Mode (managed credits)',
    ],
    ctas: [
      { label: 'Get notified', href: officedexNotifyPath, variant: 'comingSoon' },
      { label: 'Read about officedex', href: officedexNotifyPath, variant: 'ghost' },
    ],
    preview: 'desktop',
  },
  {
    id: 'officecli-tui',
    eyebrow: 'Interactive TUI',
    name: 'officecli-tui',
    tabSub: 'Run `officecli` to enter',
    lede: 'Run `officecli` with no subcommand to enter the interactive terminal: guided menus, hotkeys, and live progress—no flags to memorize.',
    sameBinaryNote: 'Same binary: officecli = interactive mode',
    features: [
      'Menu-driven: pick format → fill prompt → choose model → preview, all via ←/→/Enter',
      'Live progress, token usage, and credit consumption shown in-flight',
      'Local task queue and history browsing—keeps working when the network does not',
    ],
    ctas: [
      { label: 'Install officecli', href: installOfficeCliPath, variant: 'primary' },
      { label: 'Read TUI docs', href: '/docs', variant: 'ghost' },
    ],
    preview: 'tui',
  },
  {
    id: 'officecli',
    eyebrow: 'Scriptable CLI',
    name: 'officecli',
    tabSub: '`officecli new <format>`',
    lede: 'The scriptable form for CI, cron, and AI agents (Claude Code, Codex, OpenClaw): `officecli new <format> …`. Dependency-free binary, stable exit codes.',
    sameBinaryNote: 'Same binary: officecli <subcmd> = script mode',
    features: [
      'Single binary, zero runtime dependencies—the same command runs on Linux, macOS, and Windows',
      'Stable subcommands and exit codes—unambiguous to wire into CI, cron, and agent runtimes',
      'External and Hosted modes share the same commands; switch local trials to hosted credits without changes',
    ],
    ctas: [
      { label: 'Install officecli', href: installOfficeCliPath, variant: 'primary' },
      { label: 'GitHub releases', href: 'https://github.com/officecli/officecli-dist/releases/latest', variant: 'ghost', external: true },
    ],
    preview: 'cli',
  },
]

export const defaultSurfaceId: SurfaceId = 'officedex'
