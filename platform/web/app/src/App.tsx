import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { api } from './api'
import { initAnalytics, trackPageView } from './analytics'
import { AppSidebar, AppTopBar } from './components/chrome'
import LoginPage from './pages/LoginPage'
import AccessDeniedPage from './pages/AccessDeniedPage'
import OverviewPage from './pages/OverviewPage'
import ApiKeysPage from './pages/ApiKeysPage'
import BillingPage from './pages/BillingPage'
import UsagePage from './pages/UsagePage'
import DownloadsPage from './pages/DownloadsPage'

function LoadingScreen() {
  return <div className="info-eyebrow grid min-h-screen place-items-center bg-background text-sm text-outline">Loading workspace…</div>
}

function ProtectedShell() {
  const { data: user, isLoading, error } = useQuery({ queryKey: ['app-me'], queryFn: api.me, retry: false })

  if (isLoading) return <LoadingScreen />
  if (error || !user) return <Navigate to="/login" replace />

  return (
    <div className="min-h-screen bg-background text-on-surface">
      <AppSidebar user={user} />
      <main className="px-4 py-4 lg:ml-72 lg:p-6">
        <div className="mx-auto max-w-7xl">
          <AppTopBar user={user} />
          <Routes>
            <Route path="/" element={<OverviewPage />} />
            <Route path="/api-keys" element={<ApiKeysPage />} />
            <Route path="/billing" element={<BillingPage />} />
            <Route path="/usage" element={<UsagePage />} />
            <Route path="/downloads" element={<DownloadsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </div>
      </main>
    </div>
  )
}

export default function App() {
  const location = useLocation()

  useEffect(() => {
    initAnalytics()
  }, [])

  useEffect(() => {
    trackPageView(location.pathname, location.search)
  }, [location.pathname, location.search])

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/access-denied" element={<AccessDeniedPage />} />
      <Route path="/*" element={<ProtectedShell />} />
    </Routes>
  )
}
