import { Copy, DownloadCloud, ShieldCheck } from 'lucide-react'
import { Panel, SectionHeading } from '../components/ui'

const commands = [
  { title: 'Install the CLI', command: 'curl -fsSL https://officecli.io/install.sh | sh' },
  { title: 'Check auth state', command: 'officecli auth status' },
  { title: 'Point the runtime at your key', command: 'officecli auth set-key cop_live_xxxxxxxx' },
]

export default function DownloadsPage() {
  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading eyebrow="Download / integrate" title="Bring OfficeCLI into your delivery workflow" body="Everything in this view is geared toward getting a team from install to authenticated generation as quickly as possible." />
        <div className="grid gap-4 lg:grid-cols-3">
          {commands.map((item) => (
            <div key={item.title} className="panel-muted p-5">
              <div className="flex items-center gap-2 text-primary">
                <DownloadCloud size={16} />
                <span className="info-eyebrow">{item.title}</span>
              </div>
              <code className="surface-console mt-4 block rounded-2xl p-4 font-mono text-sm text-white">{item.command}</code>
              <button type="button" className="ghost-button mt-4" onClick={() => navigator.clipboard?.writeText(item.command)}><Copy size={14} /> Copy command</button>
            </div>
          ))}
        </div>
      </Panel>

      <div className="grid gap-4 lg:grid-cols-2">
        <Panel>
          <SectionHeading eyebrow="Recommended sequence" title="Connect the runtime in four moves" />
          <ol className="space-y-3 text-sm text-outline">
            <li>1. Install the latest CLI runtime on your workstation or CI runner.</li>
            <li>2. Sign in with Google from the terminal once to bind the workspace session.</li>
            <li>3. Create or pick an API key from the app shell.</li>
            <li>4. Route purchased credits into that key, then wire it into your document jobs.</li>
          </ol>
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
