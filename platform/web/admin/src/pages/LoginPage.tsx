import { ShieldCheck, Radar, Siren, Workflow } from 'lucide-react'
import { api } from '../api'
import { OfficeCliBrand } from '../components/branding'

const valuePoints = [
  { icon: Radar, title: 'Platform health at a glance', body: 'Track key inventory, blocked traffic, and free-machine pressure from one operator surface.' },
  { icon: Workflow, title: 'Audit-first workflow review', body: 'Inspect recent usage events before quota abuse expands into production incidents.' },
  { icon: Siren, title: 'Restricted governance actions', body: 'Only allowlisted company accounts can enter the admin plane and change controls.' },
]

export default function LoginPage() {
  return (
    <div className="min-h-screen bg-background px-6 py-8 text-on-surface">
      <div className="login-shell mx-auto max-w-7xl">
        <section className="panel relative overflow-hidden p-8 md:p-12">
          <div className="login-hero-admin-glow absolute inset-0" />
          <div className="relative">
            <OfficeCliBrand
              className="mb-8"
              markClassName="h-12 w-12"
              titleClassName="text-xl font-bold text-white"
              subtitle="admin plane / allowlist required"
            />
            <span className="chip">Company OAuth2 / admin</span>
            <h1 className="mt-6 max-w-xl text-5xl font-bold leading-[0.92] text-white md:text-6xl">Authorized company accounts only</h1>
            <p className="mt-6 max-w-2xl text-base leading-7 text-outline">Enter the OfficeCLI admin plane to manage API key governance, tune free quotas, investigate blocked traffic, and keep platform controls under explicit operator access. The current production allowlist is intentionally narrowed to a single operator account: <span className="text-white">luyang950@gmail.com</span>.</p>

            <div className="mt-10 flex flex-wrap gap-3">
              <button type="button" className="tonal-button" onClick={() => api.login('/admin')}>
                <ShieldCheck size={16} />
                Continue with company OAuth2
              </button>
              <a href="https://platform.officecli.io/app/" className="ghost-button">Go to user app</a>
            </div>

            <div className="mt-12 grid gap-4 md:grid-cols-3">
              {valuePoints.map((item) => (
                <div key={item.title} className="panel-muted p-5">
                  <item.icon size={18} className="text-primary" />
                  <div className="mt-4 text-lg font-semibold text-white">{item.title}</div>
                  <div className="mt-2 text-sm text-outline">{item.body}</div>
                </div>
              ))}
            </div>
          </div>
        </section>

        <aside className="panel flex flex-col justify-between p-8 md:p-10">
          <div>
            <OfficeCliBrand
              markClassName="h-11 w-11"
              title="OfficeCLI admin"
              titleClassName="text-xl font-bold text-white"
              subtitle="governance console / allowlist required"
            />

            <div className="mt-10 panel-muted p-6">
              <div className="info-eyebrow text-tertiary">Access contract</div>
              <div className="mt-4 space-y-5">
                <div>
                  <div className="text-sm font-semibold text-white">Authentication source</div>
                  <div className="mt-1 text-sm text-outline">Use the company OAuth2 identity source for all admin sessions.</div>
                </div>
                <div>
                  <div className="text-sm font-semibold text-white">Authorization rule</div>
                  <div className="mt-1 text-sm text-outline">A signed-in account still needs an exact email match in the admin allowlist before a session is issued. At the moment, only <span className="text-white">luyang950@gmail.com</span> is expected to pass.</div>
                </div>
                <div>
                  <div className="text-sm font-semibold text-white">Rejected accounts</div>
                  <div className="mt-1 text-sm text-outline">Accounts that pass OAuth2 auth but miss the allowlist are redirected to a dedicated access denied view.</div>
                </div>
              </div>
            </div>
          </div>

          <div className="terminal-card mt-8 p-6 font-mono text-xs text-outline">
            <div className="info-eyebrow mb-3 flex items-center gap-2 text-primary">
              <Workflow size={14} />
              admin gate
            </div>
            <div>$ oauth2 redirect --target admin</div>
            <div className="mt-2 text-secondary">identity: waiting_for_oauth2_sign_in</div>
            <div className="mt-1">policy: allowlist_enforced</div>
          </div>
        </aside>
      </div>
    </div>
  )
}
