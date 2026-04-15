import { useEffect, useState } from 'react'
import { Copy, CheckCheck } from 'lucide-react'
import { detectOperatingSystem, installTabs, type InstallTabID } from '../installData'

interface InstallTabsProps {
  compact?: boolean
  headline?: string
  intro?: string
}

export default function InstallTabs({
  compact = false,
  headline = 'Install OfficeCLI',
  intro = 'Pick the setup path that matches your machine. OfficeCLI stays lightweight: one binary, plus your LLM endpoint for the core local workflow.',
}: InstallTabsProps) {
  const [activeTab, setActiveTab] = useState<InstallTabID>('manual')
  const [copiedValue, setCopiedValue] = useState<string>('')

  useEffect(() => {
    setActiveTab(detectOperatingSystem())
  }, [])

  useEffect(() => {
    if (!copiedValue) return
    const timer = window.setTimeout(() => setCopiedValue(''), 1500)
    return () => window.clearTimeout(timer)
  }, [copiedValue])

  const current = installTabs.find((tab) => tab.id === activeTab) ?? installTabs[0]

  async function copyCommand(value: string) {
    try {
      await navigator.clipboard?.writeText(value)
      setCopiedValue(value)
    } catch {
      setCopiedValue('')
    }
  }

  return (
    <section id="download" className={compact ? '' : 'py-24 px-8 md:px-16 max-w-[1440px] mx-auto'}>
      <div className={compact ? '' : 'max-w-6xl'}>
        <div className={compact ? 'mb-8' : 'mb-12'}>
          <span className="text-primary font-headline text-xs uppercase tracking-widest mb-4 block">Quick install</span>
          <h2 className={`font-headline font-bold text-white tracking-tight ${compact ? 'text-3xl md:text-4xl mb-4' : 'text-4xl md:text-5xl mb-5'}`}>{headline}</h2>
          <p className="text-outline-variant text-lg leading-relaxed max-w-3xl">{intro}</p>
        </div>

        <div className="grid grid-cols-1 xl:grid-cols-[240px_minmax(0,1fr)] gap-6">
          <div className="bg-surface-low border border-outline-variant/10 rounded-2xl p-3 flex xl:flex-col gap-3">
            {installTabs.map((tab) => {
              const active = tab.id === current.id
              return (
                <button
                  key={tab.id}
                  type="button"
                  className={`rounded-xl text-left px-4 py-4 transition-all border ${active ? 'bg-surface-high border-primary/30 text-white' : 'bg-transparent border-transparent text-outline-variant hover:text-white hover:border-outline-variant/20'}`}
                  onClick={() => setActiveTab(tab.id)}
                >
                  <div className="font-headline text-xs uppercase tracking-widest mb-2">{tab.eyebrow}</div>
                  <div className="font-bold text-lg">{tab.label}</div>
                </button>
              )
            })}
          </div>

          <div className="bg-surface-low border border-outline-variant/10 rounded-3xl p-8 md:p-10">
            <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-6 mb-8">
              <div>
                <div className="text-xs font-headline uppercase tracking-widest text-tertiary mb-3">{current.eyebrow}</div>
                <h3 className="font-headline text-3xl font-bold text-white mb-4">{current.title}</h3>
                <p className="text-outline-variant leading-relaxed max-w-2xl">{current.description}</p>
              </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
              {current.commands.map((item) => {
                const copied = copiedValue === item.command
                const isLink = item.command.startsWith('https://')
                return (
                  <article key={item.label} className="bg-background border border-outline-variant/10 rounded-2xl p-5">
                    <div className="text-xs font-headline uppercase tracking-widest text-primary mb-3">{item.label}</div>
                    {isLink ? (
                      <a className="block font-mono text-sm text-white break-all hover:text-primary" href={item.command} target="_blank" rel="noreferrer">{item.command}</a>
                    ) : (
                      <code className="block font-mono text-sm text-white break-all">{item.command}</code>
                    )}
                    <p className="mt-4 text-sm text-outline-variant leading-relaxed">{item.detail}</p>
                    <button
                      type="button"
                      className="mt-5 inline-flex items-center gap-2 border border-outline-variant/20 rounded-full px-4 py-2 text-sm font-semibold text-white hover:border-primary/30 hover:text-primary"
                      onClick={() => copyCommand(item.command)}
                    >
                      {copied ? <CheckCheck size={16} /> : <Copy size={16} />}
                      {copied ? 'Copied' : 'Copy'}
                    </button>
                  </article>
                )
              })}
            </div>

            <ul className="mt-8 grid grid-cols-1 md:grid-cols-2 gap-3 text-sm text-outline-variant">
              {current.notes.map((note) => (
                <li key={note} className="bg-background/70 border border-outline-variant/10 rounded-xl px-4 py-3">{note}</li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}
