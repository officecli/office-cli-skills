import fs from 'node:fs'
import path from 'node:path'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import App from './App'

describe('marketing site shell', () => {
  it('renders the OfficeCLI brand and home hero copy', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getAllByText('OfficeCLI').length).toBeGreaterThan(0)
    expect(
      screen.getByRole('heading', { name: /Plug Document Production Into Your Workflows/i, level: 1 }),
    ).toBeInTheDocument()
  })

  it('keeps pricing available when the pricing api fails', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(new Error('network error'))

    render(
      <MemoryRouter initialEntries={['/pricing']}>
        <App />
      </MemoryRouter>,
    )

    expect((await screen.findAllByText('Starter Shell')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('Production Pack').length).toBeGreaterThan(0)
  })
})

describe('site metadata and assets', () => {
  it('defines marketing metadata in index.html', () => {
    const html = fs.readFileSync(path.resolve(__dirname, '..', 'index.html'), 'utf8')

    expect(html).toContain('name="description"')
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
