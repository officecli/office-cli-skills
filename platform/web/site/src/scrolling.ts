function isJSDOMScrollToStub() {
  const scrollTo = window.scrollTo as typeof window.scrollTo & { mock?: unknown; _isMockFunction?: boolean }
  return navigator.userAgent.includes('jsdom') && !scrollTo.mock && !scrollTo._isMockFunction
}

export function scrollToPageTop() {
  if (isJSDOMScrollToStub()) return

  window.scrollTo({ top: 0, behavior: 'smooth' })
}

export function scrollToHashTarget(hash: string) {
  const element = document.getElementById(decodeURIComponent(hash.slice(1)))
  if (!element) return false

  element.scrollIntoView({ behavior: 'smooth' })
  return true
}
