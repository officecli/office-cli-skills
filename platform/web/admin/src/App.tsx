import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { api } from './api'
import { AdminSidebar, AdminTopBar } from './components/chrome'
import AccessDeniedPage from './pages/AccessDeniedPage'
import ApiKeysPage from './pages/ApiKeysPage'
import BillingEventsPage from './pages/BillingEventsPage'
import DashboardPage from './pages/DashboardPage'
import FreeQuotasPage from './pages/FreeQuotasPage'
import GrowthPage from './pages/GrowthPage'
import HostedPricingRulesPage from './pages/HostedPricingRulesPage'
import NotFoundPage from './pages/NotFoundPage'
import OrdersPage from './pages/OrdersPage'
import QuotaSourcesPage from './pages/QuotaSourcesPage'
import UsersPage from './pages/UsersPage'
import UsageEventsPage from './pages/UsageEventsPage'

function LoadingScreen() {
  return <div className="info-eyebrow grid min-h-screen place-items-center bg-background text-sm text-outline">Loading admin session...</div>
}

function AdminAuthRedirect() {
  useEffect(() => {
    const returnTo = `${window.location.pathname}${window.location.search}${window.location.hash}`
    api.login(returnTo.startsWith('/admin') ? returnTo : '/admin')
  }, [])

  return <LoadingScreen />
}

function ProtectedShell() {
  const { data: admin, isLoading, error } = useQuery({ queryKey: ['admin-session'], queryFn: api.session, retry: false })

  if (isLoading) return <LoadingScreen />
  if (error || !admin) return <AdminAuthRedirect />

  return (
    <div className="min-h-screen bg-background text-on-surface">
      <AdminSidebar admin={admin} />
      <main className="px-4 py-4 lg:ml-72 lg:p-6">
        <div className="mx-auto max-w-7xl">
          <AdminTopBar admin={admin} />
          <Routes>
            <Route path="/" element={<DashboardPage />} />
            <Route path="/growth" element={<GrowthPage />} />
            <Route path="/hosted-pricing" element={<HostedPricingRulesPage />} />
            <Route path="/api-keys" element={<ApiKeysPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/orders" element={<OrdersPage />} />
            <Route path="/billing-events" element={<BillingEventsPage />} />
            <Route path="/quota-sources" element={<QuotaSourcesPage />} />
            <Route path="/free-quotas" element={<FreeQuotasPage />} />
            <Route path="/usage-events" element={<UsageEventsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </div>
      </main>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<NotFoundPage />} />
      <Route path="/access-denied" element={<AccessDeniedPage />} />
      <Route path="/*" element={<ProtectedShell />} />
    </Routes>
  )
}
