export interface NavItem {
  to: string
  label: string
  newTab?: boolean
}

const viteEnv = (import.meta as ImportMeta & { env?: Record<string, string | undefined> }).env

export const siteBaseURL = viteEnv?.VITE_SITE_BASE_URL ?? 'https://officecli.io'
export const platformBaseURL = viteEnv?.VITE_PLATFORM_BASE_URL ?? 'https://platform.officecli.io'
export const platformAppURL = `${platformBaseURL}/app`
export const platformBillingURL = `${platformBaseURL}/app/billing`
export const platformLicenseAPIURL = `${platformBaseURL}/api/license/check`

export const navItems: NavItem[] = [
  { to: '/#', label: 'Home' },
  { to: '/#pricing', label: 'Pricing' },
  { to: '/#download', label: 'Install the CLI' },
  { to: '/docs', label: 'Product Docs', newTab: true },
  { to: '/#faq', label: 'FAQ' },
]

export const footerGroups = [
  {
    title: 'Product',
    links: [
      { label: 'Install the CLI', to: '/#download', external: false },
      { label: 'Agent Skills', to: '/claude-code-codex-office-skills', external: false },
      { label: 'Product Docs', to: '/docs', external: false },
      { label: 'Console', to: platformAppURL, external: true },
    ],
  },
  {
    title: 'Platform',
    links: [
      { label: 'Login', to: platformAppURL, external: true },
      { label: 'Billing', to: platformBillingURL, external: true },
      { label: 'Security', to: '/#faq', external: false },
    ],
  },
  {
    title: 'Legal',
    links: [
      { label: 'Privacy', to: platformAppURL, external: true },
      { label: 'Terms', to: platformAppURL, external: true },
      { label: 'DPA', to: platformAppURL, external: true },
    ],
  },
] as const
