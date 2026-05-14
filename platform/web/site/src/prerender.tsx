import { agentSkillsHubPath, agentSkillsRoutes, generationDemos, getAgentSkillsRoute, legacyAgentSkillsPath } from './agentSkillsData'

function compact(lines: string[]) {
  return lines.join('\n').trim()
}

function renderAgentSkillsRoute(pathname: string) {
  const normalizedPath = pathname === legacyAgentSkillsPath ? agentSkillsHubPath : pathname
  const route = getAgentSkillsRoute(normalizedPath)
  if (!route) {
    return ''
  }

  if (normalizedPath === agentSkillsHubPath) {
    return compact([
      '<div class="min-h-screen bg-background text-white selection:bg-primary/30">',
      '  <main class="overflow-x-hidden">',
      '    <section class="px-8 md:px-16 pt-24 pb-20 max-w-[1440px] mx-auto">',
      '      <span class="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-primary/8 text-primary text-xs font-headline uppercase tracking-[0.28em] mb-6 border border-primary/25">officecli public repository</span>',
      '      <h1 class="font-headline text-5xl md:text-7xl font-bold tracking-tight text-white max-w-5xl">officecli for Claude Code, Codex, and AI Agents</h1>',
      '      <p class="text-lg md:text-xl text-outline-variant leading-relaxed max-w-4xl mt-6">officecli is the public GitHub repository and install surface for Claude Code, Codex, OpenClaw, and other AI agents that need local PPTX, DOCX, XLSX, and workbook-backed report workflows.</p>',
      '    </section>',
      '    <section class="px-8 md:px-16 py-6 max-w-[1440px] mx-auto">',
      '      <h2 class="font-headline text-4xl font-bold text-white mb-4">Primary entrypoints</h2>',
      '      <p class="text-outline-variant text-lg leading-relaxed max-w-3xl">Use the install, Claude Code, Codex, OpenClaw, and FAQ pages as stable child pages for search and onboarding instead of relying on random raw file URLs.</p>',
      '    </section>',
      '    <section class="px-8 md:px-16 py-12 max-w-[1440px] mx-auto">',
      '      <h2 class="font-headline text-4xl font-bold text-white mb-4">Generation demos</h2>',
      '      <p class="text-outline-variant text-lg leading-relaxed max-w-3xl">Preview images, generated files, prompts, commands, and metadata live together in the public repository.</p>',
      ...generationDemos.map((demo) => `      <p class="text-outline-variant text-sm leading-relaxed">${demo.title}: ${demo.artifactLabel}</p>`),
      '    </section>',
      '    <section class="px-8 md:px-16 py-12 max-w-[1440px] mx-auto">',
      '      <h2 class="font-headline text-4xl font-bold text-white mb-4">Install paths</h2>',
      '      <p class="text-outline-variant text-lg leading-relaxed max-w-3xl">Choose Claude Code marketplace install, direct Codex-style skill install, or the OpenClaw installer depending on the agent runtime.</p>',
      '      <p class="text-outline-variant text-sm leading-relaxed max-w-3xl mt-4">GitHub repository: https://github.com/officecli/officecli</p>',
      '    </section>',
      '  </main>',
      '</div>',
    ])
  }

  return compact([
    '<div class="min-h-screen bg-background text-white selection:bg-primary/30">',
    '  <main class="overflow-x-hidden">',
    '    <section class="px-8 md:px-16 pt-24 pb-20 max-w-[1440px] mx-auto">',
    `      <h1 class="font-headline text-5xl md:text-7xl font-bold tracking-tight text-white max-w-5xl">${route.title}</h1>`,
    `      <p class="text-lg md:text-xl text-outline-variant leading-relaxed max-w-4xl mt-6">${route.description}</p>`,
    '      <p class="text-outline-variant text-sm leading-relaxed max-w-3xl mt-4">Related pages: officecli overview, install, Claude Code, Codex, OpenClaw, and FAQ.</p>',
    '    </section>',
    '  </main>',
    '</div>',
  ])
}

export function renderRouteApp(pathname: string) {
  if (pathname === legacyAgentSkillsPath || agentSkillsRoutes.some((route) => route.path === pathname)) {
    return renderAgentSkillsRoute(pathname)
  }

  if (pathname !== '/') {
    return '<main class="overflow-x-hidden"></main>'
  }

  return compact([
    '<div class="min-h-screen bg-background text-white selection:bg-primary/30">',
    '  <main class="overflow-x-hidden">',
    '    <section class="relative min-h-[90vh] flex items-center px-8 md:px-16 max-w-[1440px] mx-auto pt-20">',
    '      <div class="grid grid-cols-1 lg:grid-cols-2 gap-16 items-center w-full">',
    '        <div class="z-10">',
    '          <span class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[#0d4c54]/40 text-[#8af3f7] text-xs font-headline uppercase tracking-widest mb-6 border border-[#0d4c54]/60">External + Hosted modes · One binary · No agent required</span>',
    '          <h1 class="font-headline text-6xl md:text-8xl font-bold tracking-tighter leading-[0.9] mb-8 text-white">AI Document Generation in External or Hosted Mode</h1>',
    '          <p class="text-xl text-outline-variant max-w-xl mb-10 leading-relaxed font-light">External: free unlimited with your own model endpoint. Hosted: OfficeCLI-managed runtime with hosted credits. Both modes use the same dependency-free officecli binary for PPTX, DOCX, XLSX, REPORT, and IMG outputs.</p>',
    '        </div>',
    '      </div>',
    '    </section>',
    '    <section id="what-is-officecli" class="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">',
    '      <h2 class="font-headline text-4xl md:text-5xl font-bold text-white tracking-tight mb-4">What Is <span class="text-primary italic">OfficeCLI</span></h2>',
    '      <p class="text-outline-variant text-lg max-w-2xl">OfficeCLI is a local-first AI document generation CLI for developers and automation teams. It keeps document automation close to your terminal with one binary, your own LLM endpoint, and no required backend stack.</p>',
    '    </section>',
    '    <section id="formats" class="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">',
    '      <h2 class="font-headline text-4xl font-bold text-white mb-6">Generate, Review, and Publish From One CLI</h2>',
    '      <p class="text-outline-variant text-lg mb-8 leading-relaxed">Stay in the terminal and run AI document automation from one local-first CLI. Install once, connect your LLM, generate PPTX, DOCX, XLSX, REPORT, and standalone IMG outputs, then publish a password-protected online preview link in the same command — no extra hosting, gateway, or upload step.</p>',
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
    '        <div class="space-y-4"><h4 class="font-bold text-white text-lg">What document types work today?</h4><p class="text-outline-variant text-sm leading-relaxed">The current public release generates PPTX, DOCX, XLSX, workbook-backed REPORT outputs, and standalone IMG visuals via `new img`. It can also score and review local PPTX files.</p></div>',
    '      </div>',
    '    </section>',
    '  </main>',
    '</div>',
  ])
}
