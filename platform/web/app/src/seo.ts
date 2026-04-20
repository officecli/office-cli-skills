interface RouteSEO {
  title: string
  description: string
  canonical: string
  robots: string
  openGraph: {
    title: string
    description: string
    url: string
    image: string
    siteName: string
  }
  twitter: {
    title: string
    description: string
    image: string
  }
}

const appBaseURL = 'https://platform.officecli.io/app/'
const defaultImage = 'https://officecli.io/og-cover.svg'
const defaultRobots = 'noindex,nofollow'

function buildCanonical(pathname: string) {
  if (!pathname || pathname === '/') return appBaseURL
  return new URL(pathname.replace(/^\//, ''), appBaseURL).toString()
}

function buildRouteSEO(pathname: string, title: string, description: string): RouteSEO {
  const canonical = buildCanonical(pathname)
  return {
    title,
    description,
    canonical,
    robots: defaultRobots,
    openGraph: {
      title,
      description,
      url: canonical,
      image: defaultImage,
      siteName: 'OfficeCLI App',
    },
    twitter: {
      title,
      description,
      image: defaultImage,
    },
  }
}

const routeSEO: Record<string, RouteSEO> = {
  '/': buildRouteSEO('/', 'OfficeCLI App | Overview', 'Review workspace quota, invite growth, and key activity inside the OfficeCLI app workspace.'),
  '/quota': buildRouteSEO('/quota', 'OfficeCLI App | Quota', 'Track reward quota, paid quota, and account-owned document capacity in the OfficeCLI app.'),
  '/api-keys': buildRouteSEO('/api-keys', 'OfficeCLI App | API Keys', 'Manage OfficeCLI app API keys, quota posture, and key metadata from one workspace surface.'),
  '/billing': buildRouteSEO('/billing', 'OfficeCLI App | Billing', 'Review OfficeCLI app billing packs, checkout state, and order history for the current workspace.'),
  '/usage': buildRouteSEO('/usage', 'OfficeCLI App | Usage', 'Inspect recent OfficeCLI app usage events and policy outcomes for this workspace.'),
  '/downloads': buildRouteSEO('/downloads', 'OfficeCLI App | Downloads', 'Install or update OfficeCLI from the app workspace using the supported public distribution channels.'),
  '/login': buildRouteSEO('/login', 'OfficeCLI App | Sign In', 'Sign in to the OfficeCLI app workspace with Google authentication.'),
  '/access-denied': buildRouteSEO('/access-denied', 'OfficeCLI App | Access Denied', 'This Google account is not allowed to open the current OfficeCLI app workspace.'),
}

function normalizePathname(pathname: string) {
  if (!pathname || pathname === '/') return '/'
  return pathname.endsWith('/') ? pathname.slice(0, -1) : pathname
}

function upsertMeta(documentRef: Document, key: 'name' | 'property', value: string, content: string) {
  let tag = documentRef.head.querySelector<HTMLMetaElement>(`meta[${key}="${value}"]`)
  if (!tag) {
    tag = documentRef.createElement('meta')
    tag.setAttribute(key, value)
    documentRef.head.appendChild(tag)
  }
  tag.setAttribute('content', content)
}

function upsertCanonical(documentRef: Document, href: string) {
  let tag = documentRef.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
  if (!tag) {
    tag = documentRef.createElement('link')
    tag.setAttribute('rel', 'canonical')
    documentRef.head.appendChild(tag)
  }
  tag.setAttribute('href', href)
}

export function getRouteSEO(pathname: string) {
  return routeSEO[normalizePathname(pathname)] ?? buildRouteSEO(pathname, 'OfficeCLI App', 'OfficeCLI app workspace.')
}

export function applyDocumentSEO(documentRef: Document, seo: RouteSEO) {
  documentRef.title = seo.title
  upsertMeta(documentRef, 'name', 'description', seo.description)
  upsertMeta(documentRef, 'name', 'robots', seo.robots)
  upsertMeta(documentRef, 'name', 'theme-color', '#0A1522')
  upsertMeta(documentRef, 'property', 'og:title', seo.openGraph.title)
  upsertMeta(documentRef, 'property', 'og:description', seo.openGraph.description)
  upsertMeta(documentRef, 'property', 'og:url', seo.openGraph.url)
  upsertMeta(documentRef, 'property', 'og:image', seo.openGraph.image)
  upsertMeta(documentRef, 'property', 'og:site_name', seo.openGraph.siteName)
  upsertMeta(documentRef, 'name', 'twitter:card', 'summary_large_image')
  upsertMeta(documentRef, 'name', 'twitter:title', seo.twitter.title)
  upsertMeta(documentRef, 'name', 'twitter:description', seo.twitter.description)
  upsertMeta(documentRef, 'name', 'twitter:image', seo.twitter.image)
  upsertCanonical(documentRef, seo.canonical)
}
