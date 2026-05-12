import { Copy, DownloadCloud, ShieldCheck, TerminalSquare } from 'lucide-react'
import { Panel, SectionHeading } from '../components/ui'

const commands = [
  {
    title: 'Homebrew on macOS',
    command: 'brew tap officecli/officecli && brew install officecli',
    detail: 'Recommended for Apple Silicon and Intel Macs.',
  },
  {
    title: 'npm on macOS or Linux',
    command: 'npm install -g officecli',
    detail: 'Supports darwin/linux on x64 and arm64.',
  },
  {
    title: 'Stable shell install on Linux',
    command: 'curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-officecli.sh | bash',
    detail: 'Installs the latest stable public release directly.',
  },
]

export default function DownloadsPage() {
  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading eyebrow="Download / integrate" title="Install OfficeCLI for document operations" body="Use Homebrew, npm, or the official shell installer to bring the same lightweight CLI into local workflows, CI, or hosted-access setups." />
        <div className="grid gap-4 lg:grid-cols-3">
          {commands.map((item) => (
            <div key={item.title} className="panel-muted p-5">
              <div className="flex items-center gap-2 text-primary">
                <DownloadCloud size={16} />
                <span className="info-eyebrow">{item.title}</span>
              </div>
              <code className="surface-console mt-4 block rounded-2xl p-4 font-mono text-sm text-white">{item.command}</code>
              <p className="mt-3 text-sm text-outline">{item.detail}</p>
              <button type="button" className="ghost-button mt-4" onClick={() => navigator.clipboard?.writeText(item.command)}><Copy size={14} /> Copy command</button>
            </div>
          ))}
        </div>
      </Panel>

      <div className="grid gap-4 lg:grid-cols-2">
        <Panel>
          <SectionHeading eyebrow="Recommended sequence" title="Connect the runtime in four moves" />
          <ol className="space-y-3 text-sm text-outline">
            <li>1. Install the CLI on your workstation or CI runner.</li>
            <li>2. Run `officecli config set-generation` to point the CLI at your LLM endpoint.</li>
            <li>3. Run `officecli new ...` for generation, then `score` or `review` when needed.</li>
            <li>4. Use this app only when you need paid access, hosted runtime features, billing, or preview publishing.</li>
          </ol>
        </Panel>
        <Panel>
          <SectionHeading eyebrow="Current scope" title="What works now and what stays optional" />
          <div className="space-y-4">
            <div className="soft-panel flex items-start gap-3 border border-outline-variant/15 bg-surface-container-high/40 p-5 text-sm text-outline">
              <TerminalSquare size={18} className="mt-0.5 text-secondary" />
              <p>The current public release generates PPTX, DOCX, XLSX, and workbook-backed Report outputs, and can review or score local PPTX files.</p>
            </div>
            <div className="soft-panel flex items-start gap-3 border border-primary/15 bg-primary/10 p-5 text-sm text-outline">
              <ShieldCheck size={18} className="mt-0.5 text-primary" />
              <p>Platform access is optional. Use it for hosted API keys, hosted runtime workflows, billing, and online preview publishing, not for basic External Mode setup.</p>
            </div>
          </div>
        </Panel>
        <Panel>
          <SectionHeading eyebrow="Security note" title="Use production keys deliberately" />
          <div className="soft-panel flex items-start gap-3 border border-primary/15 bg-primary/10 p-5 text-sm text-outline">
            <ShieldCheck size={18} className="mt-0.5 text-primary" />
            <p>Keep live keys in a secret manager, rotate them from the API Keys view when a pipeline changes ownership, and avoid baking plaintext credentials into local scripts.</p>
          </div>
        </Panel>
      </div>
    </div>
  )
}
