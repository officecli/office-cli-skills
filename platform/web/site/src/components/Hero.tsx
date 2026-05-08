import { useState, type ReactNode } from 'react'
import { motion } from 'motion/react'
import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import heroReportPreview from '../assets/hero-report-preview.svg'
import { platformAppURL } from '../siteData'

type HeroFormatTab = 'pptx' | 'docx' | 'xlsx' | 'report' | 'img'

const HERO_TERMINAL_TABS: Array<{ id: HeroFormatTab; label: string }> = [
  { id: 'pptx', label: 'PPTX' },
  { id: 'docx', label: 'DOCX' },
  { id: 'xlsx', label: 'XLSX' },
  { id: 'report', label: 'REPORT' },
  { id: 'img', label: 'IMG' },
]

function DocPageMock() {
  return (
    <div className="w-full h-full bg-white rounded-sm shadow-sm flex flex-col p-2 gap-1.5 opacity-50 grayscale hover:grayscale-0 transition-all">
      <div className="w-1/3 h-1.5 bg-outline-variant/30 rounded-full mb-1"></div>
      <div className="w-full h-1 bg-outline-variant/20 rounded-full"></div>
      <div className="w-5/6 h-1 bg-outline-variant/20 rounded-full"></div>
      <div className="w-full h-1 bg-outline-variant/20 rounded-full"></div>
      <div className="w-4/5 h-1 bg-outline-variant/20 rounded-full"></div>
      <div className="w-full h-1 bg-outline-variant/20 rounded-full"></div>
      <div className="w-2/3 h-1 bg-outline-variant/20 rounded-full"></div>
    </div>
  )
}

function SpreadsheetMock() {
  const rows = 7
  const cols = 8
  const rowPx = 6
  return (
    <div className="w-full flex items-center justify-center py-0.5 opacity-50 grayscale hover:grayscale-0 transition-all">
      <div
        className="w-full max-w-[7.5rem] grid gap-px bg-outline-variant/35 rounded-sm overflow-hidden border border-outline-variant/25"
        style={{
          gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${rows}, ${rowPx}px)`,
        }}
      >
        {Array.from({ length: rows * cols }, (_, i) => {
          const r = Math.floor(i / cols)
          const c = i % cols
          const isHeaderRow = r === 0
          const isHeaderCol = c === 0
          return (
            <div
              key={i}
              className={`min-h-0 min-w-0 ${
                isHeaderRow || isHeaderCol ? 'bg-primary/18' : 'bg-surface-high'
              }`}
            />
          )
        })}
      </div>
    </div>
  )
}

function ImageMock() {
  return (
    <div className="w-full h-full rounded-sm shadow-sm relative overflow-hidden opacity-80 group-hover:opacity-100 transition-all bg-gradient-to-br from-[#1d3557] via-[#2a6f97] to-[#013a63]">
      <div className="absolute inset-0">
        <div className="absolute top-1.5 left-1.5 w-5 h-5 rounded-full bg-tertiary/40 blur-[2px]" />
        <div className="absolute bottom-2 right-2 w-9 h-9 rounded-full bg-primary/30 blur-md" />
        <div className="absolute top-1/2 left-1/3 w-10 h-0.5 bg-white/40 rounded-full rotate-12" />
      </div>
      <div className="relative z-10 h-full w-full flex flex-col items-center justify-center gap-1">
        <div className="w-8 h-1 bg-white/70 rounded-full" />
        <div className="w-12 h-0.5 bg-white/40 rounded-full" />
      </div>
    </div>
  )
}

function ReportPageMock() {
  return (
    <div className="w-full h-full bg-white rounded-sm shadow-sm flex flex-col p-2 gap-1.5 opacity-50 grayscale hover:grayscale-0 transition-all overflow-hidden">
      {/* Title */}
      <div className="w-1/2 h-1.5 bg-primary/40 rounded-sm mb-0.5"></div>
      
      {/* Summary text */}
      <div className="space-y-0.5 mb-1">
        <div className="w-full h-0.5 bg-outline-variant/20 rounded-full"></div>
        <div className="w-5/6 h-0.5 bg-outline-variant/20 rounded-full"></div>
        <div className="w-4/5 h-0.5 bg-outline-variant/20 rounded-full"></div>
      </div>

      {/* Grid for charts */}
      <div className="flex gap-1.5 flex-1 min-h-0">
        {/* Left: Bar chart */}
        <div className="flex-1 bg-surface-low/30 rounded border border-outline-variant/10 p-1 flex flex-col justify-end gap-0.5">
          <div className="flex items-end justify-between h-full gap-0.5 pt-1">
            <div className="w-full bg-primary/30 h-[40%] rounded-t-[1px]"></div>
            <div className="w-full bg-primary/50 h-[70%] rounded-t-[1px]"></div>
            <div className="w-full bg-tertiary/60 h-[90%] rounded-t-[1px]"></div>
            <div className="w-full bg-primary/40 h-[60%] rounded-t-[1px]"></div>
          </div>
        </div>
        
        {/* Right: Donut chart & legend */}
        <div className="flex-1 bg-surface-low/30 rounded border border-outline-variant/10 p-1 flex items-center justify-center gap-1">
          <div className="w-4 h-4 rounded-full border-[2px] border-primary/20 border-r-tertiary/60 border-t-tertiary/60 shrink-0"></div>
          <div className="flex flex-col gap-0.5 flex-1">
            <div className="w-full h-0.5 bg-outline-variant/30 rounded-full"></div>
            <div className="w-2/3 h-0.5 bg-outline-variant/20 rounded-full"></div>
          </div>
        </div>
      </div>
    </div>
  )
}

function HeroOutputFileCard({
  preview,
  filename,
  size,
  elapsed,
}: {
  preview: ReactNode
  filename: string
  size: string
  elapsed: string
}) {
  const path = `./output/${filename.toLowerCase()}`
  return (
    <div className="bg-surface-high rounded-lg border border-primary/20 p-3 sm:p-4 flex flex-col sm:flex-row gap-3 sm:gap-4 items-stretch w-full">
      <div className="shrink-0 w-full sm:w-32 h-24 rounded-md overflow-hidden bg-background border border-outline-variant/10 flex items-center justify-center">
        {preview}
      </div>
      <div className="min-w-0 flex-1 flex flex-col justify-center gap-2 py-0.5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[10px] font-headline text-primary break-all">{filename}</span>
          <span className="text-[9px] font-headline uppercase tracking-wider text-tertiary bg-tertiary/10 border border-tertiary/25 px-2 py-0.5 rounded">
            Success
          </span>
        </div>
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-[10px] text-outline-variant">
          <div>
            <dt className="sr-only">Size</dt>
            <dd>Size {size}</dd>
          </div>
          <div>
            <dt className="sr-only">Elapsed</dt>
            <dd>Time {elapsed}</dd>
          </div>
        </dl>
        <div className="text-[9px] text-outline-variant/80 font-mono truncate" title={path}>
          {path}
        </div>
      </div>
    </div>
  )
}

function HighlightedCommand({ command }: { command: string }) {
  // Simple syntax highlighting for officecli commands
  const parts = command.split(' ')
  return (
    <span className="break-all">
      {parts.map((part, i) => {
        if (part === 'officecli') return <span key={i} className="text-[#27c93f] mr-1.5">{part}</span>
        if (part === 'new' || part === 'config' || part === 'set-generation') return <span key={i} className="text-[#8af3f7] mr-1.5">{part}</span>
        if (part === 'pptx' || part === 'docx' || part === 'xlsx' || part === 'report' || part === 'img') return <span key={i} className="text-[#ffbd2e] mr-1.5">{part}</span>
        if (part.startsWith('"') || part.startsWith("'")) return <span key={i} className="text-[#ff5f56] mr-1.5">{part}</span>
        if (part.startsWith('--')) return <span key={i} className="text-outline-variant mr-1.5">{part}</span>
        return <span key={i} className="text-white mr-1.5">{part}</span>
      })}
    </span>
  )
}

export default function Hero() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)
  const [activeTab, setActiveTab] = useState<HeroFormatTab>('pptx')

  const terminalByTab: Record<
    HeroFormatTab,
    { command: string; logLines: string[]; output: ReactNode }
  > = {
    pptx: {
      command: 'officecli new pptx --prompt-file ./brief.md',
      logLines: [
        'Generation completed. Saved to output/Q3_BOARD_REVIEW.PPTX',
      ],
      output: (
        <div className="space-y-4">
          <div className="space-y-1.5">
            <div className="text-white">
              Preview URL: <a href="https://officecli.io/p/xyz123" target="_blank" rel="noreferrer" className="text-primary hover:underline">https://officecli.io/p/xyz123</a>; password: abcdef
            </div>
            <div className="text-outline-variant">
              Access: paid mode; 109 generations remaining; trial 10
            </div>
          </div>
          <HeroOutputFileCard
            filename="Q3_BOARD_REVIEW.PPTX"
            size="2.4 MB"
            elapsed="3.1s"
            preview={
              <img
                className="h-full w-full object-cover opacity-50 grayscale hover:grayscale-0 transition-all"
                src={heroReportPreview}
                alt="Presentation preview"
              />
            }
          />
        </div>
      ),
    },
    docx: {
      command: 'officecli new docx --prompt "Write a 2-page project proposal for a new mobile app launch..."',
      logLines: [
        'Generation completed. Saved to output/PROJECT_PROPOSAL.DOCX',
      ],
      output: (
        <div className="space-y-4">
          <div className="space-y-1.5">
            <div className="text-white">
              Preview URL: <a href="https://officecli.io/p/xyz123" target="_blank" rel="noreferrer" className="text-primary hover:underline">https://officecli.io/p/xyz123</a>; password: abcdef
            </div>
            <div className="text-outline-variant">
              Access: paid mode; 109 generations remaining; trial 10
            </div>
          </div>
          <HeroOutputFileCard
            filename="PROJECT_PROPOSAL.DOCX"
            size="412 KB"
            elapsed="1.8s"
            preview={
              <div className="h-full w-full p-2 flex items-center justify-center">
                <DocPageMock />
              </div>
            }
          />
        </div>
      ),
    },
    xlsx: {
      command: 'officecli new xlsx --prompt-file ./metrics.md',
      logLines: [
        'Generation completed. Saved to output/Q3_FINANCIAL_MODEL.XLSX',
      ],
      output: (
        <div className="space-y-4">
          <div className="space-y-1.5">
            <div className="text-white">
              Preview URL: <a href="https://officecli.io/p/xyz123" target="_blank" rel="noreferrer" className="text-primary hover:underline">https://officecli.io/p/xyz123</a>; password: abcdef
            </div>
            <div className="text-outline-variant">
              Access: paid mode; 109 generations remaining; trial 10
            </div>
          </div>
          <HeroOutputFileCard
            filename="Q3_FINANCIAL_MODEL.XLSX"
            size="89 KB"
            elapsed="2.0s"
            preview={
              <div className="h-full w-full p-2 flex items-center justify-center">
                <SpreadsheetMock />
              </div>
            }
          />
        </div>
      ),
    },
    report: {
      command: 'officecli new report "Q2 Business Review" --file ./data/q2-metrics.xlsx --prompt "Summarize revenue shifts and decision points for the board"',
      logLines: [
        'Generation completed. Saved to output/Q2_BUSINESS_REVIEW.HTML',
      ],
      output: (
        <div className="space-y-4">
          <div className="space-y-1.5">
            <div className="text-white">
              Preview URL: <a href="https://officecli.io/p/xyz123" target="_blank" rel="noreferrer" className="text-primary hover:underline">https://officecli.io/p/xyz123</a>; password: abcdef
            </div>
            <div className="text-outline-variant">
              Access: paid mode; 109 generations remaining; trial 10
            </div>
          </div>
          <div className="text-[10px] font-headline uppercase tracking-[0.25em] text-tertiary">
            REPORT output
          </div>
          <HeroOutputFileCard
            filename="Q2_BUSINESS_REVIEW.HTML"
            size="24 KB"
            elapsed="0.9s"
            preview={
              <div className="h-full w-full flex flex-col min-h-0 p-1 gap-1">
                <div className="shrink-0 h-4 rounded bg-background/90 border border-outline-variant/10 flex items-center gap-1 px-1.5">
                  <div className="w-1 h-1 rounded-full bg-outline-variant/40"></div>
                  <span className="text-[7px] font-headline text-outline-variant truncate">q2-business-review.html</span>
                </div>
                <div className="min-h-0 flex-1 overflow-hidden flex items-center justify-center px-0.5 pb-0.5">
                  <ReportPageMock />
                </div>
              </div>
            }
          />
        </div>
      ),
    },
    img: {
      command: 'officecli new img "Launch Visual" --prompt "Polished product launch hero image" --ratio landscape --reference-image ./brand.png',
      logLines: [
        'Generation completed. Saved to output/LAUNCH_VISUAL.PNG',
      ],
      output: (
        <div className="space-y-4">
          <div className="space-y-1.5">
            <div className="text-white">
              Preview URL: <a href="https://officecli.io/p/img-xyz" target="_blank" rel="noreferrer" className="text-primary hover:underline">https://officecli.io/p/img-xyz</a>; password: abcdef
            </div>
            <div className="text-outline-variant">
              Access: paid mode; 24 image generations remaining; free bucket 3/day
            </div>
          </div>
          <div className="text-[10px] font-headline uppercase tracking-[0.25em] text-tertiary">
            IMG output (landscape, 1 reference)
          </div>
          <HeroOutputFileCard
            filename="LAUNCH_VISUAL.PNG"
            size="612 KB"
            elapsed="4.7s"
            preview={
              <div className="h-full w-full p-1.5 flex items-center justify-center group">
                <ImageMock />
              </div>
            }
          />
        </div>
      ),
    },
  }

  const current = terminalByTab[activeTab]

  return (
    <section className="relative min-h-[90vh] flex items-center px-8 md:px-16 max-w-[1440px] mx-auto pt-20">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-16 items-center w-full">
        <motion.div
          initial={{ opacity: 0, x: -20 }}
          animate={{ opacity: 1, x: 0 }}
          transition={{ duration: 0.6 }}
          className="z-10"
        >
          <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[#0d4c54]/40 text-[#8af3f7] text-xs font-headline uppercase tracking-widest mb-6 border border-[#0d4c54]/60">
            <span className="w-2 h-2 rounded-full bg-tertiary terminal-pulse shadow-[0_0_8px_#00dce5]"></span>
            Local-first AI document generation CLI
          </span>
          <h1 className="font-headline text-6xl md:text-8xl font-bold tracking-tighter leading-[0.9] mb-8 text-white">
            Generate <span className="text-primary italic">PPTX</span>, DOCX, XLSX, REPORT, and IMG Outputs From One AI CLI
          </h1>
          <p className="text-xl text-outline-variant max-w-xl mb-10 leading-relaxed font-light">
            OfficeCLI is a local-first AI document generation CLI for terminal workflows. Generate PPTX, DOCX, XLSX, workbook-backed REPORT outputs, and standalone IMG visuals with your own LLM endpoint or the OfficeCLI image service, without a backend stack or cluster, then review decks and expand into broader document automation.
          </p>
          <div className="flex flex-wrap gap-4">
            <motion.a
              whileHover={{ scale: 0.95 }}
              className="bg-gradient-to-br from-primary to-primary-container text-[#002e6b] px-8 py-4 rounded-md font-bold text-lg transition-all"
              href={platformAppHref}
              onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.consoleOpen, { surface: 'site', placement: 'hero', ...extractAttributionParams(location.search) })}
            >
              Start Building for Free
            </motion.a>
            <motion.div whileHover={{ backgroundColor: 'rgba(255,255,255,0.05)' }}>
              <Link className="border border-outline-variant/30 text-white px-8 py-4 rounded-md font-bold text-lg transition-all inline-flex" to="/download">
                Install the AI CLI
              </Link>
            </motion.div>
          </div>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, scale: 0.95 }}
          animate={{ opacity: 1, scale: 1 }}
          transition={{ duration: 0.8, delay: 0.2 }}
          className="relative group"
        >
          <div className="absolute -inset-4 bg-primary/10 blur-3xl rounded-full opacity-30 group-hover:opacity-50 transition-opacity"></div>
          <div className="relative bg-surface-low rounded-xl border border-outline-variant/20 overflow-hidden shadow-2xl">
            <div className="flex items-center gap-2 px-4 py-3 bg-surface-high border-b border-outline-variant/10">
              <div className="flex gap-1.5">
                <div className="w-3 h-3 rounded-full bg-[#ff5f56]"></div>
                <div className="w-3 h-3 rounded-full bg-[#ffbd2e]"></div>
                <div className="w-3 h-3 rounded-full bg-[#27c93f]"></div>
              </div>
              <div className="ml-4 text-[10px] font-headline text-outline uppercase tracking-[0.2em]">document_ops_terminal</div>
            </div>
            <div className="flex flex-wrap gap-1 px-4 pt-3 pb-2 bg-surface-high/80 border-b border-outline-variant/10" role="tablist" aria-label="Example output format">
              {HERO_TERMINAL_TABS.map(({ id, label }) => {
                const selected = activeTab === id
                return (
                  <button
                    key={id}
                    type="button"
                    role="tab"
                    aria-selected={selected}
                    className={`rounded-md px-3 py-1.5 text-[10px] font-headline uppercase tracking-widest transition-colors ${
                      selected
                        ? 'bg-primary/20 text-primary border border-primary/30'
                        : 'text-outline-variant hover:text-white border border-transparent'
                    }`}
                    onClick={() => setActiveTab(id)}
                  >
                    {label}
                  </button>
                )
              })}
            </div>
            <motion.div
              key={activeTab}
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.25 }}
              className="p-6 font-mono text-sm leading-relaxed"
            >
              <div className="flex gap-3 mb-4">
                <span className="text-tertiary shrink-0">$</span>
                <HighlightedCommand command={current.command} />
              </div>
              {current.logLines.map((line, i) => (
                <div
                  key={line}
                  className={`${i === 0 ? 'text-white' : 'text-outline-variant'} ${i === current.logLines.length - 1 ? 'mb-6' : 'mb-2'}`}
                >
                  {line}
                </div>
              ))}
              {current.output}
            </motion.div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
