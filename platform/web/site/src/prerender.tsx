function compact(lines: string[]) {
  return lines.join('\n').trim()
}

export function renderRouteApp(pathname: string) {
  if (pathname !== '/') {
    return '<main class="overflow-x-hidden"></main>'
  }

  return compact([
    '<div class="min-h-screen bg-background text-white selection:bg-primary/30">',
    '  <main class="overflow-x-hidden">',
    '    <section class="relative min-h-[90vh] flex items-center px-8 md:px-16 max-w-[1440px] mx-auto pt-20">',
    '      <div class="grid grid-cols-1 lg:grid-cols-2 gap-16 items-center w-full">',
    '        <div class="z-10">',
    '          <span class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[#0d4c54]/40 text-[#8af3f7] text-xs font-headline uppercase tracking-widest mb-6 border border-[#0d4c54]/60">Local-first AI document generation CLI</span>',
    '          <h1 class="font-headline text-6xl md:text-8xl font-bold tracking-tighter leading-[0.9] mb-8 text-white">Generate <span class="text-primary italic">PPTX</span>, DOCX, XLSX, and REPORT Outputs From One AI CLI</h1>',
    '          <p class="text-xl text-outline-variant max-w-xl mb-10 leading-relaxed font-light">OfficeCLI is a local-first AI document generation CLI for terminal workflows. Generate PPTX, DOCX, XLSX, and workbook-backed REPORT outputs with your own LLM endpoint, without a backend stack or cluster, then review presentations and expand into broader document automation.</p>',
    '        </div>',
    '      </div>',
    '    </section>',
    '    <section id="what-is-officecli" class="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">',
    '      <h2 class="font-headline text-4xl md:text-5xl font-bold text-white tracking-tight mb-4">What Is <span class="text-primary italic">OfficeCLI</span></h2>',
    '      <p class="text-outline-variant text-lg max-w-2xl">OfficeCLI is a local-first AI document generation CLI for developers and automation teams. It keeps document automation close to your terminal with one binary, your own LLM endpoint, and no required backend stack.</p>',
    '    </section>',
    '    <section id="formats" class="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">',
    '      <h2 class="font-headline text-4xl font-bold text-white mb-6">Generate PPTX, DOCX, XLSX, and REPORT Outputs</h2>',
    '      <p class="text-outline-variant text-lg mb-8 leading-relaxed">Stay in the terminal and run AI document automation from one local-first CLI. Install once, connect your LLM, and generate PPTX, DOCX, XLSX, and REPORT outputs without standing up extra infrastructure.</p>',
    '    </section>',
    '    <section id="download" class="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">',
    '      <h2 class="font-headline font-bold text-white tracking-tight text-4xl md:text-5xl mb-5">Install OfficeCLI</h2>',
    '      <p class="text-outline-variant text-lg leading-relaxed max-w-3xl">Pick the setup path that matches your machine. OfficeCLI stays lightweight: one binary, plus your LLM endpoint for the core local workflow.</p>',
    '    </section>',
    '    <section id="pricing" class="px-8 md:px-16 max-w-[1440px] mx-auto text-center py-32">',
    '      <h2 class="font-headline text-5xl md:text-6xl font-bold text-white mb-8 tracking-tighter">Simple Access for Paid Workflows</h2>',
    '    </section>',
    '    <section id="faq" class="px-8 md:px-16 max-w-[1440px] mx-auto py-24">',
    '      <h2 class="font-headline font-bold text-white mb-16 text-center text-4xl">Frequently Asked Questions</h2>',
    '      <div class="grid grid-cols-1 md:grid-cols-2 gap-12 max-w-5xl mx-auto">',
    '        <div class="space-y-4"><h4 class="font-bold text-white text-lg">What document types work today?</h4><p class="text-outline-variant text-sm leading-relaxed">The current public release generates PPTX, DOCX, XLSX, and workbook-backed REPORT outputs. It can also score and review local PPTX files.</p></div>',
    '      </div>',
    '    </section>',
    '  </main>',
    '</div>',
  ])
}
