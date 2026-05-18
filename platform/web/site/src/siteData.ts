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
  { to: '/officecli', label: 'Agent Skills' },
  { to: '/pricing', label: 'Pricing' },
  { to: '/download', label: 'Install the CLI' },
  { to: '/docs', label: 'Product Docs', newTab: true },
  { to: '/faq', label: 'FAQ' },
]

export const footerGroups = [
  {
    title: 'Product',
    links: [
      { label: 'Install the CLI', to: '/download', external: false },
      { label: 'Agent Skills', to: '/officecli', external: false },
      { label: 'Product Docs', to: '/docs', external: false },
      { label: 'AI PPTX Generator', to: '/ai-pptx-generator', external: false },
      { label: 'DOCX Generator CLI', to: '/docx-generator-cli', external: false },
      { label: 'XLSX Reports', to: '/xlsx-report-generation', external: false },
      { label: 'Agent Automation', to: '/office-automation-ai-agents', external: false },
      { label: 'Compare Libraries', to: '/officecli-vs-python-docx-openpyxl-libreoffice', external: false },
      { label: 'Console', to: platformAppURL, external: true },
    ],
  },
  {
    title: 'Skills',
    links: [
      { label: 'Install', to: '/officecli/install', external: false },
      { label: 'Claude Code', to: '/officecli/claude-code', external: false },
      { label: 'Codex', to: '/officecli/codex', external: false },
      { label: 'OpenClaw', to: '/officecli/openclaw', external: false },
    ],
  },
  {
    title: 'Platform',
    links: [
      { label: 'Login', to: platformAppURL, external: true },
      { label: 'Billing', to: platformBillingURL, external: true },
      { label: 'Security', to: '/faq', external: false },
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
