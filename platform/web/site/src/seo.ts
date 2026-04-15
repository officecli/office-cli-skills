export interface FAQEntry {
  q: string
  a: string
}

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

const homeTitle = 'OfficeCLI | AI PPTX, DOCX, XLSX, and REPORT Generator for Terminal Workflows'
const homeDescription =
  'OfficeCLI is a local-first AI document generation CLI for PPTX, DOCX, XLSX, and REPORT outputs. Generate files from the terminal with your own LLM endpoint, without a backend stack or cluster.'

export const homeFAQs: FAQEntry[] = [
  {
    q: 'Is OfficeCLI only for generating files?',
    a: 'No. Generation is the primary focus right now, but the product direction is broader document operations: conversion, content modification, summarization, extraction, and layout handling.',
  },
  {
    q: 'Do I need Docker, Kubernetes, or a backend?',
    a: 'Not for the core local workflow. OfficeCLI is designed to stay lightweight: one binary plus your LLM endpoint. Platform features are optional, not a requirement for basic local use.',
  },
  {
    q: 'What document types work today?',
    a: 'The current public release generates PPTX, DOCX, XLSX, and workbook-backed REPORT outputs. It can also score and review local PPTX files.',
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
    a: 'Use the platform when you need paid access management, hosted runtime features, billing, API-key workflows, or optional online preview publishing.',
  },
]

function buildCanonical(pathname: string) {
  return new URL(pathname, siteBaseURL).toString()
}

function buildRouteSEO(pathname: string, title: string, description: string, jsonLd?: RouteSEO['jsonLd']): RouteSEO {
  const canonical = buildCanonical(pathname)
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
      image: defaultImage,
      siteName,
    },
    twitter: {
      card: 'summary_large_image',
      title,
      description,
      image: defaultImage,
    },
    jsonLd,
  }
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
  {
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    mainEntity: homeFAQs.map((faq) => ({
      '@type': 'Question',
      name: faq.q,
      acceptedAnswer: {
        '@type': 'Answer',
        text: faq.a,
      },
    })),
  },
]

export const routeSEO: Record<string, RouteSEO> = {
  '/': buildRouteSEO('/', homeTitle, homeDescription, homeJSONLD),
  '/docs': buildRouteSEO(
    '/docs',
    'OfficeCLI Docs | PPTX, DOCX, XLSX, and REPORT capabilities',
    'Review OfficeCLI capabilities, supported PPTX, DOCX, XLSX, and REPORT outputs, and understand which document automation workflows are available today.',
  ),
  '/download': buildRouteSEO(
    '/download',
    'Install OfficeCLI | AI document generation CLI for macOS and Linux',
    'Install OfficeCLI with Homebrew, npm, the official script, or manual binaries for macOS and Linux, then generate PPTX, DOCX, XLSX, and REPORT outputs from the terminal.',
  ),
  '/pricing': buildRouteSEO(
    '/pricing',
    'OfficeCLI Pricing | Paid access for document automation workflows',
    'See OfficeCLI pricing for hosted access, billing-aware document automation workflows, and secure checkout for recurring team usage.',
  ),
  '/faq': buildRouteSEO(
    '/faq',
    'OfficeCLI FAQ | AI document generation CLI answers',
    'Read answers about OfficeCLI installation, supported formats, REPORT outputs, local-first usage, and optional hosted platform features.',
  ),
  '/login': buildRouteSEO(
    '/login',
    'OfficeCLI Login | Open the hosted platform',
    'Open the OfficeCLI platform for hosted runtime access, billing-aware workflows, API keys, and authenticated console features.',
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
