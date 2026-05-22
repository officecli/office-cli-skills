import { Fragment, useCallback, useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { surfaces, defaultSurfaceId, type SurfaceCta, type SurfaceData, type SurfaceId } from '../heroProductsData'

interface HeroProductTabsProps {
  defaultSurface?: SurfaceId
}

export default function HeroProductTabs({ defaultSurface = defaultSurfaceId }: HeroProductTabsProps) {
  const [active, setActive] = useState<SurfaceId>(defaultSurface)
  const tabRefs = useRef<Partial<Record<SurfaceId, HTMLButtonElement | null>>>({})
  const baseId = useId().replace(/:/g, '')

  const focusSurface = useCallback((id: SurfaceId) => {
    setActive(id)
    requestAnimationFrame(() => {
      tabRefs.current[id]?.focus()
    })
  }, [])

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
      if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft' && event.key !== 'Home' && event.key !== 'End') {
        return
      }
      event.preventDefault()
      let nextIndex = index
      if (event.key === 'ArrowRight') nextIndex = (index + 1) % surfaces.length
      else if (event.key === 'ArrowLeft') nextIndex = (index - 1 + surfaces.length) % surfaces.length
      else if (event.key === 'Home') nextIndex = 0
      else if (event.key === 'End') nextIndex = surfaces.length - 1
      focusSurface(surfaces[nextIndex].id)
    },
    [focusSurface],
  )

  return (
    <div className="mt-10 overflow-hidden rounded-2xl border border-outline-variant/15 bg-surface-low/40 shadow-2xl shadow-black/30">
      <div
        role="tablist"
        aria-label="OfficeCLI surfaces"
        className="flex flex-col border-b border-outline-variant/10 bg-black/25 sm:flex-row"
      >
        {surfaces.map((surface, idx) => {
          const selected = active === surface.id
          const tabId = `${baseId}-tab-${surface.id}`
          const panelId = `${baseId}-panel-${surface.id}`
          return (
            <button
              key={surface.id}
              type="button"
              role="tab"
              id={tabId}
              aria-selected={selected}
              aria-controls={panelId}
              tabIndex={selected ? 0 : -1}
              ref={(el) => {
                tabRefs.current[surface.id] = el
              }}
              onClick={() => setActive(surface.id)}
              onKeyDown={(event) => handleKeyDown(event, idx)}
              className={`relative flex-1 px-5 py-4 text-left transition-colors ${
                selected
                  ? 'bg-surface-low/60 text-white'
                  : 'text-outline-variant hover:bg-white/[0.02] hover:text-white'
              } ${idx > 0 ? 'sm:border-l sm:border-outline-variant/10' : ''}`}
            >
              {selected ? (
                <span aria-hidden="true" className="absolute inset-x-0 top-0 h-0.5 bg-tertiary" />
              ) : null}
              <span className="flex items-center gap-2">
                <span
                  className={`text-[10px] font-headline uppercase tracking-[0.22em] ${
                    selected ? 'text-tertiary' : 'text-outline'
                  }`}
                >
                  {surface.eyebrow}
                </span>
                {surface.id === 'officedex' ? (
                  <span className="rounded border border-[#ffbd2e]/50 bg-[#ffbd2e]/15 px-1.5 py-0.5 text-[9px] font-headline uppercase tracking-widest text-[#ffbd2e]">
                    Soon
                  </span>
                ) : null}
              </span>
              <span className="mt-1.5 block font-mono text-base font-bold">{surface.name}</span>
              <span className="mt-1 block text-xs text-outline-variant">{surface.tabSub}</span>
            </button>
          )
        })}
      </div>

      <div className="p-6 sm:p-8">
        {surfaces.map((surface) => {
          const selected = active === surface.id
          const tabId = `${baseId}-tab-${surface.id}`
          const panelId = `${baseId}-panel-${surface.id}`
          return (
            <div
              key={surface.id}
              role="tabpanel"
              id={panelId}
              aria-labelledby={tabId}
              aria-hidden={!selected}
              hidden={!selected}
              className="grid grid-cols-1 items-center gap-8 lg:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]"
            >
              <SurfaceCopy surface={surface} />
              <SurfacePreview kind={surface.preview} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

function SurfaceCopy({ surface }: { surface: SurfaceData }) {
  return (
    <div>
      <span className="text-[11px] font-headline uppercase tracking-[0.22em] text-tertiary">{surface.eyebrow}</span>
      <h2 className="mt-2 font-headline text-3xl font-bold leading-tight tracking-tight text-white sm:text-4xl">
        {surface.name}
      </h2>
      <p className="mt-3 max-w-xl text-base leading-relaxed text-outline-variant">{surface.lede}</p>
      {surface.sameBinaryNote ? (
        <p className="mt-3 inline-flex rounded-lg border border-tertiary/30 bg-tertiary/10 px-3 py-1.5 font-mono text-xs text-outline-variant">
          {surface.sameBinaryNote}
        </p>
      ) : null}
      <ul className="mt-5 space-y-2">
        {surface.features.map((feature) => (
          <li key={feature} className="flex gap-3 text-sm text-white">
            <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-tertiary" />
            <span>{feature}</span>
          </li>
        ))}
      </ul>
      <div className="mt-6 flex flex-wrap items-center gap-3">
        {surface.ctas.map((cta) => (
          <Fragment key={cta.label}>
            <CtaButton cta={cta} />
          </Fragment>
        ))}
      </div>
    </div>
  )
}

function CtaButton({ cta }: { cta: SurfaceCta }) {
  if (cta.variant === 'comingSoon') {
    return (
      <button
        type="button"
        aria-disabled="true"
        className="inline-flex cursor-not-allowed items-center gap-2 rounded-md border border-dashed border-[#ffbd2e]/50 bg-[#ffbd2e]/10 px-5 py-2.5 text-sm font-bold text-[#ffd97a]"
        onClick={(event) => event.preventDefault()}
      >
        {cta.label}
      </button>
    )
  }

  const className =
    cta.variant === 'primary'
      ? 'inline-flex items-center gap-2 rounded-md bg-gradient-to-br from-primary to-primary-container px-5 py-2.5 text-sm font-bold text-[#002e6b]'
      : 'inline-flex items-center gap-2 rounded-md border border-outline-variant/25 px-5 py-2.5 text-sm font-bold text-white hover:bg-white/5'

  if (!cta.href) {
    return (
      <span className={className} aria-disabled="true">
        {cta.label}
      </span>
    )
  }

  if (cta.external || /^https?:/.test(cta.href)) {
    return (
      <a className={className} href={cta.href} target="_blank" rel="noreferrer">
        {cta.label}
      </a>
    )
  }

  return (
    <Link className={className} to={cta.href}>
      {cta.label}
    </Link>
  )
}

function SurfacePreview({ kind }: { kind: SurfaceData['preview'] }) {
  if (kind === 'desktop') return <DesktopPreview />
  if (kind === 'tui') return <TuiPreview />
  return <CliPreview />
}

function PreviewShell({ caption, children }: { caption: string; children: ReactNode }) {
  return (
    <div className="overflow-hidden rounded-2xl border border-outline-variant/15 bg-surface-high shadow-2xl">
      <div className="flex items-center gap-2 border-b border-outline-variant/10 bg-black/30 px-4 py-2.5">
        <span className="h-2.5 w-2.5 rounded-full bg-[#ff5f56]" />
        <span className="h-2.5 w-2.5 rounded-full bg-[#ffbd2e]" />
        <span className="h-2.5 w-2.5 rounded-full bg-[#27c93f]" />
        <span className="ml-3 font-mono text-[10px] uppercase tracking-[0.2em] text-outline">{caption}</span>
      </div>
      {children}
    </div>
  )
}

function DesktopPreview() {
  return (
    <PreviewShell caption="officedex · Workspace · preview">
      <div className="relative grid min-h-[20rem] grid-cols-[10rem_1fr]">
        <div className="absolute right-[-2.5rem] top-4 rotate-[35deg] bg-gradient-to-r from-[#ffbd2e] to-[#ff9d00] px-12 py-1 text-[10px] font-extrabold uppercase tracking-[0.25em] text-[#2a1900]">
          Coming soon
        </div>
        <div className="space-y-1 border-r border-outline-variant/10 bg-black/30 p-3 text-xs">
          {['Projects', 'Templates', 'History', 'Settings'].map((label, idx) => (
            <div
              key={label}
              className={`flex items-center gap-2 rounded-md px-2.5 py-2 ${
                idx === 0 ? 'bg-primary/15 text-white' : 'text-outline-variant'
              }`}
            >
              <span className="h-3.5 w-3.5 rounded bg-gradient-to-br from-primary to-tertiary" />
              {label}
            </div>
          ))}
        </div>
        <div className="space-y-3 p-4">
          <div className="rounded-lg bg-white p-3 text-[#1d3557] shadow-lg">
            <div className="mb-2 h-2 w-1/2 rounded bg-[#1d3557]/85" />
            <div className="mb-1.5 h-1 rounded bg-[#c5d4e0]" />
            <div className="mb-1.5 h-1 rounded bg-[#c5d4e0]" />
            <div className="h-1 w-3/5 rounded bg-[#c5d4e0]" />
          </div>
          <div className="grid grid-cols-3 gap-2">
            {['PPTX', 'DOCX', 'XLSX'].map((tag) => (
              <div
                key={tag}
                className="flex h-20 items-end rounded-lg border border-primary/25 bg-gradient-to-b from-primary/20 to-tertiary/5 p-2"
              >
                <span className="text-[9px] font-headline uppercase tracking-widest text-tertiary">{tag}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </PreviewShell>
  )
}

function TuiPreview() {
  return (
    <PreviewShell caption="officecli · interactive">
      <div className="min-h-[20rem] bg-[#04080d] p-4 font-mono text-xs text-[#cfe7ff]">
        <div className="flex items-center gap-2">
          <span className="text-tertiary">▌</span>
          <span>OfficeCLI · interactive</span>
          <span className="ml-auto text-outline">v0.3.0</span>
        </div>
        <div className="mt-3 grid min-h-[15rem] grid-cols-[10rem_1fr] gap-3">
          <div className="rounded border border-[#1a3146] p-2">
            <div className="mb-2 text-[10px] uppercase tracking-[0.2em] text-tertiary">Format</div>
            {['PPTX', 'DOCX', 'XLSX', 'REPORT', 'IMG'].map((label) => (
              <div
                key={label}
                className={
                  label === 'DOCX'
                    ? 'rounded-sm bg-tertiary px-1.5 py-0.5 text-[#04080d]'
                    : 'py-0.5 text-[#b6cbe0]'
                }
              >
                {label}
              </div>
            ))}
          </div>
          <div className="rounded border border-[#1a3146] p-2">
            <div className="mb-2 text-[10px] uppercase tracking-[0.2em] text-tertiary">New DOCX</div>
            <div className="space-y-1">
              <div>
                <span className="text-primary">prompt</span> &nbsp;Write a 2-page project proposal…
              </div>
              <div>
                <span className="text-primary">model</span> &nbsp;<span className="text-[#ffbd2e]">claude-sonnet-4-6</span>
              </div>
              <div>
                <span className="text-primary">mode</span> &nbsp;External (bring-your-own-endpoint)
              </div>
              <div>
                <span className="text-primary">output</span>&nbsp;./output/PROJECT_PROPOSAL.DOCX
              </div>
            </div>
            <div className="mt-3 text-tertiary">▶ generating… ████████░░ 80%</div>
            <div className="mt-1 text-outline-variant">elapsed 1.6s · 4 credits used</div>
          </div>
        </div>
        <div className="mt-3 bg-primary/20 px-2 py-1 text-[11px] tracking-wide text-white">
          <span className="mr-4"><b className="text-tertiary">↑↓</b> nav</span>
          <span className="mr-4"><b className="text-tertiary">Enter</b> run</span>
          <span className="mr-4"><b className="text-tertiary">Tab</b> switch pane</span>
          <span><b className="text-tertiary">q</b> quit</span>
        </div>
      </div>
    </PreviewShell>
  )
}

function CliPreview() {
  return (
    <PreviewShell caption="officecli · terminal">
      <div className="min-h-[20rem] bg-[#04080d] p-4 font-mono text-[13px] leading-relaxed text-[#d0e5ff]">
        <div>
          <span className="mr-2 text-tertiary">$</span>
          <span className="text-[#27c93f]">officecli</span> <span className="text-[#8af3f7]">new</span>{' '}
          <span className="text-[#ffbd2e]">pptx</span> <span className="text-outline">--prompt-file</span> ./brief.md
        </div>
        <div className="text-white">
          Generation completed. Saved to <span className="text-primary">output/Q3_BOARD_REVIEW.PPTX</span>
        </div>
        <div className="text-outline">Access: External Mode; free unlimited with your model endpoint</div>
        <div className="mt-4 flex items-center gap-3 rounded-lg border border-primary/25 bg-primary/10 p-3">
          <div className="h-11 w-16 shrink-0 rounded-md bg-gradient-to-br from-[#1d3557] to-[#2a6f97]" />
          <div className="text-[11px] text-outline-variant">
            <div>
              <span className="font-bold text-primary">Q3_BOARD_REVIEW.PPTX</span>
              <span className="ml-2 text-[#8af3f7]">Success</span>
            </div>
            <div>Size 2.4 MB · Time 3.1s · ./output/q3_board_review.pptx</div>
          </div>
        </div>
        <div className="mt-4">
          <span className="mr-2 text-tertiary">$</span>
          <span className="text-[#27c93f]">officecli</span> <span className="text-[#8af3f7]">new</span>{' '}
          <span className="text-[#ffbd2e]">img</span> <span className="text-outline">"Launch Visual"</span>{' '}
          <span className="text-outline">--ratio</span> landscape
        </div>
        <div className="text-white">
          Generation completed. Saved to <span className="text-primary">output/LAUNCH_VISUAL.PNG</span>
        </div>
      </div>
    </PreviewShell>
  )
}
