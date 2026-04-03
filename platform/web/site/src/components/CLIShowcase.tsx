import { motion } from "motion/react";
import { CheckCircle2 } from "lucide-react";

export default function CLIShowcase() {
  return (
    <section className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-16">
        <motion.div
          initial={{ opacity: 0, x: -20 }}
          whileInView={{ opacity: 1, x: 0 }}
          viewport={{ once: true }}
        >
          <h2 className="font-headline text-4xl font-bold text-white mb-6">Engineered for the Terminal</h2>
          <p className="text-outline-variant text-lg mb-8 leading-relaxed">Don't break your focus. OfficeCLI lives where you do. Integrated flags for styling, data sourcing, and output destinations make automation seamless.</p>
          <ul className="space-y-6">
            <li className="flex items-start gap-4">
              <CheckCircle2 className="text-tertiary w-6 h-6 mt-1" />
              <div>
                <h5 className="font-bold text-white">Headless First</h5>
                <p className="text-sm text-outline-variant">Runs perfectly in Docker, Lambda, or your local shell.</p>
              </div>
            </li>
            <li className="flex items-start gap-4">
              <CheckCircle2 className="text-tertiary w-6 h-6 mt-1" />
              <div>
                <h5 className="font-bold text-white">Style Templates</h5>
                <p className="text-sm text-outline-variant">Apply consistent branding via CSS-like theme files.</p>
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
            <span className="text-[10px] text-outline font-headline uppercase tracking-widest">Bash — officecli-v1.0.4</span>
            <div className="flex gap-2">
              <div className="w-2 h-2 rounded-full bg-outline-variant/30"></div>
              <div className="w-2 h-2 rounded-full bg-outline-variant/30"></div>
            </div>
          </div>
          <div className="p-8 space-y-4">
            <div className="flex gap-4">
              <span className="text-tertiary italic"># Install CLI</span>
            </div>
            <div className="flex gap-4">
              <span className="text-primary">$</span>
              <span className="text-white">brew install officecli</span>
            </div>
            <div className="pt-4 flex gap-4">
              <span className="text-tertiary italic"># Authenticate and generate output</span>
            </div>
            <div className="flex gap-4">
              <span className="text-primary">$</span>
              <span className="text-white">officecli generate ./brief.md --output ./artifacts/q3-report.pptx</span>
            </div>
            <motion.div 
              animate={{ opacity: [0.5, 1, 0.5] }}
              transition={{ repeat: Infinity, duration: 1.5 }}
              className="text-tertiary mt-4"
            >
              Writing artifacts/q3-report.pptx [##########] 100%
            </motion.div>
          </div>
        </motion.div>
      </div>
    </section>
  );
}
