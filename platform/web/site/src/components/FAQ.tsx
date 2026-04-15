import { homeFAQs } from '../seo'

interface FAQProps {
  standalone?: boolean
  compact?: boolean
}

export default function FAQ({ standalone = false, compact = false }: FAQProps) {
  return (
    <section id="faq" className={`px-8 md:px-16 max-w-[1440px] mx-auto ${standalone ? 'pb-24' : 'py-24'}`}>
      <h2 className={`font-headline font-bold text-white mb-16 text-center ${compact ? 'text-3xl' : 'text-4xl'}`}>Frequently Asked Questions</h2>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-12 max-w-5xl mx-auto">
        {homeFAQs.map((faq) => (
          <div key={faq.q} className="space-y-4">
            <h4 className="font-bold text-white text-lg">{faq.q}</h4>
            <p className="text-outline-variant text-sm leading-relaxed">{faq.a}</p>
          </div>
        ))}
      </div>
    </section>
  )
}
