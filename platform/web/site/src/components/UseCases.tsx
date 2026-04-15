import { motion } from 'motion/react'
import { ArrowRight } from 'lucide-react'
import { Link } from 'react-router-dom'
import enterpriseVisual from '../assets/use-case-enterprise.svg'
import indieVisual from '../assets/use-case-indie.svg'
import teamVisual from '../assets/use-case-team.svg'

export default function UseCases() {
  const cases = [
    {
      title: 'Product Teams',
      description: 'Create presentations, documents, spreadsheets, and workbook-backed report outputs for launches, business reviews, and internal communication without maintaining separate document stacks.',
      image: indieVisual,
      borderClass: 'hover:border-primary/20',
    },
    {
      title: 'Automation Builders',
      description: 'Wire document operations into CI, custom AI agents, or OpenClaw workflows that need lightweight local execution and reproducible outputs.',
      image: teamVisual,
      borderClass: 'hover:border-tertiary/20',
    },
    {
      title: 'Ops and Research Teams',
      description: 'Use one CLI for document creation and review today, then expand into summarization, conversion, and editing workflows as the product evolves.',
      image: enterpriseVisual,
      borderClass: 'hover:border-secondary/20',
    },
  ]

  return (
    <section className="py-24 bg-surface-low">
      <div className="max-w-[1440px] mx-auto px-8 md:px-16">
        <div className="flex flex-col md:flex-row justify-between items-end mb-16 gap-8">
          <div className="max-w-2xl">
            <h2 className="font-headline text-4xl font-bold text-white mb-4">Where Document Operations Actually Matter</h2>
            <p className="text-outline-variant">OfficeCLI is built for teams that want useful document workflows in the terminal first, then optional platform features when hosted access, billing, or preview publishing matters.</p>
          </div>
          <Link className="text-primary font-bold hover:underline flex items-center gap-2" to="/docs">
            Explore current capabilities <ArrowRight className="w-5 h-5" />
          </Link>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
          {cases.map((item, index) => (
            <motion.div
              key={index}
              initial={{ opacity: 0, scale: 0.95 }}
              whileInView={{ opacity: 1, scale: 1 }}
              viewport={{ once: true }}
              transition={{ delay: index * 0.1 }}
              className={`group bg-surface p-8 rounded-xl border border-transparent transition-all ${item.borderClass}`}
            >
              <div className="h-40 bg-surface-low rounded-lg mb-8 overflow-hidden">
                <img
                  className="w-full h-full object-cover opacity-30 group-hover:opacity-60 transition-all duration-500"
                  src={item.image}
                  alt={item.title}
                />
              </div>
              <h4 className="font-headline text-2xl font-bold text-white mb-4">{item.title}</h4>
              <p className="text-outline-variant text-sm leading-relaxed">{item.description}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
