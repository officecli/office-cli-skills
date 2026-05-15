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

const adminBaseURL = 'https://platform.officecli.io/admin/'
const defaultImage = 'https://officecli.io/og-cover.svg'
const defaultRobots = 'noindex,nofollow'

function buildCanonical(pathname: string) {
  if (!pathname || pathname === '/') return adminBaseURL
  return new URL(pathname.replace(/^\//, ''), adminBaseURL).toString()
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
      siteName: 'OfficeCLI Admin',
    },
    twitter: {
      title,
      description,
      image: defaultImage,
    },
  }
}

const routeSEO: Record<string, RouteSEO> = {
  '/': buildRouteSEO('/', 'OfficeCLI Admin | Overview', 'Review governance posture, credential inventory, and account hosted credit state across the OfficeCLI platform.'),
  '/growth': buildRouteSEO('/growth', 'OfficeCLI Admin | Growth', 'Inspect reward grants, referrals, and Discord connection state from the OfficeCLI admin plane.'),
  '/hosted-pricing': buildRouteSEO('/hosted-pricing', 'OfficeCLI Admin | Hosted Pricing', 'Review config-backed hosted pricing rules for the OfficeCLI platform.'),
  '/api-keys': buildRouteSEO('/api-keys', 'OfficeCLI Admin | API Keys', 'Audit and edit OfficeCLI platform API keys, limits, and routing metadata.'),
  '/users': buildRouteSEO('/users', 'OfficeCLI Admin | Users', 'Inspect registered OfficeCLI platform users and account state from the admin plane.'),
  '/orders': buildRouteSEO('/orders', 'OfficeCLI Admin | Orders', 'Track OfficeCLI platform orders, checkout transitions, and operator actions.'),
  '/billing-events': buildRouteSEO('/billing-events', 'OfficeCLI Admin | Billing Events', 'Review Stripe webhook ingestion and payment-side event processing for OfficeCLI.'),
  '/quota-sources': buildRouteSEO('/quota-sources', 'OfficeCLI Admin | Quota Sources', 'Audit OfficeCLI quota sources across free, reward, paid, and hosted surfaces.'),
  '/free-quotas': buildRouteSEO('/free-quotas', 'OfficeCLI Admin | Free Trial Devices', 'Inspect daily anonymous free-trial device quota for OfficeCLI.'),
  '/usage-events': buildRouteSEO('/usage-events', 'OfficeCLI Admin | Usage Events', 'Filter OfficeCLI usage events to investigate blocked or allowed traffic.'),
  '/access-denied': buildRouteSEO('/access-denied', 'OfficeCLI Admin | Access Denied', 'This Google account is not present in the OfficeCLI admin allowlist.'),
  '/login': buildRouteSEO('/login', 'OfficeCLI Admin | Page Not Found', 'The requested OfficeCLI admin route could not be found.'),
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
  return routeSEO[normalizePathname(pathname)] ?? buildRouteSEO(pathname, 'OfficeCLI Admin', 'OfficeCLI admin surface.')
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
