export interface AgentSkillsRoute {
  path: string
  label: string
  title: string
  description: string
  seoTitle: string
  seoDescription: string
}

export interface WorkflowCard {
  title: string
  description: string
}

export interface InstallPath {
  title: string
  intro: string
  command: string
}

export interface RepoArtifact {
  title: string
  detail: string
}

export interface GenerationDemo {
  title: string
  type: string
  description: string
  previewSrc: string
  artifactHref: string
  artifactLabel: string
  promptHref: string
  metadataHref: string
  command: string
}

export interface FAQEntry {
  q: string
  a: string
}

export { githubRepoURL } from './siteData'
import { githubRepoURL } from './siteData'
export const agentSkillsHubPath = '/officecli'
export const legacyAgentSkillsPath = '/claude-code-codex-office-skills'
export const productDocsPath = '/docs'
export const downloadPath = '/download'

export const agentSkillsRoutes: AgentSkillsRoute[] = [
  {
    path: agentSkillsHubPath,
    label: 'Overview',
    title: 'officecli overview',
    description:
      'Understand what officecli is, which agent runtimes it supports, and which subpages cover install, Claude Code, Codex, OpenClaw, and FAQ topics.',
    seoTitle: 'officecli | Public GitHub Repository for Claude Code, Codex, and AI Agents',
    seoDescription:
      'officecli is the public GitHub repository for Claude Code, Codex, OpenClaw, and AI agent Office workflows. Explore install, runtime-specific setup, and FAQ pages for local PPTX, DOCX, XLSX, and report automation.',
  },
  {
    path: `${agentSkillsHubPath}/install`,
    label: 'Install',
    title: 'Install officecli',
    description:
      'Choose the correct installation path for Claude Code, Codex-style local agents, or OpenClaw, avoid mixing Homebrew and npm binary installs, and verify the local OfficeCLI runtime afterward.',
    seoTitle: 'Install officecli | Claude Code, Codex, and OpenClaw setup',
    seoDescription:
      'Install officecli through the Claude Code marketplace, direct Codex-style local skill install, or the OpenClaw installer, avoid mixing Homebrew and npm installs, then verify the local OfficeCLI runtime.',
  },
  {
    path: `${agentSkillsHubPath}/claude-code`,
    label: 'Claude Code',
    title: 'officecli for Claude Code',
    description:
      'Use the public marketplace repository to install the OfficeCLI plugin in Claude Code and keep Office generation on the same machine.',
    seoTitle: 'officecli for Claude Code | Marketplace install and local Office workflows',
    seoDescription:
      'Learn how officecli integrates with Claude Code through marketplace install, local OfficeCLI routing, and same-machine document generation for PPTX, DOCX, XLSX, and report tasks.',
  },
  {
    path: `${agentSkillsHubPath}/codex`,
    label: 'Codex',
    title: 'officecli for Codex',
    description:
      'Install the public skill bundle directly on Codex-style local agent hosts without relying on a marketplace layer.',
    seoTitle: 'officecli for Codex | Direct local skill install for OfficeCLI',
    seoDescription:
      'Install officecli directly on Codex-style local agents, refresh the local bundle, and route supported Office document tasks into OfficeCLI.',
  },
  {
    path: `${agentSkillsHubPath}/openclaw`,
    label: 'OpenClaw',
    title: 'officecli for OpenClaw',
    description:
      'Use the OpenClaw-oriented installer and officecli agent-bridge for structured, channel-based Office document generation.',
    seoTitle: 'officecli for OpenClaw | officecli agent-bridge setup',
    seoDescription:
      'Set up officecli for OpenClaw with the public installer, officecli agent-bridge, and local channel-based Office document workflows.',
  },
  {
    path: `${agentSkillsHubPath}/faq`,
    label: 'FAQ',
    title: 'officecli FAQ',
    description:
      'Read the most common questions about officecli, the OfficeCLI runtime, supported agent runtimes, and hosted versus local execution.',
    seoTitle: 'officecli FAQ | Public repo, local runtime, and install answers',
    seoDescription:
      'Read the officecli FAQ covering the public GitHub repository, local OfficeCLI runtime requirements, Claude Code, Codex, OpenClaw, and local-first Office generation.',
  },
]

export const agentSkillsSubpages = agentSkillsRoutes.filter((route) => route.path !== agentSkillsHubPath)
export const agentSkillsPrerenderRoutes = [agentSkillsHubPath, ...agentSkillsSubpages.map((route) => route.path), legacyAgentSkillsPath]

export const keywordChips = ['officecli', 'Claude Code', 'Codex', 'OpenClaw', 'PPTX', 'DOCX', 'XLSX', 'Report', 'IMG']

export const workflowCards: WorkflowCard[] = [
  {
    title: 'AI PPTX generation',
    description:
      'Create slide decks, proposal decks, product intros, and executive reviews from natural-language prompts while keeping generation local to the same machine.',
  },
  {
    title: 'AI DOCX drafting',
    description:
      'Draft retrospectives, proposals, memos, and customer-facing documents through an OfficeCLI skill instead of building one-off document automation scripts.',
  },
  {
    title: 'AI XLSX creation',
    description:
      'Generate spreadsheet structures, budget trackers, sales workbooks, and table-heavy outputs through the same skill surface used by agent clients.',
  },
  {
    title: 'Workbook-backed report workflows',
    description:
      'Route report generation through OfficeCLI when the workbook is the source of truth and the agent needs a local report artifact rather than a chat-only summary.',
  },
]

export const installPaths: InstallPath[] = [
  {
    title: 'Claude Code marketplace install',
    intro: 'Use the public marketplace source when Claude Code should discover the OfficeCLI skill through the plugin flow.',
    command: '/plugin marketplace add officecli/officecli\n/plugin install officecli@officecli',
  },
  {
    title: 'Codex and local agent install',
    intro: 'Use the direct installer when you want the public OfficeCLI skill files on a Codex-style local agent host without a marketplace dependency. Keep the OfficeCLI binary on its existing install channel; do not add npm on top of Homebrew.',
    command:
      'curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-skill.sh | bash -s -- officecli',
  },
  {
    title: 'OpenClaw install',
    intro:
      'Use the OpenClaw-oriented installer when the agent should generate Office files through `officecli agent-bridge` and return them to chat channels as attachments. If the binary came from Homebrew, keep that channel unless you uninstall it first.',
    command:
      'curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-openclaw-skill.sh | bash',
  },
]

export const repoArtifacts: RepoArtifact[] = [
  {
    title: 'Public GitHub repository',
    detail:
      'The repo is the public distribution surface for OfficeCLI skills, install scripts, skill docs, and marketplace wrappers.',
  },
  {
    title: 'Claude Code plugin wrappers',
    detail:
      'The `officecli` plugin targets Claude Code workflows where the agent should route Office tasks into the local OfficeCLI runtime.',
  },
  {
    title: 'Codex-compatible skill bundle',
    detail:
      'The public `skills/officecli` bundle is designed for local skill installs where Codex or similar agents can refresh the bundle and validate the host environment.',
  },
  {
    title: 'OpenClaw package',
    detail:
      'The OpenClaw-facing package keeps channel-based Office file generation on the same host through `officecli agent-bridge` instead of scraping human CLI output.',
  },
]

const demoBaseURL = `${githubRepoURL}/blob/main/demos`
const demoRawBaseURL = 'https://raw.githubusercontent.com/officecli/officecli/main/demos'

export const generationDemos: GenerationDemo[] = [
  {
    title: 'Image-rich strategy deck',
    type: 'PPTX',
    description: 'A visual strategy deck that demonstrates PPTX generation with embedded image assets and local preview sidecars.',
    previewSrc: `${demoRawBaseURL}/pptx-image-rich/preview.png`,
    artifactHref: `${demoBaseURL}/pptx-image-rich/image-rich-strategy-deck.pptx`,
    artifactLabel: 'Download PPTX',
    promptHref: `${demoBaseURL}/pptx-image-rich/prompt.md`,
    metadataHref: `${demoBaseURL}/pptx-image-rich/metadata.json`,
    command: 'officecli new pptx "Image-rich strategy deck" --prompt-file ./prompt.md --local-preview --no-publish',
  },
  {
    title: 'Text-only executive briefing',
    type: 'PPTX',
    description: 'A compact executive deck that shows the reproducible text-only path with `--no-images`.',
    previewSrc: `${demoRawBaseURL}/pptx-text-only/preview.png`,
    artifactHref: `${demoBaseURL}/pptx-text-only/text-only-executive-briefing.pptx`,
    artifactLabel: 'Download PPTX',
    promptHref: `${demoBaseURL}/pptx-text-only/prompt.md`,
    metadataHref: `${demoBaseURL}/pptx-text-only/metadata.json`,
    command: 'officecli new pptx "Text-only executive briefing" --prompt-file ./prompt.md --local-preview --no-publish --no-images',
  },
  {
    title: 'OfficeCLI customer brief',
    type: 'DOCX',
    description: 'A customer-facing brief with headings, a decision callout, and a rollout table.',
    previewSrc: `${demoRawBaseURL}/docx-brief/preview.png`,
    artifactHref: `${demoBaseURL}/docx-brief/officecli-customer-brief.docx`,
    artifactLabel: 'Download DOCX',
    promptHref: `${demoBaseURL}/docx-brief/prompt.md`,
    metadataHref: `${demoBaseURL}/docx-brief/metadata.json`,
    command: 'officecli new docx "OfficeCLI customer brief" --prompt-file ./prompt.md --local-preview --no-publish',
  },
  {
    title: 'Demo adoption dashboard',
    type: 'XLSX',
    description: 'A structured workbook showing demo coverage and readiness fields for operations tracking.',
    previewSrc: `${demoRawBaseURL}/xlsx-dashboard/preview.png`,
    artifactHref: `${demoBaseURL}/xlsx-dashboard/demo-adoption-dashboard.xlsx`,
    artifactLabel: 'Download XLSX',
    promptHref: `${demoBaseURL}/xlsx-dashboard/prompt.md`,
    metadataHref: `${demoBaseURL}/xlsx-dashboard/metadata.json`,
    command: 'officecli new xlsx "Demo adoption dashboard" --prompt-file ./prompt.md --local-preview --no-publish',
  },
  {
    title: 'Demo program readiness report',
    type: 'REPORT',
    description: 'A workbook-backed HTML report with KPI cards, findings, chart evidence, and a source XLSX file.',
    previewSrc: `${demoRawBaseURL}/report-workbook/preview.png`,
    artifactHref: `${demoBaseURL}/report-workbook/demo-program-readiness-report.html`,
    artifactLabel: 'Open HTML report',
    promptHref: `${demoBaseURL}/report-workbook/prompt.md`,
    metadataHref: `${demoBaseURL}/report-workbook/metadata.json`,
    command: 'officecli new report "Demo program readiness report" --file ./demo-program-source-workbook.xlsx --prompt-file ./prompt.md --no-publish',
  },
  {
    title: 'OfficeCLI deadline automation image',
    type: 'IMG',
    description: 'A cinematic office scene showing teams hand-writing PPT, Word, and Excel work while the foreground user generates organized outputs with OfficeCLI.',
    previewSrc: `${demoRawBaseURL}/standalone-img/preview.png`,
    artifactHref: `${demoBaseURL}/standalone-img/officecli-hero-image.png`,
    artifactLabel: 'Download PNG',
    promptHref: `${demoBaseURL}/standalone-img/prompt.md`,
    metadataHref: `${demoBaseURL}/standalone-img/metadata.json`,
    command: 'officecli new img "OfficeCLI deadline automation image" --prompt-file ./prompt.md --ratio landscape --no-publish',
  },
]

export const agentSkillsFAQs: FAQEntry[] = [
  {
    q: 'What is the public officecli repository for?',
    a: '`officecli` is the public repository that distributes skill definitions, plugin wrappers, demos, and install scripts for agent clients. The private OfficeCLI engine source lives separately.',
  },
  {
    q: 'Can Claude Code create PPTX, DOCX, XLSX, or report outputs with this repo?',
    a: 'Yes, when the OfficeCLI runtime is installed and configured locally. The public repo tells Claude Code how to route supported Office tasks into OfficeCLI instead of improvising another generation path.',
  },
  {
    q: 'Why mention Codex if this is a GitHub skills repo?',
    a: 'Because the repo is also a direct skill distribution surface for Codex-style local agents. Marketplace install is only one entrypoint; direct skill install is another.',
  },
  {
    q: 'Is this a hosted SaaS plugin backend?',
    a: 'No. The public skills repo distributes local wrappers and setup logic. Document generation still runs through the user-managed OfficeCLI runtime on the local machine.',
  },
  {
    q: 'Why create multiple officecli pages instead of one long landing page?',
    a: 'Because clear child pages give users and search engines distinct entrypoints for install, Claude Code, Codex, OpenClaw, and FAQ intent instead of forcing every query onto one generic page.',
  },
]

export const runtimeQuickLinks = [
  {
    title: 'Install',
    href: `${agentSkillsHubPath}/install`,
    description: 'Compare marketplace, direct local install, and OpenClaw setup paths.',
  },
  {
    title: 'Claude Code',
    href: `${agentSkillsHubPath}/claude-code`,
    description: 'Marketplace install and local OfficeCLI routing for Claude Code.',
  },
  {
    title: 'Codex',
    href: `${agentSkillsHubPath}/codex`,
    description: 'Direct local skill install for Codex-style agents.',
  },
  {
    title: 'OpenClaw',
    href: `${agentSkillsHubPath}/openclaw`,
    description: 'Structured bridge-based Office workflows for chat channels.',
  },
  {
    title: 'FAQ',
    href: `${agentSkillsHubPath}/faq`,
    description: 'Hosted versus local execution, repo scope, and install answers.',
  },
]

export const verificationCommands = 'officecli --version\nofficecli config status\nofficecli agent-bridge'

export function getAgentSkillsRoute(pathname: string) {
  return agentSkillsRoutes.find((route) => route.path === pathname)
}

export function getAgentSkillsBreadcrumbs(pathname: string) {
  const route = getAgentSkillsRoute(pathname)
  if (!route) {
    return []
  }

  if (route.path === agentSkillsHubPath) {
    return [
      { label: 'Home', to: '/' },
      { label: 'officecli' },
    ]
  }

  return [
    { label: 'Home', to: '/' },
    { label: 'officecli', to: agentSkillsHubPath },
    { label: route.label },
  ]
}
