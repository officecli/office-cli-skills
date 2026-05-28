import { Activity, BookOpen, CreditCard, Funnel, Gift, Image, KeyRound, Layers3, LogOut, ReceiptText, ShieldCheck, TerminalSquare, Ticket, Users, SlidersHorizontal } from 'lucide-react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { Button, Layout, Space, Tag, Typography } from 'antd'
import { api } from '../api'
import { cn } from '../lib/utils'
import type { AdminIdentity } from '../types'
import { OfficeCliBrand } from './branding'

const navItems = [
  { to: '/', label: 'Overview', icon: ShieldCheck },
  { to: '/operations/funnel', label: 'Operations / Funnel', icon: Funnel },
  { to: '/growth', label: 'Growth', icon: Gift },
  { to: '/hosted-pricing', label: 'Hosted Pricing', icon: SlidersHorizontal },
  { to: '/image-templates', label: 'Image Templates', icon: Image },
  { to: '/api-keys', label: 'API Keys', icon: KeyRound },
  { to: '/users', label: 'Users', icon: Users },
  { to: '/orders', label: 'Orders', icon: CreditCard },
  { to: '/billing-events', label: 'Billing Events', icon: ReceiptText },
  { to: '/credit-ledger', label: 'Credit Ledger', icon: BookOpen },
  { to: '/quota-sources', label: 'Quota Sources', icon: Layers3 },
  { to: '/redemption-codes', label: 'Redemption Codes', icon: Ticket },
  { to: '/redemption-records', label: 'Redemption Records', icon: Gift },
  { to: '/usage-events', label: 'Usage Events', icon: Activity },
]

export function AdminSidebar({ admin }: { admin: AdminIdentity }) {
  return (
    <Layout.Sider width={288} className="admin-sider hidden lg:block">
      <div className="sticky top-0 px-5 py-6">
        <Link to="/" className="mb-8 flex items-center gap-4">
          <OfficeCliBrand
            markClassName="h-12 w-12"
            titleClassName="text-lg font-bold text-white"
            subtitle="admin governance / restricted"
          />
        </Link>

        <div className="admin-operator-card mb-6">
          <Typography.Text className="admin-eyebrow" type="secondary">Authorized operator</Typography.Text>
          <Typography.Text className="mt-3 block font-semibold">{admin.name || admin.email}</Typography.Text>
          <Typography.Text className="mt-1 block break-all text-xs" type="secondary">{admin.email}</Typography.Text>
        </div>

        <nav>
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className="block"
            >
              {({ isActive }) => (
                <div className={cn('admin-nav-item', isActive && 'admin-nav-item-active')}>
                  <item.icon size={18} />
                  <span>{item.label}</span>
                </div>
              )}
            </NavLink>
          ))}
        </nav>

        <div className="admin-policy-card mt-8 text-sm">
          <Typography.Text className="admin-eyebrow flex items-center gap-2" type="warning">
            <TerminalSquare size={14} />
            access policy
          </Typography.Text>
          <Typography.Paragraph className="!mb-0 !mt-3" type="secondary">Company OAuth2 authentication is required first, then the exact account email must exist in the OfficeCLI admin allowlist. The default production expectation is a single operator: luyang950@gmail.com.</Typography.Paragraph>
        </div>
      </div>
    </Layout.Sider>
  )
}

export function AdminTopBar({ admin }: { admin: AdminIdentity }) {
  const navigate = useNavigate()
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => navigate('/login', { replace: true }),
  })

  return (
    <Layout.Header className="admin-header sticky top-0 z-30 mb-6 flex h-auto min-h-24 flex-col items-start justify-between gap-4 px-5 py-4 xl:flex-row xl:items-center">
      <div className="min-w-0">
        <Typography.Text className="admin-eyebrow text-primary">Governance plane</Typography.Text>
        <Typography.Title level={3} className="!mb-0 !mt-1 max-w-3xl break-words !text-xl xl:!text-2xl">Operator visibility for quota and abuse control</Typography.Title>
      </div>
      <Space wrap className="shrink-0">
        <Button href="https://officecli.io/docs">Docs</Button>
        <Button href="https://officecli.io/faq">Policy</Button>
        <Tag className="hidden sm:inline-flex">Session: {admin.auth_method || 'google'}</Tag>
        <Button onClick={() => logout.mutate()} disabled={logout.isPending} loading={logout.isPending} icon={<LogOut size={14} />}>
          Sign out
        </Button>
      </Space>
    </Layout.Header>
  )
}
