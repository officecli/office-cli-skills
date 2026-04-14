import { motion } from 'motion/react'
import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import heroReportPreview from '../assets/hero-report-preview.svg'
import { platformAppURL } from '../siteData'

export default function Hero() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)

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
            Local-first document operations
          </span>
          <h1 className="font-headline text-6xl md:text-8xl font-bold tracking-tighter leading-[0.9] mb-8 text-white">
            Run <span className="text-primary italic">Document</span> Operations From One Lightweight Binary
          </h1>
          <p className="text-xl text-outline-variant max-w-xl mb-10 leading-relaxed font-light">
            OfficeCLI handles document operations locally: generate files today, review decks, and grow into conversion, modification, summarization, and layout workflows next. For the core path, you only need the binary and an LLM endpoint.
          </p>
          <div className="flex flex-wrap gap-4">
            <motion.a
              whileHover={{ scale: 0.95 }}
              className="bg-gradient-to-br from-primary to-primary-container text-[#002e6b] px-8 py-4 rounded-md font-bold text-lg transition-all"
              href={platformAppHref}
              onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.consoleOpen, { surface: 'site', placement: 'hero', ...extractAttributionParams(location.search) })}
            >
              Get Started
            </motion.a>
            <motion.div whileHover={{ backgroundColor: 'rgba(255,255,255,0.05)' }}>
              <Link className="border border-outline-variant/30 text-white px-8 py-4 rounded-md font-bold text-lg transition-all inline-flex" to="/download">
                Install CLI
              </Link>
            </motion.div>
          </div>
          <div className="mt-10 grid grid-cols-1 md:grid-cols-2 gap-4 max-w-3xl">
            <div className="rounded-2xl border border-outline-variant/10 bg-surface-low px-5 py-4">
              <div className="text-xs font-headline uppercase tracking-widest text-primary mb-3">Homebrew</div>
              <code className="font-mono text-sm text-white break-all">brew install officecli/homebrew-officecli/officecli</code>
            </div>
            <div className="rounded-2xl border border-outline-variant/10 bg-surface-low px-5 py-4">
              <div className="text-xs font-headline uppercase tracking-widest text-primary mb-3">npm</div>
              <code className="font-mono text-sm text-white break-all">npm install -g officecli</code>
            </div>
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
            <div className="p-6 font-mono text-sm leading-relaxed">
              <div className="flex gap-3 mb-4">
                <span className="text-tertiary">$</span>
                <span className="text-white">officecli new pptx "Q3 Board Review" --prompt-file ./brief.md --no-publish</span>
              </div>
              <div className="text-outline-variant mb-2">/ loading prompt and planning document operations... [OK]</div>
              <div className="text-outline-variant mb-2">/ generating slides and embedded visuals... [OK]</div>
              <div className="text-outline-variant mb-6">/ writing local output and preview metadata... [OK]</div>
              <div className="grid grid-cols-2 gap-4">
                <div className="bg-surface-high p-4 rounded-lg border border-primary/20">
                  <div className="h-24 w-full bg-background rounded mb-3 overflow-hidden flex items-center justify-center">
                    <img
                      className="w-full h-full object-cover opacity-50 grayscale hover:grayscale-0 transition-all"
                      src={heroReportPreview}
                      alt="Document preview"
                    />
                  </div>
                  <div className="text-[10px] font-headline text-primary">Q3_BOARD_REVIEW.PPTX</div>
                </div>
                <div className="bg-surface-high p-4 rounded-lg border border-outline-variant/10">
                  <div className="h-24 w-full bg-background rounded mb-3 flex items-center justify-center">
                    <div className="text-outline-variant">
                      <svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" strokeLinecap="round" strokeLinejoin="round"><path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/></svg>
                    </div>
                  </div>
                  <div className="text-[10px] font-headline text-outline-variant">BOARD_SUMMARY.DOCX</div>
                </div>
              </div>
            </div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
