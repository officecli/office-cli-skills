import { platformAppURL, platformBillingURL } from './siteData'

export interface DocsSectionLink {
  id: string
  label: string
}

export interface DocsChecklist {
  title: string
  items: string[]
}

export interface CommandExample {
  label: string
  command: string
  detail: string
}

export interface CommandGroup {
  title: string
  command: string
  summary: string
  notes?: string[]
  examples?: CommandExample[]
}

export interface PromptExample {
  title: string
  format: 'pptx' | 'docx' | 'xlsx' | 'report'
  directCommand: string
  fileCommand: string
  promptFileName: string
  prompt: string
}

export interface UsageRule {
  title: string
  detail: string
}

export interface TipGroup {
  title: string
  detail: string
}

export const docsSections: DocsSectionLink[] = [
  { id: 'quickstart', label: 'Quickstart' },
  { id: 'install-update-uninstall', label: 'Install / Update / Uninstall' },
  { id: 'command-reference', label: 'Command Reference' },
  { id: 'prompting-tips', label: 'Prompting Tips' },
  { id: 'prompt-cookbook', label: 'Prompt Cookbook' },
  { id: 'agents', label: 'Use With Agents' },
  { id: 'openclaw', label: 'Use With OpenClaw' },
  { id: 'pricing-rules', label: 'Pricing & Usage Rules' },
  { id: 'invite-rewards', label: 'Invite Rewards' },
  { id: 'troubleshooting', label: 'Troubleshooting' },
]

export const quickstartChecklist: DocsChecklist[] = [
  {
    title: 'Install and verify',
    items: [
      'Install OfficeCLI with Homebrew, npm, the Linux install script, or a manual binary.',
      'Run `officecli --version` to confirm the binary is on your PATH.',
    ],
  },
  {
    title: 'Configure generation and access',
    items: [
      'Run `officecli config set-generation` to configure your LLM endpoint.',
      'Run `officecli config set-license` to enable free or paid access checks.',
      'Run `officecli config set-publish` only if you want online preview publishing.',
    ],
  },
  {
    title: 'Generate the first file',
    items: [
      'Use `officecli new pptx|docx|xlsx|report ...` for file generation.',
      'For `pptx`, images are enabled by default when the plan benefits from visuals; add `--no-images` for a text-only deck.',
    ],
  },
]

export const updateMethods: CommandExample[] = [
  {
    label: 'Portable default',
    command: 'officecli upgrade',
    detail: 'Checks the current installation channel and applies the supported update path when automatic upgrades are available.',
  },
  {
    label: 'Homebrew',
    command: 'brew upgrade officecli/homebrew-officecli/officecli',
    detail: 'Best match when the original install used Homebrew.',
  },
  {
    label: 'npm',
    command: 'npm install -g officecli',
    detail: 'Refreshes the npm wrapper and the matching OfficeCLI binary.',
  },
  {
    label: 'Linux install script',
    command: 'curl -fsSL https://raw.githubusercontent.com/officecli/officecli/main/scripts/install-officecli.sh | bash',
    detail: 'Re-runs the public installer and fetches the latest stable release.',
  },
]

export const uninstallMethods: CommandExample[] = [
  {
    label: 'Homebrew',
    command: 'brew uninstall officecli/homebrew-officecli/officecli',
    detail: 'Use when OfficeCLI was installed from the official tap.',
  },
  {
    label: 'npm',
    command: 'npm uninstall -g officecli',
    detail: 'Removes the npm wrapper command from your global package manager.',
  },
  {
    label: 'Script / manual binary',
    command: 'rm -f ~/.local/bin/officecli',
    detail: 'Removes the common local binary path used by the public installer and skill repair scripts.',
  },
]

export const commandGroups: CommandGroup[] = [
  {
    title: 'Configuration',
    command: 'officecli config status | set-generation | set-license | set-publish | set-defaults',
    summary: 'View current configuration or update generation, license, publish, and default output settings.',
    examples: [
      {
        label: 'Inspect current config',
        command: 'officecli config status',
        detail: 'Useful before debugging missing images, auth, or publish behavior.',
      },
      {
        label: 'Configure the generation provider',
        command: 'officecli config set-generation',
        detail: 'Sets the text model and, when needed, the image model for PPT generation.',
      },
    ],
  },
  {
    title: 'Access and quota status',
    command: 'officecli auth status | set-key <api-key>',
    summary: 'Check whether the current machine is using free or paid access, or save a paid API key.',
    examples: [
      {
        label: 'Check current access mode',
        command: 'officecli auth status',
        detail: 'Shows free or paid status together with remaining quota information.',
      },
      {
        label: 'Save a paid key',
        command: 'officecli auth set-key cop_live_xxx',
        detail: 'Routes future generation requests through the paid document-generation quota linked to that key.',
      },
    ],
  },
  {
    title: 'Generate new files',
    command: 'officecli new <pptx|docx|xlsx|report> <topic> [brief]',
    summary: 'Create a new PPTX, DOCX, XLSX, or workbook-backed HTML report from a direct prompt or a prompt file.',
    notes: [
      '`--prompt` takes highest precedence when you want to provide the full brief directly in the command.',
      '`--prompt-file` is the best fit for longer structured prompts or reusable prompt templates.',
      '`report` requires `--file <xlsx-path>` because the workbook is the source of truth for the report output.',
    ],
    examples: [
      {
        label: 'PPT with a direct prompt',
        command: 'officecli new pptx "Enterprise Collaboration Platform" --prompt "Create a six-slide executive deck for mid-market and enterprise buyers. Focus on collaboration workflows, permission control, knowledge retention, and security posture."',
        detail: 'Use `--no-images` if you want a text-only presentation.',
      },
      {
        label: 'DOCX from a prompt file',
        command: 'officecli new docx "Quarterly Review" --prompt-file ./examples/docx-prompt.txt',
        detail: 'Best for longer narrative instructions and reusable document patterns.',
      },
      {
        label: 'XLSX from a direct prompt',
        command: 'officecli new xlsx "Sales Analysis" --prompt "Generate a quarterly sales analysis workbook with summary and regional sheets. Include region, revenue, year-over-year growth, owner, and target attainment."',
        detail: 'Be explicit about sheets, headers, and sample-data constraints.',
      },
      {
        label: 'Workbook-backed report',
        command: 'officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt "Summarize regional revenue shifts, efficiency signals, and the board-level decisions this workbook implies."',
        detail: 'Produces a local HTML report based on workbook data.',
      },
    ],
  },
  {
    title: 'Scoring and review',
    command: 'officecli score pptx <file> | officecli review pptx <file>',
    summary: 'Run structural and optional visual review on demand for an existing PPTX file.',
    notes: [
      'Scoring does not run automatically after generation.',
      'If LibreOffice is available, review can include a PDF-based visual pass unless `--no-visual` is set.',
    ],
    examples: [
      {
        label: 'Standard deck review',
        command: 'officecli review pptx ./output/Enterprise-Collaboration-Platform.pptx',
        detail: 'Compatibility alias for `score` remains available.',
      },
      {
        label: 'Structural checks only',
        command: 'officecli score pptx ./output/Enterprise-Collaboration-Platform.pptx --no-visual --fail-below 80',
        detail: 'Useful in CI or when LibreOffice is not installed.',
      },
    ],
  },
  {
    title: 'Upgrades and agent bridge',
    command: 'officecli upgrade | officecli agent-bridge',
    summary: 'Upgrade the CLI or expose the structured JSON-RPC interface used by agent integrations.',
    notes: [
      '`agent-bridge` is the preferred protocol surface for agents; do not parse human CLI spinner output as a machine protocol.',
      'Use `capabilities/get` to discover image support and update availability when building an agent client.',
    ],
    examples: [
      {
        label: 'Check for updates',
        command: 'officecli upgrade',
        detail: 'Shows the suggested update command when automatic upgrades are not available.',
      },
      {
        label: 'Start the bridge locally',
        command: 'officecli agent-bridge',
        detail: 'Starts a JSON-RPC 2.0 over stdio endpoint for agent clients such as Codex or OpenClaw.',
      },
    ],
  },
]

export const promptingTips: TipGroup[] = [
  {
    title: 'State the audience and outcome first',
    detail: 'Lead with who the document is for, what decision or action it should enable, and what tone it should use. This consistently improves business-facing output.',
  },
  {
    title: 'Control structure explicitly',
    detail: 'For PPTX, give slide count and slide flow. For DOCX, list required sections. For XLSX, define sheets and headers. For reports, describe the business questions the workbook should answer.',
  },
  {
    title: 'Use direct prompts for short briefs, prompt files for reusable playbooks',
    detail: 'Short prompts work well inline with `--prompt`. Longer narrative instructions, templates, or team-standard prompts are easier to review and version with `--prompt-file`.',
  },
  {
    title: 'Add style and audience flags when format matters',
    detail: 'Use `--style`, `--audience`, and `--lang` when the document must match a brand, role, or locale. Use `--mode best` when you want more iterative quality over speed.',
  },
  {
    title: 'Turn images off when clarity matters more than visuals',
    detail: 'For executive text-only decks, compliance decks, or environments without a configured image model, add `--no-images` to keep the output deterministic.',
  },
]

export const promptExamples: PromptExample[] = [
  {
    title: 'PPTX: product introduction deck',
    format: 'pptx',
    directCommand: 'officecli new pptx "Enterprise Collaboration Platform" --prompt "Create a six-slide executive deck for mid-size and large enterprise buyers. Cover product positioning, collaboration workflows, permission control, knowledge retention, security, and the business case for adoption."',
    fileCommand: 'officecli new pptx "Enterprise Collaboration Platform" --prompt-file ./examples/prompt.txt',
    promptFileName: 'examples/prompt.txt',
    prompt: `Please generate a PPT introducing an enterprise collaboration platform:\n\n- Target mid-size and large enterprise customers\n- Focus on collaboration features, permission management, knowledge retention, and security\n- Keep the tone professional, restrained, and conclusion-first\n- Limit the deck to 6 slides or fewer\n- Use this structure: cover, product positioning, core capabilities, business value, security and governance, summary`,
  },
  {
    title: 'PPTX: quarterly business review',
    format: 'pptx',
    directCommand: 'officecli new pptx "SaaS Quarterly Business Review" --prompt "Create a seven-slide QBR for the board. Summarize growth, retention, efficiency, regional momentum, top risks, and next-quarter actions. Keep the tone analytical and concise."',
    fileCommand: 'officecli new pptx "SaaS Quarterly Business Review" --prompt-file ./docs/qbr-prompt.txt',
    promptFileName: 'docs/qbr-prompt.txt',
    prompt: `Create a board-ready quarterly business review deck:\n\n- Audience: board members and executive staff\n- Keep it under 7 slides\n- Include these sections: headline performance, regional shifts, customer efficiency, major risks, and next-quarter actions\n- Prefer conclusion-first slide titles and short, defensible bullets\n- If visuals are used, keep them simple and presentation-ready`,
  },
  {
    title: 'DOCX: retrospective draft',
    format: 'docx',
    directCommand: 'officecli new docx "Quarterly Review" --prompt "Write a quarterly project retrospective for leadership. Cover background, goals, outcomes, issues, lessons learned, and the next-step plan in a direct, professional tone."',
    fileCommand: 'officecli new docx "Quarterly Review" --prompt-file ./examples/docx-prompt.txt',
    promptFileName: 'examples/docx-prompt.txt',
    prompt: `Please generate a quarterly project retrospective Word document:\n\n- Target company leadership\n- Include these sections: background, goals, outcomes, issues, lessons learned, next-step plan\n- Keep the tone professional, direct, and free of filler\n- Emphasize key results and actionable improvement recommendations\n- Make it suitable as a first internal retrospective draft`,
  },
  {
    title: 'DOCX: customer proposal draft',
    format: 'docx',
    directCommand: 'officecli new docx "Customer Proposal" --prompt "Draft a customer-facing proposal for a workflow automation rollout. Include executive summary, current pain points, proposed approach, implementation phases, risks, and commercial next steps."',
    fileCommand: 'officecli new docx "Customer Proposal" --prompt-file ./docs/customer-proposal-prompt.txt',
    promptFileName: 'docs/customer-proposal-prompt.txt',
    prompt: `Draft a customer proposal document:\n\n- Audience: operations and IT stakeholders at a prospective customer\n- Keep the tone persuasive, concrete, and implementation-aware\n- Include: executive summary, current-state challenges, proposed solution, rollout phases, delivery assumptions, risks, and success measures\n- Make it suitable as a first proposal draft for account teams to refine`,
  },
  {
    title: 'XLSX: sales analysis workbook',
    format: 'xlsx',
    directCommand: 'officecli new xlsx "Sales Analysis" --prompt "Generate a quarterly sales analysis workbook with a summary sheet and a regional analysis sheet. Include region, revenue, year-over-year growth, owner, and target attainment with plausible demo data."',
    fileCommand: 'officecli new xlsx "Sales Analysis" --prompt-file ./examples/xlsx-prompt.txt',
    promptFileName: 'examples/xlsx-prompt.txt',
    prompt: `Please generate an Excel workbook for sales analysis:\n\n- Include at least one summary sheet and one regional analysis sheet\n- Recommended fields: region, sales, year-over-year growth, owner, target attainment\n- Use business-plausible sample data suitable for demos\n- Keep headers clear for later editing\n- Make it suitable for quarterly management review`,
  },
  {
    title: 'XLSX: project budget tracker',
    format: 'xlsx',
    directCommand: 'officecli new xlsx "Project Budget Tracker" --prompt "Create a budget workbook with summary, department detail, and variance sheets. Include monthly budget, actuals, variance, owner, and status columns for a cross-functional program."',
    fileCommand: 'officecli new xlsx "Project Budget Tracker" --prompt-file ./docs/budget-tracker-prompt.txt',
    promptFileName: 'docs/budget-tracker-prompt.txt',
    prompt: `Create an Excel workbook for project budget tracking:\n\n- Include a summary sheet, a department detail sheet, and a variance sheet\n- Required columns: category, owner, monthly budget, actual spend, variance, status\n- Use realistic but fictional sample values\n- Keep formulas simple and headers explicit for later editing`,
  },
  {
    title: 'Report: workbook-to-board summary',
    format: 'report',
    directCommand: 'officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt "Summarize regional revenue shifts, margin pressure, and the board-level decisions implied by this workbook."',
    fileCommand: 'officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt-file ./docs/report-board-prompt.txt',
    promptFileName: 'docs/report-board-prompt.txt',
    prompt: `Generate a workbook-backed business review report:\n\n- Use the workbook as the source of truth\n- Summarize the most important regional changes, efficiency signals, and decision points\n- Keep the narrative suitable for board or investor review\n- Every section should connect findings to an action or decision`,
  },
]

export const agentSkillHighlights: DocsChecklist[] = [
  {
    title: 'Codex',
    items: [
      'Use the `officecli` skill for Office-document tasks such as PPTX, DOCX, XLSX, or report generation.',
      'Run `fix-officecli-env.sh` and `check-officecli-env.sh` before generation tasks so the local skill bundle and binary state stay valid.',
      'Prefer `officecli agent-bridge` over scraping human CLI output when the client is acting as an agent.',
    ],
  },
  {
    title: 'Claude Code',
    items: [
      'Marketplace repository target: `officecli/officecli-skills`.',
      'Install the general Office skill with `/plugin install officecli@officecli-skills` after adding the marketplace.',
      'Install the OpenClaw-oriented variant with `/plugin install openclaw-officecli@officecli-skills` when you want the OpenClaw package as well.',
    ],
  },
]

export const usageRules: UsageRule[] = [
  {
    title: 'Anonymous users can use a free quota',
    detail: 'Free access is tracked by machine fingerprint and works without a paid API key.',
  },
  {
    title: 'Paid usage is API-key based',
    detail: 'Paid users consume purchased document-generation quota through API keys managed by the platform.',
  },
  {
    title: 'Quota is consumed only after successful generation',
    detail: 'Availability is checked before generation, but quota is spent only after the document is generated successfully.',
  },
  {
    title: 'Platform and CLI surfaces reflect quota status',
    detail: 'The CLI, app, and admin surfaces are expected to show the current free or paid quota state.',
  },
  {
    title: 'Invite rewards are available with current limits',
    detail: 'Invite codes, referral progress, and reward quota are visible today, but Discord reward automation and attribution analytics are still partial.',
  },
]

export const inviteRewardSteps: TipGroup[] = [
  {
    title: 'Find your invite code in the app',
    detail: 'Open the platform Overview page and look at Reward Quota. That card shows the invite code currently attached to your account.',
  },
  {
    title: 'Share the invite-bearing login link',
    detail: 'A referral is registered only after the invited user finishes the Google login flow through a link that carries your invite code.',
  },
  {
    title: 'Wait for activation before expecting quota',
    detail: 'Registration and activation are different states. Bonus generations are granted only after the invited account completes its first successful generation.',
  },
]

export const inviteRewardRules: UsageRule[] = [
  {
    title: 'Each account can invite up to 5 users',
    detail: 'The current backend limit is five captured referrals per inviter account.',
  },
  {
    title: 'Each activated referral adds 10 bonus generations',
    detail: 'Reward quota increases only when the referral becomes activated, not when the invite link is merely shared.',
  },
  {
    title: 'Google allowlist access still applies',
    detail: 'The invited user must sign in with an allowlisted Google account or the login will be rejected before the referral flow can continue.',
  },
  {
    title: 'Use app surfaces to verify progress',
    detail: 'Overview shows Reward Quota, Referral Progress, and the referral timeline. Quota shows reward grant detail and current remaining reward balance.',
  },
]

export const inviteRewardChecklists: DocsChecklist[] = [
  {
    title: 'How to share the flow',
    items: [
      'Copy your invite code from the app Overview page.',
      'Replace the placeholder in the invite link template with that code.',
      'Send the finished link to the teammate you want to invite.',
      'Ask the invited user to complete Google login through that exact link.',
    ],
  },
  {
    title: 'How to verify the result',
    items: [
      'Use Referral Progress to see how many invite slots are still available.',
      'Use the referral timeline to distinguish registered, activated, and reward-granted states.',
      'Use Reward grant detail on the Quota page to confirm the bonus quota that landed on the account.',
    ],
  },
]

export const troubleshootingTips: TipGroup[] = [
  {
    title: 'PPT generated without images',
    detail: 'Run `officecli config set-generation` and verify the image model URL, credentials, and model name. Use `--no-images` when a text-only deck is intentional.',
  },
  {
    title: 'Publish link did not appear',
    detail: 'Online preview publishing is optional. Run `officecli config set-publish` if you want published previews, or use `--no-publish` for fully local output.',
  },
  {
    title: 'Report generation failed immediately',
    detail: 'The `report` format requires `--file <xlsx-path>`. The workbook is the source of truth for the generated HTML report.',
  },
  {
    title: 'Access or quota behavior looks wrong',
    detail: 'Run `officecli auth status` and confirm whether the machine is in free mode or using a paid API key. If you expect paid usage, save the correct key with `officecli auth set-key`.',
  },
]

export const docsLinks = {
  platformAppURL,
  platformQuotaURL: `${platformAppURL}/quota`,
  platformBillingURL,
  inviteLinkTemplate: `${platformAppURL}/login?invite=<your-invite-code>`,
  openClawInstallCommand: 'bash ./scripts/install-openclaw-skill.sh',
  codexBridgeCommand: 'officecli agent-bridge',
}
