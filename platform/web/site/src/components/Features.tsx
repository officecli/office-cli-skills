import { motion } from 'motion/react'
import { Bolt, LaptopMinimal, Sparkles, ScanSearch } from 'lucide-react'

export default function Features() {
  const features = [
    {
      icon: <Bolt className="text-tertiary w-10 h-10 mb-6" />,
      title: 'Single-Binary Document Ops',
      description: 'OfficeCLI ships as one lightweight binary. For the core local path, it only needs your LLM endpoint instead of a backend stack, queueing layer, or cluster.',
      footer: 'Local-first by default',
      large: true,
    },
    {
      icon: <LaptopMinimal className="text-primary w-10 h-10 mb-6" />,
      title: 'Fits Local and Automated Workflows',
      description: 'Run the same CLI on your laptop, in CI, or inside agent workflows without switching to a separate document backend.',
      large: false,
    },
    {
      icon: <ScanSearch className="text-secondary w-10 h-10 mb-6" />,
      title: 'Create, Review, Export Today',
      description: 'The current release creates PPTX, DOCX, XLSX, and HTML outputs, and can review or score local PPTX decks.',
      large: false,
    },
  ]

  return (
    <section className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        {features.map((feature, index) => (
          <motion.div 
            key={index}
            initial={{ opacity: 0, y: 20 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true }}
            transition={{ delay: index * 0.1 }}
            className={`${feature.large ? 'md:col-span-2' : 'md:col-span-1'} bg-surface-low p-10 rounded-xl flex flex-col justify-between group hover:bg-surface-high transition-all border border-white/5`}
          >
            <div>
              {feature.icon}
              <h3 className="font-headline text-3xl font-bold text-white mb-4">{feature.title}</h3>
              <p className="text-outline-variant leading-relaxed">{feature.description}</p>
            </div>
            {feature.footer && (
              <div className="mt-8 border-t border-outline-variant/10 pt-6">
                <span className="text-xs font-headline uppercase text-tertiary tracking-widest">{feature.footer}</span>
              </div>
            )}
          </motion.div>
        ))}

        <motion.div 
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ delay: 0.3 }}
          className="md:col-span-4 bg-surface-low p-10 rounded-xl flex flex-col md:flex-row items-center gap-12 group hover:bg-surface-high transition-all border border-white/5"
        >
          <div className="flex-1">
            <Sparkles className="text-primary w-10 h-10 mb-6" />
            <h3 className="font-headline text-3xl font-bold text-white mb-4">Built for Broader Document Operations</h3>
            <p className="text-outline-variant max-w-2xl">OfficeCLI starts with creation and PPT review, then expands toward conversion, modification, summarization, and richer document formatting workflows.</p>
          </div>
          <div className="hidden lg:flex w-48 h-24 bg-surface-high rounded border border-outline-variant/10 items-center justify-center">
            <div className="flex gap-2 items-end">
              <motion.div animate={{ height: [20, 40, 20] }} transition={{ repeat: Infinity, duration: 2 }} className="w-2 bg-tertiary h-8"></motion.div>
              <motion.div animate={{ height: [30, 60, 30] }} transition={{ repeat: Infinity, duration: 2.5 }} className="w-2 bg-tertiary h-12"></motion.div>
              <motion.div animate={{ height: [15, 30, 15] }} transition={{ repeat: Infinity, duration: 1.8 }} className="w-2 bg-tertiary h-6"></motion.div>
              <motion.div animate={{ height: [25, 50, 25] }} transition={{ repeat: Infinity, duration: 2.2 }} className="w-2 bg-tertiary h-10"></motion.div>
            </div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
