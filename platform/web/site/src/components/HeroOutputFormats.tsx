interface FormatCard {
  ext: string
  desc: string
  variant: 'deck' | 'doc' | 'sheet' | 'report' | 'img'
}

const FORMATS: FormatCard[] = [
  { ext: 'PPTX', desc: 'Decks & board reviews', variant: 'deck' },
  { ext: 'DOCX', desc: 'Proposals & long-form', variant: 'doc' },
  { ext: 'XLSX', desc: 'Financial models & sheets', variant: 'sheet' },
  { ext: 'REPORT', desc: 'Self-contained HTML', variant: 'report' },
  { ext: 'IMG', desc: 'Hero & launch visuals', variant: 'img' },
]

export default function HeroOutputFormats() {
  return (
    <section
      aria-label="Supported output formats"
      className="mt-10 rounded-2xl border border-outline-variant/10 bg-white/[0.02] p-6 sm:p-7"
    >
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-3">
        <div>
          <h3 className="text-lg font-bold tracking-tight text-white">Five outputs, supported by every surface</h3>
          <p className="mt-1 text-sm text-outline-variant">
            Same generation engine behind all three surfaces—pick the surface you like, get the same five formats.
          </p>
        </div>
        <span className="inline-flex items-center gap-2 rounded-full border border-tertiary/30 bg-tertiary/10 px-3 py-1 font-headline text-[10px] uppercase tracking-[0.22em] text-tertiary">
          PPTX · DOCX · XLSX · REPORT · IMG
        </span>
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        {FORMATS.map((format) => (
          <article
            key={format.ext}
            className="flex min-h-[8rem] flex-col justify-between rounded-xl border border-outline-variant/10 bg-surface-high p-4"
          >
            <div>
              <div className="font-mono text-sm font-bold tracking-wider text-primary">{format.ext}</div>
              <div className="text-xs text-outline-variant">{format.desc}</div>
            </div>
            <FormatMini variant={format.variant} />
          </article>
        ))}
      </div>
    </section>
  )
}

function FormatMini({ variant }: { variant: FormatCard['variant'] }) {
  if (variant === 'deck') {
    return (
      <div className="mt-3 h-10 rounded-md border border-primary/20 bg-gradient-to-b from-primary/20 to-tertiary/5 p-1.5">
        <div className="h-1/2 w-full rounded-sm bg-white/35" />
      </div>
    )
  }
  if (variant === 'doc') {
    return (
      <div className="mt-3 space-y-1 rounded-md border border-primary/20 bg-gradient-to-b from-primary/20 to-tertiary/5 p-2">
        <div className="h-0.5 w-full rounded bg-white/45" />
        <div className="h-0.5 w-4/5 rounded bg-white/30" />
        <div className="h-0.5 w-full rounded bg-white/45" />
        <div className="h-0.5 w-3/5 rounded bg-white/30" />
      </div>
    )
  }
  if (variant === 'sheet') {
    return (
      <div className="mt-3 grid h-10 grid-cols-5 grid-rows-3 gap-px overflow-hidden rounded-md border border-primary/20 bg-white/10 p-px">
        {Array.from({ length: 15 }).map((_, idx) => (
          <div key={idx} className={idx % 5 === 0 || idx < 5 ? 'bg-primary/30' : 'bg-surface-high'} />
        ))}
      </div>
    )
  }
  if (variant === 'report') {
    return (
      <div className="mt-3 flex h-10 items-end gap-1 rounded-md border border-primary/20 bg-gradient-to-b from-primary/15 to-tertiary/5 p-1.5">
        <div className="h-1/3 flex-1 rounded-sm bg-primary/40" />
        <div className="h-2/3 flex-1 rounded-sm bg-primary/55" />
        <div className="h-full flex-1 rounded-sm bg-tertiary/60" />
        <div className="h-1/2 flex-1 rounded-sm bg-primary/40" />
      </div>
    )
  }
  return (
    <div className="mt-3 h-10 overflow-hidden rounded-md border border-primary/20 bg-gradient-to-br from-[#1d3557] via-[#2a6f97] to-[#013a63]">
      <div className="mx-auto mt-1.5 h-3 w-3 rounded-full bg-[#ffbd2e] shadow-[0_0_10px_#ffbd2e]" />
    </div>
  )
}
