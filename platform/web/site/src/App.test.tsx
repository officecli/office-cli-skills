import fs from 'node:fs'
import path from 'node:path'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { detectOperatingSystem } from './installData'
import { renderRouteApp } from './prerender'

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
      screen.getByRole('heading', { name: /Generate PPTX.*DOCX, XLSX, REPORT, and IMG Outputs From One AI CLI/i, level: 1 }),
    ).toBeInTheDocument()
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

  it('shows a non-price placeholder when the pricing api fails', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(new Error('network error'))

    render(
      <MemoryRouter initialEntries={['/pricing']}>
        <App />
      </MemoryRouter>,
    )

    expect((await screen.findAllByText(/Live pricing is currently unavailable/i)).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('link', { name: /Open Billing Workspace/i })[0]).toHaveAttribute('href', 'https://platform.officecli.io/app/billing')
    expect(screen.queryByText('External 100')).not.toBeInTheDocument()
  })

  it('renders hosted pricing with credits as the primary unit and USD as auxiliary copy', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        data: [
          { code: 'hosted-300', name: 'Hosted 300', description: '300 hosted credits', currency: 'usd', amount_total: 300, quota_amount: 0, credit_amount: 300, pack_kind: 'hosted_credits' },
          { code: 'hosted-1000', name: 'Hosted 1000', description: '1000 hosted credits', currency: 'usd', amount_total: 1000, quota_amount: 0, credit_amount: 1000, pack_kind: 'hosted_credits' },
        ],
      }),
    } as Response)

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
      <MemoryRouter initialEntries={['/officecli-skills']}>
        <App />
      </MemoryRouter>,
    )

    expect(
      screen.getByRole('heading', { name: /officecli-skills for Claude Code, Codex, and AI Agents/i, level: 1 }),
    ).toBeInTheDocument()
    expect(document.title).toContain('officecli-skills')
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe('https://officecli.io/officecli-skills')
  })

  it('uses the new canonical skills hub for the legacy agent skills path', () => {
    render(
      <MemoryRouter initialEntries={['/claude-code-codex-office-skills']}>
        <App />
      </MemoryRouter>,
    )

    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe('https://officecli.io/officecli-skills')
  })

  it('renders the install child page with route-specific metadata', () => {
    render(
      <MemoryRouter initialEntries={['/officecli-skills/install']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getByRole('heading', { name: /Choose the install path that matches the agent runtime/i, level: 1 })).toBeInTheDocument()
    expect(document.title).toContain('Install officecli-skills')
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe(
      'https://officecli.io/officecli-skills/install',
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
    expect(screen.getByText(/Each activated referral adds 10 bonus generations/i)).toBeInTheDocument()
    expect(screen.getAllByText(/--prompt-file/i).length).toBeGreaterThan(0)
    expect((await screen.findAllByText(/Live pricing is currently unavailable/i)).length).toBeGreaterThan(0)
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

  it('defines crawl assets for the marketing site', () => {
    const robots = fs.readFileSync(path.resolve(__dirname, '..', 'public', 'robots.txt'), 'utf8')
    const sitemap = fs.readFileSync(path.resolve(__dirname, '..', 'public', 'sitemap.xml'), 'utf8')

    expect(robots).toContain('Sitemap: https://officecli.io/sitemap.xml')
    expect(sitemap).toContain('<loc>https://officecli.io/docs</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/officecli-skills</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/officecli-skills/install</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/officecli-skills/codex</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/claude-code-codex-office-skills</loc>')
    expect(sitemap).toContain('<loc>https://officecli.io/download</loc>')
  })

  it('renders prerendered home app html with main content', () => {
    const html = renderRouteApp('/')

    expect(html).toContain('<main')
    expect(html).toContain('REPORT, and IMG Outputs From One AI CLI')
    expect(html).toContain('id="faq"')
  })

  it('renders prerendered agent skills html with search-focused copy', () => {
    const html = renderRouteApp('/officecli-skills')

    expect(html).toContain('officecli-skills for Claude Code, Codex, and AI Agents')
    expect(html).toContain('Primary entrypoints')
  })

  it('renders prerendered child skills pages with unique headings', () => {
    const html = renderRouteApp('/officecli-skills/openclaw')

    expect(html).toContain('officecli-skills for OpenClaw')
    expect(html).toContain('structured, channel-based Office document generation')
  })
})
