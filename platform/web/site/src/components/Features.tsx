import { motion } from 'motion/react'
import { Bolt, LaptopMinimal, Sparkles, ScanSearch, Terminal, Bot, Plug, ArrowRight } from 'lucide-react'

export default function Features() {
  return (
    <section className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="mb-16">
        <h2 className="font-headline text-4xl md:text-5xl font-bold text-white tracking-tight mb-4">
          Everything you need, <span className="text-primary italic">locally</span>.
        </h2>
        <p className="text-outline-variant text-lg max-w-2xl">
          OfficeCLI is built for modern developer workflows. No complex backend setups, just a single binary and your LLM endpoint.
        </p>
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
                <h3 className="font-headline text-4xl font-bold text-white mb-4">Single-Binary Document Ops</h3>
                <p className="text-outline-variant text-lg leading-relaxed max-w-md">
                  OfficeCLI ships as one lightweight binary. For the core local path, it only needs your LLM endpoint instead of a backend stack, queueing layer, or cluster.
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
                <span className="font-bold text-white block mb-0.5">AI Agents</span>
                <span className="text-outline-variant">Embed in custom agentic workflows</span>
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
            The current release creates PPTX, DOCX, XLSX, and HTML outputs, and can review or score local PPTX decks.
          </p>
        </motion.div>

        {/* Bento Box 4: Wide Feature */}
        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ delay: 0.4 }}
          className="md:col-span-3 bg-surface-low p-10 rounded-3xl flex flex-col md:flex-row items-center gap-12 group hover:bg-surface-high transition-all border border-white/5 relative overflow-hidden"
        >
          <div className="absolute bottom-0 left-1/2 -translate-x-1/2 w-full h-32 bg-primary/5 rounded-full blur-3xl transition-opacity group-hover:opacity-100 opacity-50 pointer-events-none"></div>
          <div className="flex-1 relative z-10">
            <Sparkles className="text-primary w-10 h-10 mb-6" />
            <h3 className="font-headline text-3xl font-bold text-white mb-4">Built for Broader Document Operations</h3>
            <p className="text-outline-variant max-w-2xl text-lg">
              OfficeCLI starts with creation and PPT review, then expands toward conversion, modification, summarization, and richer document formatting workflows.
            </p>
          </div>
          <div className="hidden lg:flex w-64 h-32 bg-surface-high rounded-xl border border-outline-variant/10 items-center justify-center relative z-10 shadow-inner">
            <div className="flex gap-3 items-end">
              <motion.div animate={{ height: [24, 48, 24] }} transition={{ repeat: Infinity, duration: 2 }} className="w-3 rounded-t-sm bg-tertiary h-10"></motion.div>
              <motion.div animate={{ height: [36, 72, 36] }} transition={{ repeat: Infinity, duration: 2.5 }} className="w-3 rounded-t-sm bg-tertiary h-14"></motion.div>
              <motion.div animate={{ height: [18, 36, 18] }} transition={{ repeat: Infinity, duration: 1.8 }} className="w-3 rounded-t-sm bg-tertiary h-8"></motion.div>
              <motion.div animate={{ height: [30, 60, 30] }} transition={{ repeat: Infinity, duration: 2.2 }} className="w-3 rounded-t-sm bg-tertiary h-12"></motion.div>
              <motion.div animate={{ height: [40, 80, 40] }} transition={{ repeat: Infinity, duration: 2.7 }} className="w-3 rounded-t-sm bg-tertiary h-16"></motion.div>
            </div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
