import { motion } from 'motion/react'
import { Terminal, Settings2, WandSparkles, Share2 } from 'lucide-react'

export default function Workflow() {
  const steps = [
    { icon: <Terminal className="text-primary" />, title: 'Install', subtitle: 'Binary via brew, npm, script, or manual release' },
    { icon: <Settings2 className="text-tertiary" />, title: 'Connect', subtitle: 'Point the runtime at your LLM endpoint' },
    { icon: <WandSparkles className="text-secondary" />, title: 'Generate', subtitle: 'Run a document or image operation from the terminal' },
    { icon: <Share2 className="text-primary-container" />, title: 'Publish', subtitle: 'Share a password-protected preview URL with one command' },
  ]

  return (
    <section className="py-24 bg-background border-y border-white/5">
      <div className="max-w-[1440px] mx-auto px-8 md:px-16 text-center">
        <h2 className="font-headline text-4xl md:text-5xl font-bold text-white mb-6 tracking-tight">From Terminal Generation to Sharable Preview Link</h2>
        <p className="text-outline-variant text-lg max-w-3xl mx-auto mb-16">
          Keep the document path local when you want a lightweight setup, then turn any successful generation into a password-protected online preview link without leaving the terminal — a publish path no other AI document CLI ships end-to-end.
        </p>
        <div className="flex flex-col md:flex-row items-center justify-between gap-8 relative">
          <div className="hidden md:block absolute top-1/2 left-0 w-full h-[1px] bg-gradient-to-r from-transparent via-outline-variant/30 to-transparent -translate-y-1/2 z-0"></div>
          
          {steps.map((step, index) => (
            <motion.div 
              key={index}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ delay: index * 0.1 }}
              className="relative z-10 flex flex-col items-center group w-full max-w-[280px]"
            >
              <div className="w-16 h-16 rounded-full bg-surface-high border border-outline-variant/20 flex items-center justify-center mb-6 group-hover:scale-110 transition-all duration-300">
                {step.icon}
              </div>
              <h4 className="font-headline text-lg font-bold text-white mb-2">{step.title}</h4>
              <p className="text-[10px] text-outline-variant font-headline uppercase tracking-widest">{step.subtitle}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
