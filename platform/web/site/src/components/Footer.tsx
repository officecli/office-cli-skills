import { Link, useLocation } from 'react-router-dom'
import { buildTrackedURL } from '../analytics'
import { scrollToHashTarget, scrollToPageTop } from '../scrolling'
import { footerGroups } from '../siteData'

export default function Footer() {
  const location = useLocation()

  const scrollToFooterTarget = (to: string) => {
    const target = new URL(to, window.location.origin)
    if (target.pathname !== location.pathname) return

    if (target.hash) {
      scrollToHashTarget(target.hash)
      return
    }

    scrollToPageTop()
  }

  return (
    <footer className="bg-surface-low w-full border-t border-white/5">
      <div className="grid grid-cols-2 md:grid-cols-5 gap-8 px-12 py-16 w-full max-w-[1440px] mx-auto">
        <div className="col-span-2 md:col-span-1">
          <div className="text-lg font-black text-white mb-6">OfficeCLI</div>
          <p className="text-gray-500 font-headline text-sm uppercase tracking-widest leading-relaxed">
            © 2026 OfficeCLI.<br />Local-first document operations.
          </p>
        </div>
        <div>
          <h5 className="text-white font-bold mb-6">Explore</h5>
          <ul className="space-y-4 text-gray-500 font-headline text-sm uppercase tracking-widest">
            <li><Link className="hover:text-primary transition-colors" to="/" onClick={() => scrollToFooterTarget('/')}>Home</Link></li>
            <li><Link className="hover:text-primary transition-colors" to="/download" onClick={() => scrollToFooterTarget('/download')}>Install the CLI</Link></li>
            <li><Link className="hover:text-primary transition-colors" to="/docs" onClick={() => scrollToFooterTarget('/docs')}>Product Docs</Link></li>
          </ul>
        </div>
        {footerGroups.map((group) => (
          <div key={group.title}>
            <h5 className="text-white font-bold mb-6">{group.title}</h5>
            <ul className="space-y-4 text-gray-500 font-headline text-sm uppercase tracking-widest">
              {group.links.map((link) => (
                <li key={link.label}>
                  {link.external ? (
                    <a className="hover:text-primary transition-colors" href={buildTrackedURL(link.to, location.search)}>{link.label}</a>
                  ) : (
                    <Link className="hover:text-primary transition-colors" to={link.to} onClick={() => scrollToFooterTarget(link.to)}>{link.label}</Link>
                  )}
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </footer>
  )
}
