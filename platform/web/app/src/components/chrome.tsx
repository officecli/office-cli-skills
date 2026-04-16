import { CreditCard, Download, KeyRound, LayoutDashboard, LogOut, Wallet, Workflow, Sparkles } from 'lucide-react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { cn } from '../lib/utils'
import type { User } from '../types'

const inviteRewardGuideHref = 'https://officecli.io/docs#invite-rewards'

const navItems = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-overview'], queryFn: api.overview })
    queryClient.prefetchQuery({ queryKey: ['app-growth'], queryFn: api.growth })
    queryClient.prefetchQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
  }},
  { to: '/quota', label: 'Quota', icon: Wallet, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-quota-summary'], queryFn: api.quotaSummary })
  }},
  { to: '/api-keys', label: 'API Keys', icon: KeyRound, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
  }},
  { to: '/billing', label: 'Billing', icon: CreditCard, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['pricing'], queryFn: api.pricing })
    queryClient.prefetchQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
    queryClient.prefetchQuery({ queryKey: ['app-orders'], queryFn: api.orders })
  }},
  { to: '/usage', label: 'Usage', icon: Workflow, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-usage'], queryFn: api.usage })
  }},
  { to: '/downloads', label: 'Downloads', icon: Download },
]

export function AppSidebar({ user }: { user: User }) {
  const queryClient = useQueryClient()
  
  return (
    <aside className="sidebar-shell fixed inset-y-0 left-0 z-40 hidden w-72 flex-col border-r border-outline-variant/20 px-5 py-6 lg:flex">
      <Link to="/" className="mb-10 flex items-center gap-4">
        <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-primary/20 bg-primary/10 text-primary">
          <Sparkles size={22} />
        </div>
        <div>
          <div className="font-headline text-lg font-bold text-white">OfficeCLI</div>
          <div className="info-eyebrow text-outline">document runtime / app</div>
        </div>
      </Link>

      <div className="soft-panel mb-8 border border-outline-variant/20 bg-surface-container-low/70 p-4">
        <div className="info-eyebrow text-outline">Signed in as</div>
        <div className="mt-3 text-sm font-semibold text-white">{user.name || user.email}</div>
        <div className="mt-1 break-all text-xs text-outline">{user.email}</div>
      </div>

      <nav className="space-y-2">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            onMouseEnter={() => item.prefetch?.(queryClient)}
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
    </aside>
  )
}

export function AppTopBar({ user }: { user: User }) {
  const navigate = useNavigate()
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => navigate('/login', { replace: true }),
  })

  return (
    <header className="topbar-shell sticky top-0 z-30 mb-8 flex items-center justify-between gap-4 border border-outline-variant/20 px-5 py-4 backdrop-blur-xl">
      <div>
        <div className="info-eyebrow-wide text-primary">Live Infrastructure</div>
        <div className="mt-2 text-2xl font-bold text-white">Production document control</div>
      </div>
      <div className="flex items-center gap-3">
        <a href={inviteRewardGuideHref} target="_blank" rel="noreferrer" className="ghost-button text-xs">Docs</a>
        <a href="https://officecli.io/pricing" target="_blank" rel="noreferrer" className="ghost-button text-xs">Pricing</a>
        <div className="hidden rounded-full border border-outline-variant/20 px-4 py-2 text-right sm:block">
          <div className="info-eyebrow-mid text-outline">Operator</div>
          <div className="text-sm font-semibold text-white">{user.name || user.email}</div>
        </div>
        <button type="button" className="ghost-button px-4 text-xs" onClick={() => logout.mutate()} disabled={logout.isPending}>
          <LogOut size={14} />
          Sign out
        </button>
      </div>
    </header>
  )
}
