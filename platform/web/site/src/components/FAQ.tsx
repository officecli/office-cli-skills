interface FAQProps {
  standalone?: boolean
  compact?: boolean
}

export default function FAQ({ standalone = false, compact = false }: FAQProps) {
  const faqs = [
    {
      q: 'Is OfficeCLI only for generating files?',
      a: 'No. Generation is the current center of gravity, but the product direction is broader document operations: conversion, content modification, summarization, extraction, and layout handling.',
    },
    {
      q: 'Do I need Docker, Kubernetes, or a backend?',
      a: 'No for the core local workflow. OfficeCLI is designed to stay lightweight: one binary plus your LLM endpoint. Platform features are optional, not a requirement for basic local use.',
    },
    {
      q: 'What document types work today?',
      a: 'The current public release generates PPTX, DOCX, XLSX, and HTML. It can also score and review local PPTX files.',
    },
    {
      q: 'Do I need LibreOffice or Microsoft Office installed?',
      a: 'Not for generation. PPTX review can run structural checks without extra tools. If `soffice` is installed, OfficeCLI can add a stronger visual review pass.',
    },
    {
      q: 'What install options are supported?',
      a: 'Homebrew, npm, the official install script, and manual release binaries are all supported on macOS and Linux for x64 and arm64.',
    },
    {
      q: 'When do I need platform.officecli.io?',
      a: 'Use the platform when you need paid access management, hosted runtime features, billing, API-key workflows, or optional online preview publishing.',
    },
  ]

  return (
    <section id="faq" className={`px-8 md:px-16 max-w-[1440px] mx-auto ${standalone ? 'pb-24' : 'py-24'}`}>
      <h2 className={`font-headline font-bold text-white mb-16 text-center ${compact ? 'text-3xl' : 'text-4xl'}`}>Frequently Asked Questions</h2>
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
