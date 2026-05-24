import { Archive, CreditCard, Download, KeyRound, LayoutDashboard, LogOut, Menu as MenuIcon, Ticket, Workflow } from 'lucide-react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Button, Dropdown, Layout, Space, Tag, Typography } from 'antd'
import type { MenuProps } from 'antd'
import { api } from '../api'
import { cn } from '../lib/utils'
import type { User } from '../types'
import { OfficeCliBrand } from './branding'

const inviteRewardGuideHref = 'https://officecli.io/docs#invite-rewards'

const navItems = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-overview'], queryFn: api.overview })
    queryClient.prefetchQuery({ queryKey: ['app-growth'], queryFn: api.growth })
    queryClient.prefetchQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
  }},
  { to: '/api-keys', label: 'API Keys', icon: KeyRound, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
  }},
  { to: '/billing', label: 'Billing', icon: CreditCard, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['pricing'], queryFn: api.pricing })
    queryClient.prefetchQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
    queryClient.prefetchQuery({ queryKey: ['app-orders'], queryFn: api.orders })
  }},
  { to: '/quota', label: 'Legacy quota', icon: Archive, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-quota-summary'], queryFn: api.quotaSummary })
  }},
  { to: '/usage', label: 'Usage', icon: Workflow, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-usage'], queryFn: api.usage })
  }},
  { to: '/downloads', label: 'Downloads', icon: Download },
  { to: '/redeem', label: 'Redeem', icon: Ticket, prefetch: (queryClient: ReturnType<typeof useQueryClient>) => {
    queryClient.prefetchQuery({ queryKey: ['app-redemption-history'], queryFn: api.myRedemptions })
  }},
]

export function AppSidebar({ user }: { user: User }) {
  const queryClient = useQueryClient()

  return (
    <Layout.Sider width={288} className="app-sider hidden lg:block">
      <div className="sticky top-0 px-5 py-6">
        <Link to="/" className="mb-8 flex items-center gap-4">
          <OfficeCliBrand
            markClassName="h-12 w-12"
            titleClassName="text-lg font-bold text-white"
            subtitle="document runtime / app"
          />
        </Link>

        <div className="app-operator-card mb-6">
          <Typography.Text className="app-eyebrow" type="secondary">Signed in as</Typography.Text>
          <Typography.Text className="mt-3 block font-semibold">{user.name || user.email}</Typography.Text>
          <Typography.Text className="mt-1 block break-all text-xs" type="secondary">{user.email}</Typography.Text>
        </div>

        <nav>
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              onMouseEnter={() => item.prefetch?.(queryClient)}
              className="block"
            >
              {({ isActive }) => (
                <div className={cn('app-nav-item', isActive && 'app-nav-item-active')}>
                  <item.icon size={18} />
                  <span>{item.label}</span>
                </div>
              )}
            </NavLink>
          ))}
        </nav>
      </div>
    </Layout.Sider>
  )
}

export function AppTopBar({ user }: { user: User }) {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => navigate('/login', { replace: true }),
  })
  const activeNavKey = navItems.find((item) => item.to === '/' ? location.pathname === '/' : location.pathname.startsWith(item.to))?.to ?? '/'
  const mobileNavMenu: MenuProps = {
    selectedKeys: [activeNavKey],
    items: navItems.map((item) => ({
      key: item.to,
      icon: <item.icon size={16} />,
      label: item.label,
    })),
    onClick: ({ key }) => {
      const item = navItems.find((candidate) => candidate.to === key)
      item?.prefetch?.(queryClient)
      navigate(key)
    },
  }

  return (
    <Layout.Header className="app-header sticky top-0 z-30 mb-6 flex h-auto min-h-24 flex-col items-start justify-between gap-4 px-5 py-4 xl:flex-row xl:items-center">
      <div className="min-w-0">
        <Typography.Text className="app-eyebrow text-primary">Live Infrastructure</Typography.Text>
        <Typography.Title level={3} className="!mb-0 !mt-1 max-w-3xl break-words !text-xl xl:!text-2xl">Production document control</Typography.Title>
      </div>
      <Space wrap className="shrink-0">
        <Dropdown menu={mobileNavMenu} trigger={['click']}>
          <Button className="lg:hidden" icon={<MenuIcon size={14} />}>Menu</Button>
        </Dropdown>
        <Button href={inviteRewardGuideHref} target="_blank" rel="noreferrer">Docs</Button>
        <Button href="https://officecli.io/pricing" target="_blank" rel="noreferrer">Pricing</Button>
        <Tag className="hidden sm:inline-flex">User: {user.name || user.email}</Tag>
        <Button onClick={() => logout.mutate()} disabled={logout.isPending} loading={logout.isPending} icon={<LogOut size={14} />}>
          Sign out
        </Button>
      </Space>
    </Layout.Header>
  )
}
