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
          <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[#94003d]/30 text-[#ff9cb0] text-xs font-headline uppercase tracking-widest mb-6 border border-[#94003d]/50">
            <span className="w-2 h-2 rounded-full bg-tertiary terminal-pulse shadow-[0_0_8px_#00dce5]"></span>
            Live Infrastructure v4.2
          </span>
          <h1 className="font-headline text-6xl md:text-8xl font-bold tracking-tighter leading-[0.9] mb-8 text-white">
            Plug <span className="text-primary italic">Document</span> Production Into Your Workflows
          </h1>
          <p className="text-xl text-outline-variant max-w-xl mb-10 leading-relaxed font-light">
            Automate the creation of production-grade PPTX, DOCX, and XLSX assets with AI-orchestrated CLI tools. Built for high-frequency infrastructure and headless pipelines.
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
              <Link className="border border-outline-variant/30 text-white px-8 py-4 rounded-md font-bold text-lg transition-all inline-flex" to="/docs">
                View Docs
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
              <div className="ml-4 text-[10px] font-headline text-outline uppercase tracking-[0.2em]">production_terminal_v1</div>
            </div>
            <div className="p-6 font-mono text-sm leading-relaxed">
              <div className="flex gap-3 mb-4">
                <span className="text-tertiary">$</span>
                <span className="text-white">officecli generate --prompt "Quarterly report for Q3 with revenue charts" --format pptx</span>
              </div>
              <div className="text-outline-variant mb-2">/ Analyzing context... [OK]</div>
              <div className="text-outline-variant mb-2">/ Generating 12 slides... [OK]</div>
              <div className="text-outline-variant mb-6">/ Exporting to production-ready PPTX... [OK]</div>
              <div className="grid grid-cols-2 gap-4">
                <div className="bg-surface-high p-4 rounded-lg border border-primary/20">
                  <div className="h-24 w-full bg-background rounded mb-3 overflow-hidden flex items-center justify-center">
                    <img
                      className="w-full h-full object-cover opacity-50 grayscale hover:grayscale-0 transition-all"
                      src={heroReportPreview}
                      alt="Revenue Chart"
                    />
                  </div>
                  <div className="text-[10px] font-headline text-primary">REPORT_Q3.PPTX</div>
                </div>
                <div className="bg-surface-high p-4 rounded-lg border border-outline-variant/10">
                  <div className="h-24 w-full bg-background rounded mb-3 flex items-center justify-center">
                    <div className="text-outline-variant">
                      <svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" strokeLinecap="round" strokeLinejoin="round"><path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/></svg>
                    </div>
                  </div>
                  <div className="text-[10px] font-headline text-outline-variant">SUMMARY.DOCX</div>
                </div>
              </div>
            </div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
