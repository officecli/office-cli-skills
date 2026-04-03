import { APP_ANALYTICS_EVENTS } from './analytics-events'

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

const measurementID = import.meta.env.VITE_GA4_MEASUREMENT_ID?.trim()
let analyticsInitialized = false

export function analyticsEnabled() {
  return Boolean(measurementID)
}

export function initAnalytics() {
  if (!analyticsEnabled() || analyticsInitialized) return

  const scriptID = 'ga4-script'
  if (!document.getElementById(scriptID)) {
    const script = document.createElement('script')
    script.id = scriptID
    script.async = true
    script.src = `https://www.googletagmanager.com/gtag/js?id=${measurementID}`
    document.head.appendChild(script)
  }

  window.dataLayer = window.dataLayer ?? []
  window.gtag = window.gtag ?? ((...args: unknown[]) => {
    window.dataLayer?.push(args)
  })
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

export function buildGoogleLoginURL(returnTo = '/app', currentSearch = window.location.search) {
  const url = new URL('/api/auth/google/login', window.location.origin)
  url.searchParams.set('return_to', returnTo)

  const attrs = extractAttributionParams(currentSearch)
  for (const [key, value] of Object.entries(attrs)) {
    url.searchParams.set(key, value)
  }

  trackAttributionCarry(url.pathname, attrs)

  return `${url.pathname}${url.search}`
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
    surface: 'app',
    ...attrs,
  })
  if (Object.keys(attrs).length > 0) {
    trackEvent(APP_ANALYTICS_EVENTS.inviteParamHit, { surface: 'app', page_path: pathname, ...attrs })
  }
}

function trackAttributionCarry(destinationPath: string, attrs: Record<string, string>) {
  if (Object.keys(attrs).length === 0) return
  trackEvent(APP_ANALYTICS_EVENTS.inviteParamCarried, {
    surface: 'app',
    destination_path: destinationPath,
    ...attrs,
  })
}
