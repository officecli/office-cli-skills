const roadmapPhases = [
  {
    phase: 'Now',
    title: 'Current public release',
    accent: 'text-tertiary',
    border: 'border-outline-variant/10',
    background: 'bg-surface-low',
    items: [
      'Publish a password-protected online preview link with one command — a differentiator other AI document CLIs do not ship out of the box. Toggle per command with `--no-publish` or globally via `officecli config set-publish`.',
      'Generate PPTX, DOCX, XLSX, and workbook-backed REPORT files from natural-language prompts.',
      'Generate standalone IMG visuals with `new img`, including ratio, explicit size, and one or more reference images. Successful images publish online previews by default once publishing is configured.',
      'Review and score local PPTX files, with optional visual review when `soffice` is available.',
    ],
  },
  {
    phase: 'Next',
    title: 'Broader document operations',
    accent: 'text-primary',
    border: 'border-primary/20',
    background: 'bg-surface-high',
    items: [
      'Add content modification and template-aware editing flows.',
      'Expand summarization, extraction, and richer processing workflows.',
      'Support format conversion across more document families.',
    ],
  },
  {
    phase: 'Later',
    title: 'Wider format coverage',
    accent: 'text-[#8af3f7]',
    border: 'border-outline-variant/10',
    background: 'bg-surface-low',
    items: [
      'Add PDF, Markdown, and CSV to the supported formats.',
      'Cover more document, spreadsheet, and presentation formats.',
      'Improve layout cleanup and advanced formatting operations.',
    ],
  },
]

export default function Roadmap() {
  return (
    <section className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="max-w-6xl">
        <div className="max-w-3xl mb-14">
          <h2 className="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-5">ROADMAP</h2>
          <p className="text-outline-variant text-lg leading-relaxed">
            OfficeCLI is moving from document generation into a broader document operations workflow. The timeline below separates current core capabilities from the next areas of expansion.
          </p>
        </div>

        <div className="relative">
          <div className="absolute left-4 top-0 bottom-0 w-px bg-outline-variant/15 hidden md:block" />
          <div className="space-y-8">
            {roadmapPhases.map((phase) => (
              <article
                key={phase.phase}
                className={`relative md:pl-16 rounded-3xl border ${phase.border} ${phase.background} p-8`}
              >
                <div className="hidden md:flex absolute left-0 top-10 h-8 w-8 items-center justify-center rounded-full border border-outline-variant/20 bg-background text-xs font-headline uppercase tracking-widest text-white">
                  {phase.phase[0]}
                </div>
                <div className={`text-xs font-headline uppercase tracking-[0.25em] ${phase.accent} mb-3`}>
                  {phase.phase}
                </div>
                <h3 className="font-headline text-3xl font-bold text-white mb-5">{phase.title}</h3>
                <ul className="space-y-3 text-outline-variant leading-relaxed">
                  {phase.items.map((item) => (
                    <li key={item} className="rounded-2xl border border-outline-variant/10 bg-background/60 px-5 py-4">
                      {item}
                    </li>
                  ))}
                </ul>
              </article>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
