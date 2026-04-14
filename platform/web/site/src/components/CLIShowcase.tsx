import { motion } from 'motion/react'
import { CheckCircle2 } from 'lucide-react'

export default function CLIShowcase() {
  return (
    <section className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-16">
        <motion.div
          initial={{ opacity: 0, x: -20 }}
          whileInView={{ opacity: 1, x: 0 }}
          viewport={{ once: true }}
        >
          <h2 className="font-headline text-4xl font-bold text-white mb-6">A Practical CLI for Document Operations</h2>
          <p className="text-outline-variant text-lg mb-8 leading-relaxed">Stay in the terminal. Install once, connect an LLM, and run document operations without standing up extra infrastructure for the core workflow.</p>
          <ul className="space-y-6">
            <li className="flex items-start gap-4">
              <CheckCircle2 className="text-tertiary w-6 h-6 mt-1" />
              <div>
                <h5 className="font-bold text-white">Lightweight by design</h5>
                <p className="text-sm text-outline-variant">Core local usage does not require a cluster, backend service, Docker, or Kubernetes.</p>
              </div>
            </li>
            <li className="flex items-start gap-4">
              <CheckCircle2 className="text-tertiary w-6 h-6 mt-1" />
              <div>
                <h5 className="font-bold text-white">Current release surface is explicit</h5>
                <p className="text-sm text-outline-variant">Use `new`, `score`, and `review` today, while broader conversion and editing workflows land next.</p>
              </div>
            </li>
          </ul>
        </motion.div>

        <motion.div 
          initial={{ opacity: 0, x: 20 }}
          whileInView={{ opacity: 1, x: 0 }}
          viewport={{ once: true }}
          className="bg-surface-low rounded-xl border border-outline-variant/20 overflow-hidden font-mono text-sm"
        >
          <div className="bg-surface-high px-4 py-2 border-b border-outline-variant/10 flex justify-between items-center">
            <span className="text-[10px] text-outline font-headline uppercase tracking-widest">Bash - officecli v0.2.5</span>
            <div className="flex gap-2">
              <div className="w-2 h-2 rounded-full bg-outline-variant/30"></div>
              <div className="w-2 h-2 rounded-full bg-outline-variant/30"></div>
            </div>
          </div>
          <div className="p-8 space-y-4">
            <div className="flex gap-4">
              <span className="text-tertiary italic"># Install via Homebrew or npm</span>
            </div>
            <div className="flex gap-4">
              <span className="text-primary">$</span>
              <span className="text-white">brew install officecli/homebrew-officecli/officecli</span>
            </div>
            <div className="flex gap-4">
              <span className="text-primary">$</span>
              <span className="text-white">npm install -g officecli</span>
            </div>
            <div className="pt-4 flex gap-4">
              <span className="text-tertiary italic"># Configure the runtime and run a document operation</span>
            </div>
            <div className="flex gap-4">
              <span className="text-primary">$</span>
              <span className="text-white">officecli config set-generation</span>
            </div>
            <div className="flex gap-4">
              <span className="text-primary">$</span>
              <span className="text-white">officecli new pptx "Q3 Business Review" --prompt-file ./brief.md --no-publish</span>
            </div>
            <motion.div 
              animate={{ opacity: [0.5, 1, 0.5] }}
              transition={{ repeat: Infinity, duration: 1.5 }}
              className="text-tertiary mt-4"
            >
              Writing output/q3-business-review.pptx [##########] 100%
            </motion.div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
