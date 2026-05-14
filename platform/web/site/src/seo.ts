import type { FAQEntry } from './agentSkillsData'
import {
  agentSkillsFAQs,
  agentSkillsHubPath,
  agentSkillsRoutes,
  agentSkillsSubpages,
  getAgentSkillsRoute,
  legacyAgentSkillsPath,
} from './agentSkillsData'

export interface RouteSEO {
  title: string
  description: string
  canonical: string
  robots: string
  openGraph: {
    type: string
    title: string
    description: string
    url: string
    image: string
    siteName: string
  }
  twitter: {
    card: string
    title: string
    description: string
    image: string
  }
  jsonLd?: Array<Record<string, unknown>>
}

export const siteBaseURL = 'https://officecli.io'
const siteName = 'OfficeCLI'
const defaultRobots = 'index,follow'
const defaultImage = `${siteBaseURL}/og-cover.svg`
const skillsImage = `${siteBaseURL}/social-preview-officecli.png`

const homeTitle = 'OfficeCLI | External and Hosted AI PPTX, DOCX, XLSX, REPORT, and IMG Generator'
const homeDescription =
  'OfficeCLI supports External Mode with free unlimited BYO LLM endpoint generation and Hosted Mode with OfficeCLI-managed runtime using hosted credits. Generate PPTX, DOCX, XLSX, REPORT, and IMG outputs from one dependency-free binary.'

export const homeFAQs: FAQEntry[] = [
  {
    q: 'What makes OfficeCLI different from other AI document CLIs?',
    a: 'OfficeCLI ships an end-to-end publish path. Every successful PPTX, DOCX, XLSX, REPORT, or standalone IMG generation can be turned into a password-protected online preview link with one command — no extra hosting, gateway, or upload step. Toggle per command with `--no-publish` or globally via `officecli config set-publish`. Other terminal-first AI document generators stop at a local file.',
  },
  {
    q: 'How does one-command online publish work?',
    a: 'After a successful generation, OfficeCLI calls the OfficeCLI publish service, returns a shareable `officecli.io/p/<id>` URL, and prints an auto-generated access password protecting the preview. Run `officecli config set-publish` once to enable the channel; subsequent `officecli new ...` runs publish by default for documents and standalone images. Add `--no-publish` to keep any single run fully local. Set `OFFICE_CLI_DEFAULT_PUBLISH=false` to flip the default for batch jobs.',
  },
  {
    q: 'Is OfficeCLI only for generating files?',
    a: 'No. Generation is the primary focus right now, but the product direction is broader document operations: standalone image generation, conversion, content modification, summarization, extraction, and layout handling.',
  },
  {
    q: 'Do I need Docker, Kubernetes, or a backend?',
    a: 'Not for the core local workflow. OfficeCLI is designed to stay lightweight: one binary plus your LLM endpoint. Platform features are optional, not a requirement for basic local use. Standalone IMG generation does call the OfficeCLI image service for license-aware quota.',
  },
  {
    q: 'What document types work today?',
    a: 'The current public release generates PPTX, DOCX, XLSX, workbook-backed REPORT outputs, and standalone IMG visuals via `new img`. It can also score and review local PPTX files, and publish any of those outputs as a password-protected online preview link with one command.',
  },
  {
    q: 'How does standalone image generation work?',
    a: '`officecli new img` calls the OfficeCLI image service, supports `--ratio square|landscape|portrait`, an explicit `--size <WxH>`, and one or more `--reference-image` inputs. A separate free image bucket of 3 images per user per day is tracked independently from the document bucket, and successful images publish online previews by default when publishing is configured.',
  },
  {
    q: 'Do I need LibreOffice or Microsoft Office installed?',
    a: 'Not for generation. PPTX review can run structural checks without extra tools. If soffice is installed, OfficeCLI can add a stronger visual review pass.',
  },
  {
    q: 'What install options are supported?',
    a: 'Homebrew, npm, the official install script, and manual release binaries are all supported on macOS and Linux for x64 and arm64.',
  },
  {
    q: 'When do I need platform.officecli.io?',
    a: 'Use the platform when you need paid access management, billing, API-key workflows, image-quota monitoring, or to inspect, revoke, and manage published online previews from a UI.',
  },
]

function buildCanonical(pathname: string) {
  return new URL(pathname, siteBaseURL).toString()
}

function buildRouteSEO(
  pathname: string,
  title: string,
  description: string,
  jsonLd?: RouteSEO['jsonLd'],
  options?: { canonicalPath?: string; image?: string },
): RouteSEO {
  const canonical = buildCanonical(options?.canonicalPath ?? pathname)
  const image = options?.image ?? defaultImage
  return {
    title,
    description,
    canonical,
    robots: defaultRobots,
    openGraph: {
      type: 'website',
      title,
      description,
      url: canonical,
      image,
      siteName,
    },
    twitter: {
      card: 'summary_large_image',
      title,
      description,
      image,
    },
    jsonLd,
  }
}

function buildFAQJSONLD(faqs: FAQEntry[]) {
  return {
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    mainEntity: faqs.map((faq) => ({
      '@type': 'Question',
      name: faq.q,
      acceptedAnswer: {
        '@type': 'Answer',
        text: faq.a,
      },
    })),
  }
}

function buildBreadcrumbJSONLD(items: Array<{ name: string; path: string }>) {
  return {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: items.map((item, index) => ({
      '@type': 'ListItem',
      position: index + 1,
      name: item.name,
      item: buildCanonical(item.path),
    })),
  }
}

function buildSkillsBreadcrumbItems(pathname: string) {
  const normalizedPath = pathname === legacyAgentSkillsPath ? agentSkillsHubPath : pathname
  const route = getAgentSkillsRoute(normalizedPath)
  if (!route) return []

  if (route.path === agentSkillsHubPath) {
    return [
      { name: 'Home', path: '/' },
      { name: 'officecli', path: agentSkillsHubPath },
    ]
  }

  return [
    { name: 'Home', path: '/' },
    { name: 'officecli', path: agentSkillsHubPath },
    { name: route.label, path: route.path },
  ]
}

function buildSkillsCollectionJSONLD() {
  const hubRoute = getAgentSkillsRoute(agentSkillsHubPath)
  if (!hubRoute) return undefined

  return {
    '@context': 'https://schema.org',
    '@type': 'CollectionPage',
    name: 'officecli',
    url: buildCanonical(agentSkillsHubPath),
    description: hubRoute.seoDescription,
    isPartOf: {
      '@type': 'WebSite',
      name: siteName,
      url: buildCanonical('/'),
    },
    hasPart: agentSkillsSubpages.map((route) => ({
      '@type': 'WebPage',
      name: route.label,
      url: buildCanonical(route.path),
      description: route.description,
    })),
  }
}

function buildSkillsWebPageJSONLD(pathname: string) {
  const normalizedPath = pathname === legacyAgentSkillsPath ? agentSkillsHubPath : pathname
  const route = getAgentSkillsRoute(normalizedPath)
  if (!route) return undefined

  return {
    '@context': 'https://schema.org',
    '@type': 'WebPage',
    name: route.seoTitle,
    url: buildCanonical(route.path),
    description: route.seoDescription,
    isPartOf: {
      '@type': 'WebSite',
      name: siteName,
      url: buildCanonical('/'),
    },
  }
}

function buildSkillsJSONLD(pathname: string): RouteSEO['jsonLd'] {
  const normalizedPath = pathname === legacyAgentSkillsPath ? agentSkillsHubPath : pathname
  const route = getAgentSkillsRoute(normalizedPath)
  if (!route) return undefined

  const jsonLd: Array<Record<string, unknown>> = []
  const webPage = buildSkillsWebPageJSONLD(normalizedPath)
  if (webPage) jsonLd.push(webPage)

  const breadcrumbs = buildSkillsBreadcrumbItems(normalizedPath)
  if (breadcrumbs.length > 0) {
    jsonLd.push(buildBreadcrumbJSONLD(breadcrumbs))
  }

  if (normalizedPath === agentSkillsHubPath) {
    const collection = buildSkillsCollectionJSONLD()
    if (collection) jsonLd.push(collection)
    jsonLd.push(buildFAQJSONLD(agentSkillsFAQs))
  }

  if (normalizedPath === `${agentSkillsHubPath}/faq`) {
    jsonLd.push(buildFAQJSONLD(agentSkillsFAQs))
  }

  return jsonLd
}

const homeJSONLD: RouteSEO['jsonLd'] = [
  {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    name: siteName,
    applicationCategory: 'DeveloperApplication',
    operatingSystem: 'macOS, Linux',
    description: homeDescription,
    url: buildCanonical('/'),
    image: defaultImage,
  },
  buildFAQJSONLD(homeFAQs),
]

const agentSkillsRouteSEO = agentSkillsRoutes.reduce<Record<string, RouteSEO>>((acc, route) => {
  acc[route.path] = buildRouteSEO(route.path, route.seoTitle, route.seoDescription, buildSkillsJSONLD(route.path), {
    image: skillsImage,
  })
  return acc
}, {})

const agentSkillsOverviewSEO = agentSkillsRouteSEO[agentSkillsHubPath]
agentSkillsRouteSEO[legacyAgentSkillsPath] = buildRouteSEO(
  legacyAgentSkillsPath,
  agentSkillsOverviewSEO.title,
  agentSkillsOverviewSEO.description,
  buildSkillsJSONLD(legacyAgentSkillsPath),
  {
    canonicalPath: agentSkillsHubPath,
    image: skillsImage,
  },
)

export const routeSEO: Record<string, RouteSEO> = {
  '/': buildRouteSEO('/', homeTitle, homeDescription, homeJSONLD),
  ...agentSkillsRouteSEO,
  '/docs': buildRouteSEO(
    '/docs',
    'OfficeCLI Docs | PPTX, DOCX, XLSX, REPORT, IMG, and one-command online publish',
    'Review OfficeCLI capabilities, supported PPTX, DOCX, XLSX, REPORT, and standalone IMG outputs, and the one-command online publish flow that returns a password-protected preview URL after every successful generation.',
  ),
  '/download': buildRouteSEO(
    '/download',
    'Install OfficeCLI | External and Hosted AI document generation CLI',
    'Install OfficeCLI with Homebrew, npm, the official script, or manual binaries for External Mode, Hosted Mode, BYO LLM endpoint generation, hosted credits, and dependency-free PPTX, DOCX, XLSX, REPORT, and IMG generation.',
  ),
  '/pricing': buildRouteSEO(
    '/pricing',
    'OfficeCLI Pricing | External free unlimited and Hosted credits',
    'See OfficeCLI pricing: External Mode is free and unlimited, while Hosted Mode uses hosted credits for the OfficeCLI-managed runtime.',
  ),
  '/faq': buildRouteSEO(
    '/faq',
    'OfficeCLI FAQ | AI document generation CLI answers',
    'Read answers about OfficeCLI installation, supported formats, REPORT outputs, local-first usage, and optional platform features.',
  ),
  '/login': buildRouteSEO(
    '/login',
    'OfficeCLI Login | Open the platform',
    'Open the OfficeCLI platform for platform access, billing-aware workflows, API keys, and authenticated console features.',
  ),
}

export function normalizePathname(pathname: string) {
  if (!pathname || pathname === '/') return '/'
  return pathname.endsWith('/') ? pathname.slice(0, -1) : pathname
}

export function getRouteSEO(pathname: string) {
  return routeSEO[normalizePathname(pathname)] ?? routeSEO['/']
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

export function applyDocumentSEO(documentRef: Document, seo: RouteSEO) {
  documentRef.title = seo.title
  upsertMeta(documentRef, 'name', 'description', seo.description)
  upsertMeta(documentRef, 'name', 'robots', seo.robots)
  upsertMeta(documentRef, 'property', 'og:type', seo.openGraph.type)
  upsertMeta(documentRef, 'property', 'og:title', seo.openGraph.title)
  upsertMeta(documentRef, 'property', 'og:description', seo.openGraph.description)
  upsertMeta(documentRef, 'property', 'og:url', seo.openGraph.url)
  upsertMeta(documentRef, 'property', 'og:image', seo.openGraph.image)
  upsertMeta(documentRef, 'property', 'og:site_name', seo.openGraph.siteName)
  upsertMeta(documentRef, 'name', 'twitter:card', seo.twitter.card)
  upsertMeta(documentRef, 'name', 'twitter:title', seo.twitter.title)
  upsertMeta(documentRef, 'name', 'twitter:description', seo.twitter.description)
  upsertMeta(documentRef, 'name', 'twitter:image', seo.twitter.image)
  upsertCanonical(documentRef, seo.canonical)

  documentRef.head.querySelectorAll('script[data-route-jsonld]').forEach((node) => node.remove())
  for (const [index, entry] of (seo.jsonLd ?? []).entries()) {
    const script = documentRef.createElement('script')
    script.type = 'application/ld+json'
    script.dataset.routeJsonld = `${index}`
    script.textContent = JSON.stringify(entry)
    documentRef.head.appendChild(script)
  }
}
