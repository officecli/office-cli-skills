import { useState, useEffect } from 'react'
import { motion } from 'motion/react'
import { CheckCircle2, Copy, CheckCheck } from 'lucide-react'

function HighlightedCommand({ command }: { command: string }) {
  const parts = command.split(' ')
  return (
    <span className="break-all">
      {parts.map((part, i) => {
        if (part === 'officecli' || part === 'brew' || part === 'npm') return <span key={i} className="text-[#27c93f] mr-1.5">{part}</span>
        if (part === 'new' || part === 'config' || part === 'install' || part === 'set-generation') return <span key={i} className="text-[#8af3f7] mr-1.5">{part}</span>
        if (part === 'pptx' || part === 'docx' || part === 'xlsx' || part === 'report' || part === 'img' || part === '-g') return <span key={i} className="text-[#ffbd2e] mr-1.5">{part}</span>
        if (part.startsWith('"') || part.startsWith("'")) return <span key={i} className="text-[#ff5f56] mr-1.5">{part}</span>
        if (part.startsWith('--')) return <span key={i} className="text-outline-variant mr-1.5">{part}</span>
        return <span key={i} className="text-white mr-1.5">{part}</span>
      })}
    </span>
  )
}

function CopyableCommand({ command, prefix = '$' }: { command: string, prefix?: string }) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 1500)
    return () => window.clearTimeout(timer)
  }, [copied])

  async function handleCopy() {
    try {
      await navigator.clipboard?.writeText(command)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="flex items-center justify-between gap-4 group/cmd">
      <div className="flex gap-4">
        <span className="text-primary">{prefix}</span>
        <HighlightedCommand command={command} />
      </div>
      <button
        type="button"
        onClick={handleCopy}
        className="opacity-0 group-hover/cmd:opacity-100 transition-opacity p-1.5 rounded bg-surface-high border border-outline-variant/20 text-outline-variant hover:text-white hover:border-outline-variant/40"
        aria-label="Copy command"
      >
        {copied ? <CheckCheck size={14} className="text-tertiary" /> : <Copy size={14} />}
      </button>
    </div>
  )
}

export default function CLIShowcase() {
  return (
    <section id="formats" className="py-24 px-8 md:px-16 max-w-[1440px] mx-auto">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-16">
        <motion.div
          initial={{ opacity: 0, x: -20 }}
          whileInView={{ opacity: 1, x: 0 }}
          viewport={{ once: true }}
        >
          <h2 className="font-headline text-4xl font-bold text-white mb-6">Generate, Review, and Publish From One CLI</h2>
          <p className="text-outline-variant text-lg mb-8 leading-relaxed">Stay in the terminal and run AI document automation from one local-first CLI. Install once, connect your LLM, generate PPTX, DOCX, XLSX, REPORT, and standalone IMG outputs, then publish a password-protected online preview link in the same command — no extra hosting, gateway, or upload step.</p>
          <ul className="space-y-6">
            <li className="flex items-start gap-4">
              <CheckCircle2 className="text-tertiary w-6 h-6 mt-1" />
              <div>
                <h5 className="font-bold text-white">Built-in online publish</h5>
                <p className="text-sm text-outline-variant">Every generation can return a shareable, password-protected preview URL — a differentiator other AI document CLIs do not ship out of the box. Use `--no-publish` for fully local output.</p>
              </div>
            </li>
            <li className="flex items-start gap-4">
              <CheckCircle2 className="text-tertiary w-6 h-6 mt-1" />
              <div>
                <h5 className="font-bold text-white">Lightweight by design</h5>
                <p className="text-sm text-outline-variant">Core local usage does not require a cluster, backend service, Docker, or Kubernetes.</p>
              </div>
            </li>
            <li className="flex items-start gap-4">
              <CheckCircle2 className="text-tertiary w-6 h-6 mt-1" />
              <div>
                <h5 className="font-bold text-white">Current release surface is explicit</h5>
                <p className="text-sm text-outline-variant">Use `new`, `score`, and `review` today for presentations, documents, spreadsheets, REPORT outputs, and standalone IMG visuals while broader conversion and editing workflows land next.</p>
              </div>
            </li>
            <li className="flex items-start gap-4">
              <CheckCircle2 className="text-tertiary w-6 h-6 mt-1" />
              <div>
                <h5 className="font-bold text-white">Standalone images are first-class</h5>
                <p className="text-sm text-outline-variant">`new img` ships with `--ratio`, `--size`, and repeatable `--reference-image` flags, publishes online previews by default, and uses a separate free image bucket.</p>
              </div>
            </li>
          </ul>
        </motion.div>

        <motion.div 
          initial={{ opacity: 0, x: 20 }}
          whileInView={{ opacity: 1, x: 0 }}
          viewport={{ once: true }}
          className="bg-surface-low rounded-xl border border-outline-variant/20 overflow-hidden font-mono text-sm"
        >
          <div className="bg-surface-high px-4 py-2 border-b border-outline-variant/10 flex justify-between items-center">
            <span className="text-[10px] text-outline font-headline uppercase tracking-widest">Bash - officecli v0.2.5</span>
            <div className="flex gap-2">
              <div className="w-2 h-2 rounded-full bg-outline-variant/30"></div>
              <div className="w-2 h-2 rounded-full bg-outline-variant/30"></div>
            </div>
          </div>
          <div className="p-8 space-y-4">
            <div className="flex gap-4">
              <span className="text-tertiary italic"># Install via Homebrew or npm</span>
            </div>
            <CopyableCommand command="brew tap officecli/officecli && brew install officecli" />
            <CopyableCommand command="npm install -g officecli" />
            <div className="pt-4 flex gap-4">
              <span className="text-tertiary italic"># Configure the runtime and run a document operation</span>
            </div>
            <CopyableCommand command="officecli config set-generation" />
            <CopyableCommand command='officecli new pptx "Q3 Business Review" --prompt-file ./brief.md' />
            <motion.div
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.3 }}
              className="text-white mt-4"
            >
              Generation completed. Saved to output/Q3_BUSINESS_REVIEW.PPTX
            </motion.div>
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.7 }}
              className="text-white mt-2"
            >
              Preview URL: <a href="https://officecli.io/p/xyz123" target="_blank" rel="noreferrer" className="text-primary hover:underline">https://officecli.io/p/xyz123</a>; password: abcdef
            </motion.div>
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 1.1 }}
              className="text-outline-variant"
            >
              Access: External Mode; free unlimited with your model endpoint
            </motion.div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
