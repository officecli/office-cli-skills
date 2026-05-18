import { motion } from 'motion/react'
import { Link } from 'react-router-dom'
import { Bolt, LaptopMinimal, Share2, ScanSearch, Terminal, Bot, Plug, ArrowRight, Link2, Lock } from 'lucide-react'

export default function Features() {
  return (
    <section id="what-is-officecli" className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="mb-16">
        <h2 className="font-headline text-4xl md:text-5xl font-bold text-white tracking-tight mb-4">
          What Is <span className="text-primary italic">OfficeCLI</span>
        </h2>
        <p className="text-outline-variant text-lg max-w-2xl">
          OfficeCLI, sometimes searched as office cli, is a local-first AI document generation CLI for developers and automation teams. Choose free unlimited External Mode with your own model endpoint, or Hosted Mode with OfficeCLI-managed runtime and hosted credits.
        </p>
        <div className="mt-6 flex flex-wrap gap-3 text-sm font-semibold">
          <Link className="rounded-full border border-outline-variant/20 px-4 py-2 text-primary hover:border-primary/30 hover:text-tertiary" to="/ai-pptx-generator">AI PPTX generator</Link>
          <Link className="rounded-full border border-outline-variant/20 px-4 py-2 text-primary hover:border-primary/30 hover:text-tertiary" to="/docx-generator-cli">DOCX generator CLI</Link>
          <Link className="rounded-full border border-outline-variant/20 px-4 py-2 text-primary hover:border-primary/30 hover:text-tertiary" to="/xlsx-report-generation">XLSX reports</Link>
          <Link className="rounded-full border border-outline-variant/20 px-4 py-2 text-primary hover:border-primary/30 hover:text-tertiary" to="/office-automation-ai-agents">Agent automation</Link>
        </div>
      </div>

      <div className="mb-8 grid gap-6 md:grid-cols-2">
        <div className="bg-surface-low p-8 rounded-2xl border border-tertiary/20">
          <h3 className="font-headline text-2xl font-bold text-white mb-3">External Mode</h3>
          <p className="text-outline-variant leading-relaxed">Free unlimited PPTX, DOCX, XLSX, REPORT, and IMG generation with your own LLM endpoint. Standalone IMG uses the image provider configured by <code>config set-generation</code>.</p>
        </div>
        <div className="bg-surface-low p-8 rounded-2xl border border-primary/20">
          <h3 className="font-headline text-2xl font-bold text-white mb-3">Hosted Mode</h3>
          <p className="text-outline-variant leading-relaxed">Use OfficeCLI-managed model runtime and hosted credits when you want fast setup without maintaining model service configuration.</p>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6 auto-rows-[minmax(200px,auto)]">
        {/* Bento Box 1: Large Feature */}
        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ delay: 0.1 }}
          className="md:col-span-2 md:row-span-2 bg-surface-low p-10 rounded-3xl flex flex-col justify-between group hover:bg-surface-high transition-all border border-white/5 relative overflow-hidden"
        >
          <div className="absolute top-0 right-0 w-64 h-64 bg-tertiary/5 rounded-full blur-3xl -mr-20 -mt-20 transition-opacity group-hover:opacity-100 opacity-50"></div>
          
          <div className="flex flex-col lg:flex-row gap-12 h-full relative z-10">
            <div className="flex-1 flex flex-col justify-between">
              <div>
                <Bolt className="text-tertiary w-12 h-12 mb-8" />
                <h3 className="font-headline text-4xl font-bold text-white mb-4">Single-Binary Document Automation</h3>
                <p className="text-outline-variant text-lg leading-relaxed max-w-md">
                  OfficeCLI ships as one lightweight binary for AI document generation. It does not require Python, LibreOffice, Microsoft Office, Docker, Kubernetes, a backend stack, queueing layer, or cluster.
                </p>
              </div>
              <div className="mt-12 border-t border-outline-variant/10 pt-6">
                <span className="text-xs font-headline uppercase text-tertiary tracking-widest">Local-first by default</span>
              </div>
            </div>

            <div className="hidden lg:flex flex-col justify-center items-center w-[28rem] shrink-0 opacity-80 group-hover:opacity-100 transition-opacity">
              {/* Architecture Diagram */}
              <div className="w-full flex gap-6 items-stretch">
                {/* Traditional Stack (Faded) */}
                <div className="flex-1 border border-outline-variant/20 rounded-xl p-5 bg-background/50 relative opacity-40">
                  <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-10">
                    <div className="w-full h-px bg-red-500/40 rotate-12"></div>
                  </div>
                  <div className="text-[10px] font-headline uppercase text-outline-variant mb-4 text-center tracking-widest">Traditional Stack</div>
                  <div className="flex flex-col gap-3 items-center">
                    <div className="px-4 py-2 bg-surface-high rounded border border-outline-variant/10 text-xs w-full text-center">API Gateway</div>
                    <ArrowRight className="w-4 h-4 text-outline-variant rotate-90" />
                    <div className="px-4 py-2 bg-surface-high rounded border border-outline-variant/10 text-xs w-full text-center">Queue Layer</div>
                    <ArrowRight className="w-4 h-4 text-outline-variant rotate-90" />
                    <div className="px-4 py-2 bg-surface-high rounded border border-outline-variant/10 text-xs w-full text-center">Worker Cluster</div>
                  </div>
                </div>

                {/* OfficeCLI Stack (Highlighted) */}
                <div className="flex-1 border border-primary/30 rounded-xl p-5 bg-primary/5 relative shadow-[0_0_30px_rgba(174,198,255,0.05)] flex flex-col">
                  <div className="text-[10px] font-headline uppercase text-primary mb-4 text-center tracking-widest">OfficeCLI</div>
                  <div className="flex flex-col gap-3 items-center flex-1 justify-center">
                    <div className="px-4 py-2 bg-surface-high rounded border border-outline-variant/10 text-xs w-full text-center">Your App / Terminal</div>
                    <ArrowRight className="w-4 h-4 text-primary rotate-90" />
                    <div className="px-4 py-3 bg-primary/20 rounded border border-primary/40 text-xs font-bold text-white w-full text-center shadow-[0_0_15px_rgba(174,198,255,0.2)]">OfficeCLI Binary</div>
                    <ArrowRight className="w-4 h-4 text-primary rotate-90" />
                    <div className="px-4 py-2 bg-surface-high rounded border border-outline-variant/10 text-xs w-full text-center">LLM Endpoint</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </motion.div>

        {/* Bento Box 2: Medium Feature */}
        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ delay: 0.2 }}
          className="md:col-span-1 md:row-span-1 bg-surface-low p-8 rounded-3xl flex flex-col group hover:bg-surface-high transition-all border border-white/5 relative overflow-hidden"
        >
          <div className="absolute top-0 right-0 w-32 h-32 bg-primary/5 rounded-full blur-2xl -mr-10 -mt-10 transition-opacity group-hover:opacity-100 opacity-50"></div>
          <LaptopMinimal className="text-primary w-10 h-10 mb-6 relative z-10" />
          <h3 className="font-headline text-2xl font-bold text-white mb-5 relative z-10">Fits Automated Workflows</h3>
          <ul className="space-y-4 relative z-10 text-sm">
            <li className="flex items-start gap-3">
              <Terminal className="text-primary w-5 h-5 shrink-0 mt-0.5" />
              <div>
                <span className="font-bold text-white block mb-0.5">Native CLI</span>
                <span className="text-outline-variant">Direct terminal & CI/CD execution</span>
              </div>
            </li>
            <li className="flex items-start gap-3">
              <Bot className="text-primary w-5 h-5 shrink-0 mt-0.5" />
              <div>
                <span className="font-bold text-white block mb-0.5">Optional AI Agents</span>
                <span className="text-outline-variant">Use fully without an agent, or integrate Codex, Claude, and OpenClaw</span>
              </div>
            </li>
            <li className="flex items-start gap-3">
              <Plug className="text-primary w-5 h-5 shrink-0 mt-0.5" />
              <div>
                <span className="font-bold text-white block mb-0.5">OpenClaw</span>
                <span className="text-outline-variant">Native protocol support for AI platforms</span>
              </div>
            </li>
          </ul>
        </motion.div>

        {/* Bento Box 3: Medium Feature */}
        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ delay: 0.3 }}
          className="md:col-span-1 md:row-span-1 bg-surface-low p-8 rounded-3xl flex flex-col group hover:bg-surface-high transition-all border border-white/5 relative overflow-hidden"
        >
          <div className="absolute top-0 right-0 w-32 h-32 bg-secondary/5 rounded-full blur-2xl -mr-10 -mt-10 transition-opacity group-hover:opacity-100 opacity-50"></div>
          <ScanSearch className="text-secondary w-10 h-10 mb-6 relative z-10" />
          <h3 className="font-headline text-2xl font-bold text-white mb-3 relative z-10">Create, Review, Export</h3>
          <p className="text-outline-variant leading-relaxed relative z-10">
            The current release creates PPTX, DOCX, XLSX, workbook-backed REPORT outputs, and standalone IMG visuals, and can review or score local PPTX decks.
          </p>
        </motion.div>

        {/* Bento Box 4: Wide Feature - One-Command Online Publish (differentiator) */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ delay: 0.4 }}
          className="md:col-span-3 bg-surface-low p-10 rounded-3xl flex flex-col md:flex-row items-center gap-12 group hover:bg-surface-high transition-all border border-primary/15 relative overflow-hidden"
        >
          <div className="absolute bottom-0 left-1/2 -translate-x-1/2 w-full h-32 bg-primary/10 rounded-full blur-3xl transition-opacity group-hover:opacity-100 opacity-60 pointer-events-none"></div>
          <div className="flex-1 relative z-10">
            <div className="flex items-center gap-3 mb-6">
              <Share2 className="text-primary w-10 h-10" />
              <span className="text-[10px] font-headline uppercase tracking-[0.25em] text-primary border border-primary/30 bg-primary/10 px-2.5 py-1 rounded-full">Differentiator</span>
            </div>
            <h3 className="font-headline text-3xl font-bold text-white mb-4">One-Command Online Publish</h3>
            <p className="text-outline-variant max-w-2xl text-lg leading-relaxed mb-5">
              Most AI document CLIs stop at a local file. OfficeCLI ships an end-to-end publish path: every successful generation can become a password-protected online preview link in the same command. Share the URL with reviewers without uploading, hosting, or wiring extra services.
            </p>
            <ul className="grid sm:grid-cols-2 gap-x-6 gap-y-3 text-sm text-outline-variant max-w-2xl">
              <li className="flex items-start gap-2">
                <Link2 className="text-primary w-4 h-4 mt-0.5 shrink-0" />
                <span>Returns a shareable <span className="text-white font-medium">officecli.io/p/&lt;id&gt;</span> URL on success.</span>
              </li>
              <li className="flex items-start gap-2">
                <Lock className="text-primary w-4 h-4 mt-0.5 shrink-0" />
                <span>Auto-generated <span className="text-white font-medium">access password</span> protects each preview by default.</span>
              </li>
              <li className="flex items-start gap-2">
                <Terminal className="text-primary w-4 h-4 mt-0.5 shrink-0" />
                <span>Toggle per command with <span className="text-white font-medium">--no-publish</span> or globally via <span className="text-white font-medium">config set-publish</span>.</span>
              </li>
              <li className="flex items-start gap-2">
                <Bolt className="text-primary w-4 h-4 mt-0.5 shrink-0" />
                <span>Standalone <span className="text-white font-medium">new img</span> publishes by default — instant share for launch visuals.</span>
              </li>
            </ul>
          </div>
          <div className="hidden lg:flex w-80 shrink-0 flex-col gap-2 bg-surface-high rounded-xl border border-outline-variant/15 p-4 relative z-10 shadow-inner font-mono">
            <div className="flex items-center gap-1.5 mb-2">
              <div className="w-2 h-2 rounded-full bg-[#ff5f56]"></div>
              <div className="w-2 h-2 rounded-full bg-[#ffbd2e]"></div>
              <div className="w-2 h-2 rounded-full bg-[#27c93f]"></div>
              <span className="ml-2 text-[9px] font-headline text-outline uppercase tracking-[0.2em]">share_preview</span>
            </div>
            <div className="text-[11px]">
              <span className="text-tertiary">$</span> <span className="text-[#27c93f]">officecli</span> <span className="text-[#8af3f7]">new</span> <span className="text-[#ffbd2e]">pptx</span> <span className="text-outline-variant">--prompt-file ./brief.md</span>
            </div>
            <div className="text-[10px] text-white">Generation completed.</div>
            <div className="text-[10px] text-white leading-relaxed">
              Preview URL: <span className="text-primary">officecli.io/p/xyz123</span>
            </div>
            <div className="text-[10px] text-outline-variant">password: abcdef</div>
            <motion.div
              animate={{ opacity: [0.3, 1, 0.3] }}
              transition={{ repeat: Infinity, duration: 2 }}
              className="mt-1 inline-flex items-center gap-1 text-[9px] font-headline uppercase tracking-[0.2em] text-tertiary"
            >
              <span className="w-1.5 h-1.5 rounded-full bg-tertiary"></span>
              link ready to share
            </motion.div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
