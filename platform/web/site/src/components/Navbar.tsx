import { motion } from 'motion/react'
import { Link, NavLink, useLocation } from 'react-router-dom'
import { buildTrackedURL, extractAttributionParams, trackEvent } from '../analytics'
import { SITE_ANALYTICS_EVENTS } from '../analytics-events'
import { navItems, platformAppURL } from '../siteData'

export default function Navbar() {
  const location = useLocation()
  const platformAppHref = buildTrackedURL(platformAppURL, location.search)

  return (
    <nav className="fixed top-0 w-full z-50 bg-[#131313]/80 backdrop-blur-xl border-b border-white/5">
      <div className="flex justify-between items-center px-8 py-4 max-w-[1440px] mx-auto font-headline tracking-tight gap-8">
        <Link className="text-2xl font-bold tracking-tighter text-white" to="/">OfficeCLI</Link>
        <div className="hidden md:flex gap-8 items-center">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) => isActive
                ? 'text-[#4f8eff] font-bold border-b-2 border-[#4f8eff] pb-1'
                : 'text-gray-400 hover:text-white transition-colors'}
            >
              {item.label}
            </NavLink>
          ))}
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
