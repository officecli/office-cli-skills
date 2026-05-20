import { SITE_ANALYTICS_EVENTS } from './analytics-events'
import { siteBaseURL } from './siteData'

const TRACKED_PARAMS = [
  'invite',
  'visitor_id',
  'utm_source',
  'utm_medium',
  'utm_campaign',
  'utm_term',
  'utm_content',
  'utm_id',
] as const

type EventParams = Record<string, string | number | boolean | undefined>
type TrackPayload = Record<string, string | number | boolean | Record<string, string | number | boolean> | undefined>

declare global {
  interface Window {
    dataLayer?: unknown[]
    gtag?: (...args: unknown[]) => void
  }
}

let analyticsInitialized = false
const visitorCookieName = 'ocli_visitor_id'
const visitorCookieMaxAge = 180 * 24 * 60 * 60

function getMeasurementID() {
  const viteEnv = (import.meta as ImportMeta & { env?: Record<string, string | undefined> }).env
  return (process.env.VITE_GA4_MEASUREMENT_ID ?? viteEnv?.VITE_GA4_MEASUREMENT_ID)?.trim()
}

export function analyticsEnabled() {
  return Boolean(getMeasurementID())
}

export function initAnalytics() {
  const measurementID = getMeasurementID()
  if (!measurementID || analyticsInitialized) return

  const scriptID = 'ga4-script'
  if (!document.getElementById(scriptID)) {
    const script = document.createElement('script')
    script.id = scriptID
    script.async = true
    script.src = `https://www.googletagmanager.com/gtag/js?id=${measurementID}`
    document.head.appendChild(script)
  }

  window.dataLayer = window.dataLayer ?? []
  window.gtag =
    window.gtag ??
    function gtag() {
      window.dataLayer?.push(arguments)
    }
  window.gtag('js', new Date())
  window.gtag('config', measurementID, { send_page_view: false })
  analyticsInitialized = true
}

export function extractAttributionParams(currentSearch: string) {
  const source = new URLSearchParams(currentSearch)
  return Object.fromEntries(
    TRACKED_PARAMS.flatMap((key) => {
      const value = source.get(key)
      return value ? [[key, value]] : []
    }),
  )
}

function attributionWithVisitorID(currentSearch: string) {
  const attrs = extractAttributionParams(currentSearch)
  return { ...attrs, visitor_id: ensureVisitorID(currentSearch) }
}

export function ensureVisitorID(currentSearch = window.location.search) {
  const existing = readCookie(visitorCookieName)
  if (existing && isVisitorIDValid(existing)) return existing
  const fromURL = new URLSearchParams(currentSearch).get('visitor_id')?.trim()
  const visitorID = fromURL && isVisitorIDValid(fromURL) ? fromURL : generateVisitorID()
  writeVisitorCookie(visitorID)
  return visitorID
}

export function buildTrackedURL(target: string, currentSearch: string) {
  const origin = typeof window !== 'undefined' ? window.location.origin : siteBaseURL
  const url = new URL(target, origin)
  const attrs = attributionWithVisitorID(currentSearch)

  for (const [key, value] of Object.entries(attrs)) {
    if (!url.searchParams.has(key)) {
      url.searchParams.set(key, value)
    }
  }

  trackAttributionCarry(url.pathname, attrs)

  if (target.startsWith('http://') || target.startsWith('https://')) {
    return url.toString()
  }
  return `${url.pathname}${url.search}${url.hash}`
}

export function trackEvent(name: string, params: EventParams = {}) {
  const payload = buildTrackPayload(name, params)
  void postFirstPartyEvent(payload)
  if (analyticsEnabled()) {
    window.dataLayer = window.dataLayer ?? []
    window.gtag?.('event', name, params)
  }
}

export function trackPageView(pathname: string, currentSearch: string) {
  const attrs = extractAttributionParams(currentSearch)
  trackEvent('page_view', {
    page_path: pathname,
    page_search: currentSearch,
    surface: 'site',
    ...attrs,
  })
  if (Object.keys(attrs).length > 0) {
    trackEvent(SITE_ANALYTICS_EVENTS.inviteParamHit, { surface: 'site', page_path: pathname, ...attrs })
  }
}

function trackAttributionCarry(destinationPath: string, attrs: Record<string, string>) {
  if (Object.keys(attrs).length === 0) return
  trackEvent(SITE_ANALYTICS_EVENTS.inviteParamCarried, {
    surface: 'site',
    destination_path: destinationPath,
    ...attrs,
  })
}

function buildTrackPayload(name: string, params: EventParams): TrackPayload {
  const attrs = attributionWithVisitorID(window.location.search)
  const payload: TrackPayload = {
    ...attrs,
    event_name: name,
    surface: typeof params.surface === 'string' ? params.surface : 'site',
    page_path: typeof params.page_path === 'string' ? params.page_path : window.location.pathname,
    referrer: document.referrer || undefined,
  }
  const metadata: Record<string, string | number | boolean> = {}
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined) continue
    if (key === 'visitor_id') continue
    if (['surface', 'invite', 'utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content', 'utm_id', 'page_path', 'referrer', 'fingerprint_hash', 'api_key_id', 'order_id'].includes(key)) {
      payload[key] = value
    } else {
      metadata[key] = value
    }
  }
  if (Object.keys(metadata).length > 0) {
    payload.metadata = metadata
  }
  return payload
}

async function postFirstPartyEvent(payload: TrackPayload) {
  try {
    await fetch('/api/events/track', {
      method: 'POST',
      credentials: 'include',
      keepalive: true,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
  } catch {
    // Analytics must never block navigation.
  }
}

function readCookie(name: string) {
  return document.cookie
    .split(';')
    .map((part) => part.trim())
    .find((part) => part.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}

function writeVisitorCookie(visitorID: string) {
  document.cookie = `${visitorCookieName}=${encodeURIComponent(visitorID)}; Max-Age=${visitorCookieMaxAge}; Path=/; SameSite=Lax`
}

function isVisitorIDValid(value: string) {
  return value.length > 0 && value.length <= 96 && /^[A-Za-z0-9_.:-]+$/.test(value)
}

function generateVisitorID() {
  if (crypto.randomUUID) {
    return crypto.randomUUID()
  }
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}
