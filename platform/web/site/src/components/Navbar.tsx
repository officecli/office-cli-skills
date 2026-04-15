import { useEffect, useState } from 'react'
import { motion } from 'motion/react'
import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import { navItems, platformAppURL } from '../siteData'

export default function Navbar() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)
  const [activeHash, setActiveHash] = useState(location.hash || '#')

  useEffect(() => {
    const handleHashChange = () => {
      setActiveHash(window.location.hash || '#')
    }
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [])

  useEffect(() => {
    if (location.pathname === '/' && location.hash) {
      const id = location.hash.replace('#', '')
      const element = document.getElementById(id)
      if (element) {
        try {
          element.scrollIntoView({ behavior: 'smooth' })
        } catch (e) {
          // ignore
        }
      }
    } else if (location.pathname === '/' && !location.hash) {
      try {
        if (typeof window !== 'undefined' && typeof window.scrollTo === 'function') {
          // jsdom doesn't implement window.scrollTo
          if (navigator.userAgent.includes('jsdom')) return
          window.scrollTo({ top: 0, behavior: 'smooth' })
        }
      } catch (e) {
        // ignore
      }
    }
  }, [location.pathname, location.hash])

  const handleAnchorClick = (e: React.MouseEvent<HTMLAnchorElement>, to: string) => {
    if (location.pathname === '/') {
      e.preventDefault()
      const hash = to.replace('/#', '#')
      if (hash === '#') {
        try {
          if (typeof window !== 'undefined' && typeof window.scrollTo === 'function') {
            if (navigator.userAgent.includes('jsdom')) return
            window.scrollTo({ top: 0, behavior: 'smooth' })
          }
        } catch (e) {
          // ignore
        }
        window.history.pushState(null, '', '/')
        setActiveHash('#')
      } else {
        const id = hash.replace('#', '')
        const element = document.getElementById(id)
        if (element) {
          try {
            element.scrollIntoView({ behavior: 'smooth' })
          } catch (e) {
            // ignore
          }
          window.history.pushState(null, '', hash)
          setActiveHash(hash)
        }
      }
    }
  }

  return (
    <nav className="fixed top-0 w-full z-50 bg-[#131313]/80 backdrop-blur-xl border-b border-white/5">
      <div className="flex justify-between items-center px-8 py-4 max-w-[1440px] mx-auto font-headline tracking-tight gap-8">
        <Link className="text-2xl font-bold tracking-tighter text-white" to="/#" onClick={(e) => handleAnchorClick(e as any, '/#')}>OfficeCLI</Link>
        <div className="hidden md:flex gap-8 items-center">
          {navItems.map((item) => {
            const isAnchor = item.to.startsWith('/#')
            const isActive = isAnchor
              ? (item.to === '/#' && activeHash === '#') || (item.to !== '/#' && activeHash === item.to.replace('/', ''))
              : location.pathname === item.to

            const className = isActive
              ? 'text-[#4f8eff] font-bold border-b-2 border-[#4f8eff] pb-1'
              : 'text-gray-400 hover:text-white transition-colors'

            return isAnchor ? (
              <a key={item.to} href={item.to} className={className} onClick={(e) => handleAnchorClick(e, item.to)}>
                {item.label}
              </a>
            ) : (
              <Link key={item.to} to={item.to} className={className}>
                {item.label}
              </Link>
            )
          })}
        </div>
        <div className="flex items-center gap-4">
          <a className="text-gray-400 hover:text-white transition-colors px-4 py-2" href={platformAppHref} onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.loginStart, { surface: 'site', placement: 'navbar', ...extractAttributionParams(location.search) })}>Login</a>
          <motion.a
            whileHover={{ scale: 0.95 }}
            whileTap={{ scale: 0.9 }}
            className="bg-gradient-to-br from-primary to-primary-container text-[#002e6b] px-6 py-2 rounded-md font-bold transition-all"
            href={platformAppHref}
            onClick={() => trackEvent(SITE_ANALYTICS_EVENTS.consoleOpen, { surface: 'site', placement: 'navbar', ...extractAttributionParams(location.search) })}
          >
            Console
          </motion.a>
        </div>
      </div>
    </nav>
  )
}
