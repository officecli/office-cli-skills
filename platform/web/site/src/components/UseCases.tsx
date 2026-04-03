import { motion } from 'motion/react'
import { ArrowRight } from 'lucide-react'
import { Link } from 'react-router-dom'
import enterpriseVisual from '../assets/use-case-enterprise.svg'
import indieVisual from '../assets/use-case-indie.svg'
import teamVisual from '../assets/use-case-team.svg'

export default function UseCases() {
  const cases = [
    {
      title: 'Indie Devs',
      description: 'Automate user reports and exportable summaries for your SaaS without building complex PDF or DOCX engines from scratch.',
      image: indieVisual,
      borderClass: 'hover:border-primary/20',
    },
    {
      title: 'Small Teams',
      description: 'Integrate document production into internal Slack bots or customer portals to provide value-added report downloads instantly.',
      image: teamVisual,
      borderClass: 'hover:border-tertiary/20',
    },
    {
      title: 'Enterprise',
      description: 'High-availability API nodes, dedicated support, and custom templating engines for massive multi-format batch production.',
      image: enterpriseVisual,
      borderClass: 'hover:border-secondary/20',
    },
  ]

  return (
    <section className="py-24 bg-surface-low">
      <div className="max-w-[1440px] mx-auto px-8 md:px-16">
        <div className="flex flex-col md:flex-row justify-between items-end mb-16 gap-8">
          <div className="max-w-2xl">
            <h2 className="font-headline text-4xl font-bold text-white mb-4">Scalable For Every Tier</h2>
            <p className="text-outline-variant">Whether you are shipping a side project or managing global report generation for an enterprise, this infrastructure scales with you.</p>
          </div>
          <Link className="text-primary font-bold hover:underline flex items-center gap-2" to="/docs">
            Explore enterprise integration <ArrowRight className="w-5 h-5" />
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
