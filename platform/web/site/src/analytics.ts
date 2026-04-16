import { SITE_ANALYTICS_EVENTS } from './analytics-events'
import { siteBaseURL } from './siteData'

const TRACKED_PARAMS = [
  'invite',
  'utm_source',
  'utm_medium',
  'utm_campaign',
  'utm_term',
  'utm_content',
  'utm_id',
] as const

type EventParams = Record<string, string | number | boolean | undefined>

declare global {
  interface Window {
    dataLayer?: unknown[]
    gtag?: (...args: unknown[]) => void
  }
}

let analyticsInitialized = false

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

export function buildTrackedURL(target: string, currentSearch: string) {
  const origin = typeof window !== 'undefined' ? window.location.origin : siteBaseURL
  const url = new URL(target, origin)
  const attrs = extractAttributionParams(currentSearch)

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
  if (!analyticsEnabled()) return
  window.dataLayer = window.dataLayer ?? []
  window.gtag?.('event', name, params)
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
