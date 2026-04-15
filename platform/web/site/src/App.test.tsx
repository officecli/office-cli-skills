import fs from 'node:fs'
import path from 'node:path'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import App from './App'
import { detectOperatingSystem } from './installData'

describe('marketing site shell', () => {
  it('renders the OfficeCLI brand and home hero copy', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getAllByText('OfficeCLI').length).toBeGreaterThan(0)
    expect(
      screen.getByRole('heading', { name: /Run Document Operations From One Lightweight Binary/i, level: 1 }),
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

  it('detects supported operating systems for install tabs', () => {
    expect(detectOperatingSystem('Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4)')).toBe('macos')
    expect(detectOperatingSystem('Mozilla/5.0 (X11; Linux x86_64)')).toBe('linux')
    expect(detectOperatingSystem('Mozilla/5.0 (Windows NT 10.0; Win64; x64)')).toBe('manual')
  })
})

describe('site metadata and assets', () => {
  it('defines marketing metadata in index.html', () => {
    const html = fs.readFileSync(path.resolve(__dirname, '..', 'index.html'), 'utf8')

    expect(html).toContain('<title>OfficeCLI</title>')
    expect(html).toContain('name="description"')
    expect(html).toContain('property="og:title" content="OfficeCLI"')
    expect(html).toContain('name="twitter:title" content="OfficeCLI"')
    expect(html).toContain('property="og:title"')
    expect(html).toContain('name="twitter:card"')
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
})
