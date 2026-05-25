import { motion } from 'motion/react'
import { Code2, FileStack, Sparkles, Workflow as WorkflowIcon } from 'lucide-react'

export default function VibeOfficing() {
  return (
    <section id="vibeofficing" className="py-24 px-6 md:px-16 max-w-[1440px] mx-auto">
      <div className="mb-12 max-w-3xl">
        <span className="mb-4 inline-flex items-center gap-2 rounded-full border border-primary/30 bg-primary/10 px-3 py-1 text-[11px] font-headline uppercase tracking-widest text-primary">
          <Sparkles className="w-3 h-3" />
          A New Paradigm
        </span>
        <h2 className="font-headline text-4xl md:text-5xl font-bold text-white tracking-tight mb-5">
          Meet <span className="text-primary italic">VibeOfficing</span> — AI-Native Document Work
        </h2>
        <p className="text-outline-variant text-lg leading-relaxed">
          What VibeCoding did for developers, <span className="text-white font-medium">VibeOfficing</span> does for everyone who lives inside DOCX, PPTX, and XLSX. Instead of clicking through ribbons, you describe intent — and an AI-native workspace handles the OOXML, the formatting memory, and the export. OfficeCLI and the OfficeDex desktop app are the two surfaces of this paradigm.
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.05fr)] gap-10 lg:items-stretch">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5 }}
          className="flex flex-col gap-5"
        >
          <div className="bg-surface-low rounded-2xl border border-white/5 p-6 hover:bg-surface-high transition-all">
            <div className="flex items-start gap-4">
              <div className="shrink-0 w-11 h-11 rounded-xl bg-tertiary/15 border border-tertiary/25 flex items-center justify-center">
                <Code2 className="w-5 h-5 text-tertiary" />
              </div>
              <div>
                <h3 className="font-headline text-xl font-bold text-white mb-2">VibeCoding → VibeOfficing</h3>
                <p className="text-outline-variant leading-relaxed text-sm">
                  Claude · Codex turned source code into a conversation with the machine. OfficeDex applies the same loop to office documents: prompts in, native <code className="text-primary">.docx</code> / <code className="text-primary">.pptx</code> / <code className="text-primary">.xlsx</code> out — no boilerplate templates, no manual layout fixes.
                </p>
              </div>
            </div>
          </div>

          <div className="bg-surface-low rounded-2xl border border-white/5 p-6 hover:bg-surface-high transition-all">
            <div className="flex items-start gap-4">
              <div className="shrink-0 w-11 h-11 rounded-xl bg-primary/15 border border-primary/25 flex items-center justify-center">
                <FileStack className="w-5 h-5 text-primary" />
              </div>
              <div>
                <h3 className="font-headline text-xl font-bold text-white mb-2">Your Workspace, Remembered</h3>
                <p className="text-outline-variant leading-relaxed text-sm">
                  Scanned physical docs, Markdown notes, existing Word reports, meeting transcripts, spreadsheets, reference images — they all feed the OfficeDex Engine, so generated output already speaks <span className="text-white">your</span> style and habits instead of a generic AI aesthetic.
                </p>
              </div>
            </div>
          </div>

          <div className="bg-surface-low rounded-2xl border border-white/5 p-6 hover:bg-surface-high transition-all">
            <div className="flex items-start gap-4">
              <div className="shrink-0 w-11 h-11 rounded-xl bg-secondary/15 border border-secondary/25 flex items-center justify-center">
                <WorkflowIcon className="w-5 h-5 text-secondary" />
              </div>
              <div>
                <h3 className="font-headline text-xl font-bold text-white mb-2">Memory · Format · Agent</h3>
                <p className="text-outline-variant leading-relaxed text-sm">
                  Three engines work together: <span className="text-white">Memory</span> remembers how your docs look, <span className="text-white">Format</span> handles OOXML natively, and <span className="text-white">Agents</span> collaborate to produce slide decks, reports, analyses, chart exports, and dashboard summaries.
                </p>
              </div>
            </div>
          </div>

          <p className="text-sm text-outline-variant italic px-2">
            Digitize your style, your habits, your design language into OfficeDex — never start from scratch again.
          </p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, scale: 0.97 }}
          whileInView={{ opacity: 1, scale: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 0.55 }}
          className="relative rounded-3xl border border-primary/15 bg-gradient-to-br from-surface-low to-background p-3 md:p-4 shadow-[0_0_60px_rgba(174,198,255,0.08)] overflow-hidden lg:min-h-0"
        >
          <div className="absolute -inset-1 rounded-3xl bg-gradient-to-br from-primary/10 via-transparent to-tertiary/10 blur-2xl -z-10 pointer-events-none" />
          <img
            src="/vibeofficing.png?v=1"
            alt="OfficeDex — AI-Native VibeOfficing platform. VibeCoding (Claude, Codex) is to developers what VibeOfficing (OfficeDex) is to office users."
            loading="lazy"
            className="block w-full h-auto rounded-2xl lg:absolute lg:inset-4 lg:w-[calc(100%-2rem)] lg:h-[calc(100%-2rem)] lg:object-contain"
          />
        </motion.div>
      </div>
    </section>
  )
}
