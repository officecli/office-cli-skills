import fs from 'node:fs'
import path from 'node:path'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { detectOperatingSystem } from './installData'
import { renderRouteApp } from './prerender'
import { routeSEO } from './seo'

const seoLandingPaths = [
  '/ai-pptx-generator',
  '/docx-generator-cli',
  '/xlsx-report-generation',
  '/office-automation-ai-agents',
  '/officecli-vs-python-docx-openpyxl-libreoffice',
]

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('marketing site shell', () => {
  it('renders the OfficeCLI brand and home hero copy', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getAllByText('OfficeCLI').length).toBeGreaterThan(0)
    expect(
      screen.getByRole('heading', { name: /^officecli$/i, level: 1 }),
    ).toBeInTheDocument()
    expect(screen.getByText(/AI document generation, on every surface/i)).toBeInTheDocument()
    expect(screen.getAllByText(/One binary · Three surfaces · Plus a desktop app/i).length).toBeGreaterThan(0)
    expect(screen.getByRole('heading', { name: /^ROADMAP$/i, level: 2 })).toBeInTheDocument()
    expect(
      screen.getByText(/OfficeCLI is moving from document generation into a broader document operations workflow/i),
    ).toBeInTheDocument()
    expect(screen.getByText('support@officecli.io')).toBeInTheDocument()
  })

  it('copies the support email from the contact section', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {
      clipboard: { writeText },
    })

    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    fireEvent.click(screen.getAllByRole('button', { name: /Copy email/i }).at(-1)!)
    expect(writeText).toHaveBeenCalledWith('support@officecli.io')
  })

  it('links the contact Discord entry to the public invite', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    const discordLink = screen.getAllByRole('link', { name: /Discord/i }).at(-1)!
    expect(discordLink).toHaveAttribute('href', 'https://discord.gg/ezAHMkdG')
    expect(discordLink).toHaveAttribute('target', '_blank')
    expect(discordLink).toHaveAttribute('rel', 'noreferrer')
  })

  it('links the contact X entry to the official account', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    const xLink = screen.getAllByRole('link', { name: /^X$/i }).at(-1)!
    expect(xLink).toHaveAttribute('href', 'https://x.com/officecli')
    expect(xLink).toHaveAttribute('target', '_blank')
    expect(xLink).toHaveAttribute('rel', 'noreferrer')
  })

  it('links the contact GitHub entry to the open-source repo', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    const githubLink = screen.getAllByRole('link', { name: /^GitHub$/i }).at(-1)!
    expect(githubLink).toHaveAttribute('href', 'https://github.com/officecli/officecli')
    expect(githubLink).toHaveAttribute('target', '_blank')
    expect(githubLink).toHaveAttribute('rel', 'noreferrer')
  })

  it('links Discord and X from the home hero', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getByRole('link', { name: /Join Discord/i })).toHaveAttribute('href', 'https://discord.gg/ezAHMkdG')
    expect(screen.getByRole('link', { name: /Follow on X/i })).toHaveAttribute('href', 'https://x.com/officecli')
    expect(screen.getByRole('link', { name: /Star on GitHub/i })).toHaveAttribute('href', 'https://github.com/officecli/officecli')
  })

  it('renders three surface tabs with officedex selected by default', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(3)
    const dexTab = screen.getByRole('tab', { name: /officedex/i })
    const tuiTab = screen.getByRole('tab', { name: /officecli-tui/i })
    const cliTab = screen.getByRole('tab', { name: /Scriptable CLI/i })
    expect(dexTab).toHaveAttribute('aria-selected', 'true')
    expect(tuiTab).toHaveAttribute('aria-selected', 'false')
    expect(cliTab).toHaveAttribute('aria-selected', 'false')
  })

  it('switches surface panels when a different tab is clicked', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    const tuiTab = screen.getByRole('tab', { name: /officecli-tui/i })
    fireEvent.click(tuiTab)
    expect(tuiTab).toHaveAttribute('aria-selected', 'true')

    const panels = screen.getAllByRole('tabpanel', { hidden: true })
    const tuiPanel = panels.find((panel) => panel.id.endsWith('-panel-officecli-tui'))
    const dexPanel = panels.find((panel) => panel.id.endsWith('-panel-officedex'))
    expect(tuiPanel).toBeDefined()
    expect(dexPanel).toBeDefined()
    expect(tuiPanel).toHaveAttribute('aria-hidden', 'false')
    expect(dexPanel).toHaveAttribute('aria-hidden', 'true')
  })

  it('marks the officedex CTA as coming soon and shares the install link across tui and cli', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    const notifyButton = screen.getByRole('button', { name: /Coming soon/ })
    expect(notifyButton).toHaveAttribute('aria-disabled', 'true')

    const installLinks = screen.getAllByRole('link', { name: /^Install officecli$/i, hidden: true })
    expect(installLinks.length).toBeGreaterThanOrEqual(2)
    const hrefs = installLinks.map((link) => link.getAttribute('href'))
    expect(new Set(hrefs).size).toBe(1)
    expect(hrefs[0]).toBe('/download')
  })

  it('shows the five output formats in the independent band', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    const band = screen.getByRole('region', { name: /Supported output formats/i })
    for (const ext of ['PPTX', 'DOCX', 'XLSX', 'REPORT', 'IMG']) {
      expect(within(band).getAllByText(new RegExp(`^${ext}$`)).length).toBeGreaterThan(0)
    }
  })

  it('shows a non-price placeholder when the pricing api fails', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
      if (String(input) === '/api/events/track') {
        return { ok: true, json: async () => ({ data: { success: true } }) } as Response
      }
      throw new Error('network error')
    })

    render(
      <MemoryRouter initialEntries={['/pricing']}>
        <App />
      </MemoryRouter>,
    )

    expect((await screen.findAllByText(/Live pricing is currently unavailable/i)).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('link', { name: /Open Billing Workspace/i })[0].getAttribute('href')).toContain('https://platform.officecli.io/app/billing')
    expect(screen.queryByText('External 100')).not.toBeInTheDocument()
  })

  it('renders hosted pricing with credits as the primary unit and USD as auxiliary copy', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
      if (String(input) === '/api/events/track') {
        return { ok: true, json: async () => ({ data: { success: true } }) } as Response
      }
      return {
        ok: true,
        json: async () => ({
          data: [
            { code: 'hosted-300', name: 'Hosted 300', description: '300 hosted credits', currency: 'usd', amount_total: 300, quota_amount: 0, credit_amount: 300, pack_kind: 'hosted_credits' },
            { code: 'hosted-1000', name: 'Hosted 1000', description: '1000 hosted credits', currency: 'usd', amount_total: 1000, quota_amount: 0, credit_amount: 1000, pack_kind: 'hosted_credits' },
          ],
        }),
      } as Response
    })

    render(
      <MemoryRouter initialEntries={['/pricing']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByText(/300 credits/i)).toBeInTheDocument()
    expect(screen.getByText(/≈ \$3\.00 USD/i)).toBeInTheDocument()
    expect(screen.getByText(/1000 credits/i)).toBeInTheDocument()
    expect(screen.getByText(/≈ \$10\.00 USD/i)).toBeInTheDocument()
  })

  it('detects supported operating systems for install tabs', () => {
    expect(detectOperatingSystem('Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4)')).toBe('macos')
    expect(detectOperatingSystem('Mozilla/5.0 (X11; Linux x86_64)')).toBe('linux')
    expect(detectOperatingSystem('Mozilla/5.0 (Windows NT 10.0; Win64; x64)')).toBe('manual')
  })

  it('updates document head metadata for docs routes', () => {
    render(
      <MemoryRouter initialEntries={['/docs']}>
        <App />
      </MemoryRouter>,
    )

    expect(document.title).toContain('OfficeCLI Docs')
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe('https://officecli.io/docs')
  })

  it('renders the agent skills landing page with route-specific metadata', () => {
    render(
      <MemoryRouter initialEntries={['/officecli']}>
        <App />
      </MemoryRouter>,
    )

    expect(
      screen.getByRole('heading', { name: /officecli for Claude Code, Codex, and AI Agents/i, level: 1 }),
    ).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Generation demos/i, level: 2 })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Image-rich strategy deck/i, level: 3 })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /OfficeCLI deadline automation image/i, level: 3 })).toBeInTheDocument()
    expect(screen.getAllByText(/Download PPTX/i).length).toBeGreaterThan(0)
    expect(document.title).toContain('officecli')
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe('https://officecli.io/officecli')
  })

  it('uses the new canonical skills hub for the legacy agent skills path', () => {
    render(
      <MemoryRouter initialEntries={['/claude-code-codex-office-skills']}>
        <App />
      </MemoryRouter>,
    )

    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe('https://officecli.io/officecli')
  })

  it('redirects old officecli-skills child paths to the new officecli path', () => {
    render(
      <MemoryRouter initialEntries={['/officecli-skills/codex']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getByRole('heading', { name: /Direct local skill install without a marketplace layer/i, level: 1 })).toBeInTheDocument()
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe(
      'https://officecli.io/officecli/codex',
    )
  })

  it('renders the install child page with route-specific metadata', () => {
    render(
      <MemoryRouter initialEntries={['/officecli/install']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getByRole('heading', { name: /Choose the install path that matches the agent runtime/i, level: 1 })).toBeInTheDocument()
    expect(document.title).toContain('Install officecli')
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe(
      'https://officecli.io/officecli/install',
    )
  })

  it('opens the product docs navbar item in a new browser tab', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    const docsLink = screen.getAllByRole('link', { name: 'Product Docs (opens in a new tab)' })[0]
    expect(docsLink).toHaveAttribute('href', '/docs')
    expect(docsLink).toHaveAttribute('target', '_blank')
    expect(docsLink).toHaveAttribute('rel', 'noreferrer')
  })

  it('resets scroll position when navigating from footer links', () => {
    const scrollTo = vi.fn()
    Object.defineProperty(window, 'scrollTo', {
      configurable: true,
      writable: true,
      value: scrollTo,
    })

    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    scrollTo.mockClear()
    fireEvent.click(screen.getAllByRole('link', { name: 'Install the CLI' }).at(-1)!)

    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' })
  })

  it('renders the docs page sections, prompt examples, and pricing fallback', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(new Error('network error'))

    render(
      <MemoryRouter initialEntries={['/docs']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getAllByText('Install / Update / Uninstall').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Command Reference').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Prompting Tips').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Prompt Cookbook').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Use With Agents').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Use With OpenClaw').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Pricing & Usage Rules').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Invite Rewards').length).toBeGreaterThan(0)
    expect(screen.getByText(/Each activated referral adds 100 hosted credits/i)).toBeInTheDocument()
    expect(screen.getAllByText(/Hosted trial access is the default/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/officecli new pptx "Q3 Business Review"/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/--prompt-file/i).length).toBeGreaterThan(0)
    expect((await screen.findAllByText(/Live pricing is currently unavailable/i)).length).toBeGreaterThan(0)
  })

  it('shows first-run hosted examples on the download page', () => {
    render(
      <MemoryRouter initialEntries={['/download']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getByText(/hosted trial access by default/i)).toBeInTheDocument()
    expect(screen.getAllByText(/Run after install/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/officecli new pptx "Q3 Business Review"/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/officecli whoami/i).length).toBeGreaterThan(0)
  })

  it('renders SEO landing pages as first-class routes', () => {
    render(
      <MemoryRouter initialEntries={['/ai-pptx-generator']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getByRole('heading', { name: /AI PPTX Generator for Agent Workflows/i, level: 1 })).toBeInTheDocument()
    expect(screen.getByText(/officecli new pptx "Q3 Business Review"/i)).toBeInTheDocument()
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe(
      'https://officecli.io/ai-pptx-generator',
    )
  })
})

describe('site metadata and assets', () => {
  it('defines marketing metadata in index.html', () => {
    const html = fs.readFileSync(path.resolve(__dirname, '..', 'index.html'), 'utf8')

    expect(html).toContain('AI PPTX, DOCX, XLSX, REPORT, and IMG Generator')
    expect(html).toContain('name="description"')
    expect(html).toContain('rel="canonical" href="https://officecli.io/"')
    expect(html).toContain('name="robots" content="index,follow"')
    expect(html).toContain('property="og:url" content="https://officecli.io/"')
    expect(html).toContain('name="twitter:card"')
    expect(html).toContain('application/ld+json')
  })

  it('loads the Stitch baseline web fonts for the marketing site', () => {
    const entry = fs.readFileSync(path.resolve(__dirname, 'main.tsx'), 'utf8')
    const css = fs.readFileSync(path.resolve(__dirname, 'index.css'), 'utf8')

    expect(entry).toContain("@fontsource/space-grotesk/latin-700.css")
    expect(entry).toContain("@fontsource/inter/latin-700.css")
    expect(entry).toContain("@fontsource/jetbrains-mono/latin-500.css")
    expect(css).toContain('--font-headline: "Space Grotesk", sans-serif;')
    expect(css).toContain('--font-sans: "Inter", sans-serif;')
    expect(css).toContain('--font-mono: "JetBrains Mono", monospace;')
  })

  it('does not rely on picsum placeholder imagery', () => {
    const heroSource = fs.readFileSync(path.resolve(__dirname, 'components/Hero.tsx'), 'utf8')
    const useCasesSource = fs.readFileSync(path.resolve(__dirname, 'components/UseCases.tsx'), 'utf8')

    expect(heroSource).not.toContain('picsum.photos')
    expect(useCasesSource).not.toContain('picsum.photos')
  })

  it('keeps the home hero compact at default browser zoom', () => {
    const heroSource = fs.readFileSync(path.resolve(__dirname, 'components/Hero.tsx'), 'utf8')

    expect(heroSource).not.toContain('min-h-[90vh]')
    expect(heroSource).not.toContain('md:text-8xl')
    expect(heroSource).toContain('<HeroProductTabs />')
    expect(heroSource).toContain('<HeroOutputFormats />')
  })

  it('defines crawl assets for the marketing site', () => {
    const robots = fs.readFileSync(path.resolve(__dirname, '..', 'public', 'robots.txt'), 'utf8')
    const sitemap = fs.readFileSync(path.resolve(__dirname, '..', 'public', 'sitemap.xml'), 'utf8')

    expect(robots).toContain('Sitemap: https://officecli.io/sitemap.xml')
    expect(sitemap).toContain('<loc>https://officecli.io/</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/download</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/docs</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/pricing</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/faq</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/officecli</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/officecli/install</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/officecli/codex</loc>')
    expect(sitemap).not.toContain('<loc>https://officecli.io/claude-code-codex-office-skills</loc>')
    for (const seoPath of seoLandingPaths) {
      expect(sitemap).toContain(`<loc>https://officecli.io${seoPath}</loc>`)
    }

    const locs = [...sitemap.matchAll(/<loc>https:\/\/officecli\.io([^<]*)<\/loc>/g)].map((match) => match[1] || '/')
    for (const loc of locs) {
      expect(routeSEO[loc]?.canonical).toBe(`https://officecli.io${loc === '/' ? '/' : loc}`)
    }
  })

  it('renders prerendered home app html with main content', () => {
    const html = renderRouteApp('/')

    expect(html).toContain('<main')
    expect(html).toContain('One binary · Three surfaces · Plus a desktop app')
    expect(html).toContain('AI document generation, on every surface')
    expect(html).toContain('Five outputs, supported by every surface')
    expect(html).toContain('id="faq"')
  })

  it('renders prerendered canonical product pages with route-specific content', () => {
    const cases = [
      ['/download', 'Install OfficeCLI for AI document generation'],
      ['/docs', 'OfficeCLI documentation for AI document generation'],
      ['/pricing', 'OfficeCLI Pricing'],
      ['/faq', 'OfficeCLI FAQ'],
    ]

    for (const [route, heading] of cases) {
      const html = renderRouteApp(route)

      expect(html).toContain(`<h1 class="font-headline text-5xl md:text-6xl font-bold text-white tracking-tight mb-6">${heading}</h1>`)
      expect(html).not.toBe('<main class="overflow-x-hidden"></main>')
    }
  })

  it('renders prerendered SEO landing pages with long-form search copy', () => {
    const html = renderRouteApp('/ai-pptx-generator')

    expect(html).toContain('AI PPTX Generator for Agent Workflows')
    expect(html).toContain('officecli new pptx &quot;Q3 Business Review&quot;')
    expect(html).toContain('External Mode is free and unlimited')
    expect(html).toContain('Hosted Mode uses hosted credits')
  })

  it('renders prerendered agent skills html with search-focused copy', () => {
    const html = renderRouteApp('/officecli')

    expect(html).toContain('officecli for Claude Code, Codex, and AI Agents')
    expect(html).toContain('Primary entrypoints')
    expect(html).toContain('Generation demos')
    expect(html).toContain('Image-rich strategy deck')
    expect(html).toContain('OfficeCLI deadline automation image')
  })

  it('renders prerendered child skills pages with unique headings', () => {
    const html = renderRouteApp('/officecli/openclaw')

    expect(html).toContain('officecli for OpenClaw')
    expect(html).toContain('structured, channel-based Office document generation')
  })
})
