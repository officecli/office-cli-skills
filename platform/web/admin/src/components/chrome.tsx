import { Activity, CreditCard, Fingerprint, Gift, KeyRound, Layers3, LogOut, ReceiptText, ShieldCheck, Sparkles, TerminalSquare, Users, SlidersHorizontal } from 'lucide-react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { api } from '../api'
import { cn } from '../lib/utils'
import type { AdminIdentity } from '../types'

const navItems = [
  { to: '/', label: 'Overview', icon: ShieldCheck },
  { to: '/growth', label: 'Growth', icon: Gift },
  { to: '/hosted-pricing', label: 'Hosted Pricing', icon: SlidersHorizontal },
  { to: '/api-keys', label: 'API Keys', icon: KeyRound },
  { to: '/users', label: 'Users', icon: Users },
  { to: '/orders', label: 'Orders', icon: CreditCard },
  { to: '/billing-events', label: 'Billing Events', icon: ReceiptText },
  { to: '/quota-sources', label: 'Quota Sources', icon: Layers3 },
  { to: '/free-quotas', label: 'Free Trial Devices', icon: Fingerprint },
  { to: '/usage-events', label: 'Usage Events', icon: Activity },
]

export function AdminSidebar({ admin }: { admin: AdminIdentity }) {
  return (
    <aside className="sidebar-shell fixed inset-y-0 left-0 z-40 hidden w-72 flex-col border-r border-outline-variant/20 px-5 py-6 lg:flex">
      <Link to="/" className="mb-10 flex items-center gap-4">
        <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-primary/20 bg-primary/10 text-primary">
          <Sparkles size={22} />
        </div>
        <div>
          <div className="font-headline text-lg font-bold text-white">OfficeCLI</div>
          <div className="info-eyebrow text-outline">admin governance / restricted</div>
        </div>
      </Link>

      <div className="soft-panel mb-8 border border-outline-variant/20 bg-surface-container-low/70 p-4">
        <div className="info-eyebrow text-outline">Authorized operator</div>
        <div className="mt-3 text-sm font-semibold text-white">{admin.name || admin.email}</div>
        <div className="mt-1 break-all text-xs text-outline">{admin.email}</div>
      </div>

      <nav className="space-y-2">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) => cn(
              'flex items-center gap-3 rounded-2xl px-4 py-3 text-sm text-outline hover:bg-surface-container-high hover:text-white',
              isActive && 'active-nav-item bg-surface-container-high text-white',
            )}
          >
            <item.icon size={18} />
            <span className="font-medium">{item.label}</span>
          </NavLink>
        ))}
      </nav>

      <div className="soft-panel mt-auto border border-tertiary/20 bg-tertiary/10 p-4 text-sm text-outline">
        <div className="info-eyebrow flex items-center gap-2 text-tertiary">
          <TerminalSquare size={14} />
          access policy
        </div>
        <p className="mt-3">Google authentication is required first, then the exact account email must exist in the OfficeCLI admin allowlist. The default production expectation is a single operator: luyang950@gmail.com.</p>
      </div>
    </aside>
  )
}

export function AdminTopBar({ admin }: { admin: AdminIdentity }) {
  const navigate = useNavigate()
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => navigate('/login', { replace: true }),
  })

  return (
    <header className="topbar-shell sticky top-0 z-30 mb-8 flex items-center justify-between gap-4 border border-outline-variant/20 px-5 py-4 backdrop-blur-xl">
      <div>
        <div className="info-eyebrow-wide text-primary">Governance plane</div>
        <div className="mt-2 text-2xl font-bold text-white">Operator visibility for quota and abuse control</div>
      </div>
      <div className="flex items-center gap-3">
        <a href="https://officecli.io/docs" className="ghost-button text-xs">Docs</a>
        <a href="https://officecli.io/faq" className="ghost-button text-xs">Policy</a>
        <div className="hidden rounded-full border border-outline-variant/20 px-4 py-2 text-right sm:block">
          <div className="info-eyebrow-mid text-outline">Session</div>
          <div className="text-sm font-semibold text-white">{admin.auth_method || 'google'}</div>
        </div>
        <button type="button" className="ghost-button px-4 text-xs" onClick={() => logout.mutate()} disabled={logout.isPending}>
          <LogOut size={14} />
          Sign out
        </button>
      </div>
    </header>
  )
}
