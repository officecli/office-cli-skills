import { useEffect } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { initAnalytics, trackPageView } from './analytics'
import Navbar from './components/Navbar'
import Footer from './components/Footer'
import DocsPage from './pages/DocsPage'
import DownloadPage from './pages/DownloadPage'
import FAQPage from './pages/FAQPage'
import HomePage from './pages/HomePage'
import LoginPage from './pages/LoginPage'
import PricingPage from './pages/PricingPage'
import AgentSkillsPage from './pages/AgentSkillsPage'
import { applyDocumentSEO, getRouteSEO } from './seo'

function SiteShell() {
  const location = useLocation()

  useEffect(() => {
    initAnalytics()
  }, [])

  useEffect(() => {
    trackPageView(location.pathname, location.search)
  }, [location.pathname, location.search])

  useEffect(() => {
    applyDocumentSEO(document, getRouteSEO(location.pathname))
  }, [location.pathname])

  return (
    <div className="min-h-screen bg-background text-white selection:bg-primary/30">
      {location.pathname !== '/docs' && <Navbar />}
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/pricing" element={<PricingPage />} />
        <Route path="/download" element={<DownloadPage />} />
        <Route path="/docs" element={<DocsPage />} />
        <Route path="/claude-code-codex-office-skills" element={<AgentSkillsPage />} />
        <Route path="/faq" element={<FAQPage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      <Footer />
    </div>
  )
}

export default function App() {
  return <SiteShell />
}
