import { useState } from 'react'
import { Check, Copy } from 'lucide-react'

interface CodeBlockProps {
  command: string
  label?: string
}

export default function CodeBlock({ command, label }: CodeBlockProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(command)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy text: ', err)
    }
  }

  return (
    <div className="rounded-2xl border border-outline-variant/10 bg-surface-low overflow-hidden group">
      {label && (
        <div className="flex items-center justify-between px-5 py-3 border-b border-outline-variant/10 bg-surface">
          <div className="text-xs font-headline uppercase tracking-widest text-tertiary">{label}</div>
        </div>
      )}
      <div className="relative flex items-start">
        <div className="flex-1 overflow-x-auto px-5 py-4">
          <code className="block text-sm text-white whitespace-pre-wrap font-mono">
            {command}
          </code>
        </div>
        <button
          onClick={handleCopy}
          className="absolute top-3 right-3 p-2 rounded-lg bg-surface/50 text-outline-variant opacity-0 group-hover:opacity-100 hover:bg-surface-high hover:text-white transition-all focus:opacity-100"
          aria-label="Copy code"
          title="Copy code"
        >
          {copied ? <Check className="w-4 h-4 text-primary" /> : <Copy className="w-4 h-4" />}
        </button>
      </div>
    </div>
  )
}
