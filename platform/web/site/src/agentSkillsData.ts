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

export interface FAQEntry {
  q: string
  a: string
}

export const githubRepoURL = 'https://github.com/officecli/officecli-skills'
export const agentSkillsHubPath = '/officecli-skills'
export const legacyAgentSkillsPath = '/claude-code-codex-office-skills'
export const productDocsPath = '/docs'
export const downloadPath = '/download'

export const agentSkillsRoutes: AgentSkillsRoute[] = [
  {
    path: agentSkillsHubPath,
    label: 'Overview',
    title: 'officecli-skills overview',
    description:
      'Understand what officecli-skills is, which agent runtimes it supports, and which subpages cover install, Claude Code, Codex, OpenClaw, and FAQ topics.',
    seoTitle: 'officecli-skills | Public GitHub Repository for Claude Code, Codex, and AI Agents',
    seoDescription:
      'officecli-skills is the public GitHub repository for Claude Code, Codex, OpenClaw, and AI agent Office workflows. Explore install, runtime-specific setup, and FAQ pages for local PPTX, DOCX, XLSX, and report automation.',
  },
  {
    path: `${agentSkillsHubPath}/install`,
    label: 'Install',
    title: 'Install officecli-skills',
    description:
      'Choose the correct installation path for Claude Code, Codex-style local agents, or OpenClaw and verify the local OfficeCLI runtime afterward.',
    seoTitle: 'Install officecli-skills | Claude Code, Codex, and OpenClaw setup',
    seoDescription:
      'Install officecli-skills through the Claude Code marketplace, direct Codex-style local skill install, or the OpenClaw installer, then verify the local OfficeCLI runtime.',
  },
  {
    path: `${agentSkillsHubPath}/claude-code`,
    label: 'Claude Code',
    title: 'officecli-skills for Claude Code',
    description:
      'Use the public marketplace repository to install the OfficeCLI plugin in Claude Code and keep Office generation on the same machine.',
    seoTitle: 'officecli-skills for Claude Code | Marketplace install and local Office workflows',
    seoDescription:
      'Learn how officecli-skills integrates with Claude Code through marketplace install, local OfficeCLI routing, and same-machine document generation for PPTX, DOCX, XLSX, and report tasks.',
  },
  {
    path: `${agentSkillsHubPath}/codex`,
    label: 'Codex',
    title: 'officecli-skills for Codex',
    description:
      'Install the public skill bundle directly on Codex-style local agent hosts without relying on a marketplace layer.',
    seoTitle: 'officecli-skills for Codex | Direct local skill install for OfficeCLI',
    seoDescription:
      'Install officecli-skills directly on Codex-style local agents, refresh the local bundle, and route supported Office document tasks into OfficeCLI.',
  },
  {
    path: `${agentSkillsHubPath}/openclaw`,
    label: 'OpenClaw',
    title: 'officecli-skills for OpenClaw',
    description:
      'Use the OpenClaw-oriented installer and officecli agent-bridge for structured, channel-based Office document generation.',
    seoTitle: 'officecli-skills for OpenClaw | officecli agent-bridge setup',
    seoDescription:
      'Set up officecli-skills for OpenClaw with the public installer, officecli agent-bridge, and local channel-based Office document workflows.',
  },
  {
    path: `${agentSkillsHubPath}/faq`,
    label: 'FAQ',
    title: 'officecli-skills FAQ',
    description:
      'Read the most common questions about officecli-skills, the OfficeCLI runtime, supported agent runtimes, and hosted versus local execution.',
    seoTitle: 'officecli-skills FAQ | Public repo, local runtime, and install answers',
    seoDescription:
      'Read the officecli-skills FAQ covering the public GitHub repository, local OfficeCLI runtime requirements, Claude Code, Codex, OpenClaw, and local-first Office generation.',
  },
]

export const agentSkillsSubpages = agentSkillsRoutes.filter((route) => route.path !== agentSkillsHubPath)
export const agentSkillsPrerenderRoutes = [agentSkillsHubPath, ...agentSkillsSubpages.map((route) => route.path), legacyAgentSkillsPath]

export const keywordChips = ['officecli-skills', 'Claude Code', 'Codex', 'OpenClaw', 'PPTX', 'DOCX', 'XLSX', 'Report']

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
    command: '/plugin marketplace add officecli/officecli-skills\n/plugin install officecli@officecli-skills',
  },
  {
    title: 'Codex and local agent install',
    intro: 'Use the direct installer when you want the public OfficeCLI skill files on a Codex-style local agent host without a marketplace dependency.',
    command:
      'curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-skill.sh | bash -s -- officecli',
  },
  {
    title: 'OpenClaw install',
    intro:
      'Use the OpenClaw-oriented installer when the agent should generate Office files through `officecli agent-bridge` and return them to chat channels as attachments.',
    command:
      'curl -fsSL https://raw.githubusercontent.com/officecli/officecli-skills/main/scripts/install-openclaw-skill.sh | bash',
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

export const agentSkillsFAQs: FAQEntry[] = [
  {
    q: 'What is the difference between OfficeCLI and officecli-skills?',
    a: 'OfficeCLI is the local document engine. `officecli-skills` is the public repository that distributes the skill definitions, plugin wrappers, and install scripts for agent clients.',
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
    q: 'Why create multiple officecli-skills pages instead of one long landing page?',
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
      { label: 'officecli-skills' },
    ]
  }

  return [
    { label: 'Home', to: '/' },
    { label: 'officecli-skills', to: agentSkillsHubPath },
    { label: route.label },
  ]
}
