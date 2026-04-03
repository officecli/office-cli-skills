interface FAQProps {
  standalone?: boolean
  compact?: boolean
}

export default function FAQ({ standalone = false, compact = false }: FAQProps) {
  const faqs = [
    {
      q: 'How is data handled?',
      a: 'OfficeCLI operates on a zero-persistence architecture. Once a document is generated and delivered to the requested output, raw input data is purged from processing nodes immediately.',
    },
    {
      q: 'Does it support custom fonts?',
      a: 'Yes. Upload custom TTF or OTF files or reference them via URL in your styling JSON to keep output aligned with your brand system.',
    },
    {
      q: 'API or CLI?',
      a: 'Both. The CLI is a workflow-friendly wrapper around the core API. Use the CLI for local automation and the API for product integrations.',
    },
    {
      q: 'What about bulk generation?',
      a: 'The platform is built for large-scale parallel work with managed queues, repeatable execution, and support for high-throughput production traffic.',
    },
  ]

  return (
    <section className={`px-8 md:px-16 max-w-[1440px] mx-auto ${standalone ? 'pb-24' : 'py-24'}`}>
      <h2 className={`font-headline font-bold text-white mb-16 text-center ${compact ? 'text-3xl' : 'text-4xl'}`}>Platform Assurance</h2>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-12 max-w-5xl mx-auto">
        {faqs.map((faq) => (
          <div key={faq.q} className="space-y-4">
            <h4 className="font-bold text-white text-lg">{faq.q}</h4>
            <p className="text-outline-variant text-sm leading-relaxed">{faq.a}</p>
          </div>
        ))}
      </div>
    </section>
  )
}
