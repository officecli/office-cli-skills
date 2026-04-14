const availableNow = [
  'Generate PPTX, DOCX, XLSX, and HTML files from natural-language prompts.',
  'Review and score local PPTX files, with optional visual review when soffice is available.',
  'Publish optional online previews when preview publishing is configured.',
]

const plannedDocumentFamilies = [
  'PDF, Markdown, and CSV',
  'Word family: doc, dot, dotx, docm, dotm, wps, wpt',
  'Spreadsheet family: slx, xlt, xltx, xlsm, et, ett',
  'Presentation family: ppt, pot, potx, pptm, potm, dps, dpt',
]

const plannedOperations = [
  'Content modification and template-aware editing',
  'Summarization, extraction, and richer document processing',
  'Format conversion across document families',
  'Layout cleanup and more advanced formatting operations',
]

export default function Roadmap() {
  return (
    <section className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="max-w-6xl">
        <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Roadmap</span>
        <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-6 mb-12">
          <div className="max-w-3xl">
            <h2 className="font-headline text-4xl md:text-5xl font-bold text-white tracking-tight mb-5">OfficeCLI is growing from document generation into document operations.</h2>
            <p className="text-outline-variant text-lg leading-relaxed">
              Today the public release is strongest at generation, PPT review, and preview workflows. The next phase expands into conversion, modification, summarization, and broader document-family support.
            </p>
          </div>
          <div className="rounded-2xl border border-outline-variant/10 bg-surface-low px-5 py-4 text-sm text-outline-variant max-w-md">
            Roadmap items below are planned directions, not capabilities in the current release.
          </div>
        </div>

        <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
          <article className="bg-surface-low border border-outline-variant/10 rounded-3xl p-8">
            <div className="text-xs font-headline uppercase tracking-widest text-tertiary mb-4">Available now</div>
            <h3 className="font-headline text-3xl font-bold text-white mb-6">Current release focus</h3>
            <ul className="space-y-4 text-outline-variant leading-relaxed">
              {availableNow.map((item) => (
                <li key={item} className="border border-outline-variant/10 rounded-2xl bg-background/60 px-5 py-4">{item}</li>
              ))}
            </ul>
          </article>

          <article className="bg-surface-high border border-primary/15 rounded-3xl p-8">
            <div className="text-xs font-headline uppercase tracking-widest text-primary mb-4">Planned next</div>
            <h3 className="font-headline text-3xl font-bold text-white mb-6">Document families and operations on the roadmap</h3>
            <div className="space-y-6">
              <div>
                <div className="text-sm font-semibold text-white mb-3">Planned document families</div>
                <ul className="space-y-3 text-outline-variant leading-relaxed">
                  {plannedDocumentFamilies.map((item) => (
                    <li key={item} className="border border-outline-variant/10 rounded-2xl bg-background/60 px-5 py-4">{item}</li>
                  ))}
                </ul>
              </div>
              <div>
                <div className="text-sm font-semibold text-white mb-3">Planned document operations</div>
                <ul className="space-y-3 text-outline-variant leading-relaxed">
                  {plannedOperations.map((item) => (
                    <li key={item} className="border border-outline-variant/10 rounded-2xl bg-background/60 px-5 py-4">{item}</li>
                  ))}
                </ul>
              </div>
            </div>
          </article>
        </div>
      </div>
    </section>
  )
}
