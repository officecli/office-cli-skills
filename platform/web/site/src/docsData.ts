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
  format: 'pptx' | 'docx' | 'xlsx' | 'report' | 'img'
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
  { id: 'online-publish', label: 'Online Publish' },
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
      'Install OfficeCLI with Homebrew, npm, the Linux install script, or a manual binary. Use one install channel at a time.',
      'If you already installed OfficeCLI with Homebrew, keep using Homebrew and do not install the npm wrapper on top of it.',
      'To intentionally switch from Homebrew to npm, run `brew uninstall officecli/homebrew-officecli/officecli` first; if your install uses the short formula name, run `brew uninstall officecli`, then run `npm install -g officecli`.',
      'Run `officecli --version` to confirm the binary is on your PATH.',
    ],
  },
  {
    title: 'Generate immediately',
    items: [
      'Hosted trial access is the default, so first-run document commands do not require a local model endpoint or a config file.',
      'Run `officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review. Cover growth, retention, risks, and next-quarter actions."`.',
      'Run `officecli new docx "Product Launch Brief" --prompt "Write a concise launch brief with audience, positioning, timeline, risks, and next steps."`.',
      'Run `officecli new xlsx "Sales Pipeline" --prompt "Create a sales pipeline workbook with stages, owners, deal values, probability, and next action columns."`.',
    ],
  },
  {
    title: 'Check access or go advanced',
    items: [
      'Run `officecli auth status` to check hosted trial, hosted key, or External Mode access.',
      'When the hosted trial is used up, create or purchase a hosted key and run `officecli auth set-key <api-key>`.',
      'For External Mode, run `officecli config set-runtime external` and `officecli config set-generation` to configure your own LLM endpoint. External Mode is free and unlimited.',
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
    detail: 'Refreshes the npm wrapper and matching binary. Do not use this on top of a Homebrew install; uninstall Homebrew first if switching channels.',
  },
  {
    label: 'Linux install script',
    command: 'curl -fsSL https://raw.githubusercontent.com/officecli/officecli-dist/main/scripts/install-officecli.sh | bash',
    detail: 'Re-runs the public installer and fetches the latest stable release.',
  },
]

export const uninstallMethods: CommandExample[] = [
  {
    label: 'Homebrew',
    command: 'brew uninstall officecli/homebrew-officecli/officecli',
    detail: 'Use when OfficeCLI was installed from the official tap. If Homebrew reports the short formula name, run `brew uninstall officecli` instead.',
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
    summary: 'Check whether the current machine is using External Mode or Hosted Mode, or save a hosted API key.',
    examples: [
      {
        label: 'Check current access mode',
        command: 'officecli auth status',
        detail: 'Shows runtime mode, hosted credit status, and access configuration.',
      },
      {
        label: 'Save a hosted key',
        command: 'officecli auth set-key cop_live_xxx',
        detail: 'Routes Hosted Mode generation requests through the hosted credits linked to that key. External Mode remains free and unlimited.',
      },
    ],
  },
  {
    title: 'Generate new files',
    command: 'officecli new <pptx|docx|xlsx|report|img> <topic> [brief]',
    summary: 'Create a new PPTX, DOCX, XLSX, workbook-backed HTML report, or standalone IMG output from a direct prompt or a prompt file.',
    notes: [
      '`--prompt` takes highest precedence when you want to provide the full brief directly in the command.',
      '`--prompt-file` is the best fit for longer structured prompts or reusable prompt templates.',
      '`report` requires `--file <xlsx-path>` because the workbook is the source of truth for the report output.',
      '`img` uses your configured image provider in External Mode and the OfficeCLI-managed runtime in Hosted Mode. It supports `--ratio`, an explicit `--size <WxH>`, and one or more `--reference-image` inputs, and publishes online previews by default when publishing is configured.',
    ],
    examples: [
      {
        label: 'PPT with a direct prompt',
        command: 'officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review. Cover growth, retention, risks, and next-quarter actions."',
        detail: 'Use `--no-images` if you want a text-only presentation.',
      },
      {
        label: 'DOCX with a direct prompt',
        command: 'officecli new docx "Product Launch Brief" --prompt "Write a concise launch brief with audience, positioning, timeline, risks, and next steps."',
        detail: 'Best for narrative first drafts that need a clear editable structure.',
      },
      {
        label: 'XLSX from a direct prompt',
        command: 'officecli new xlsx "Sales Pipeline" --prompt "Create a sales pipeline workbook with stages, owners, deal values, probability, and next action columns."',
        detail: 'Be explicit about sheets, headers, and sample-data constraints.',
      },
      {
        label: 'Workbook-backed report',
        command: 'officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt "Summarize regional revenue shifts, efficiency signals, and the board-level decisions this workbook implies."',
        detail: 'Produces a local HTML report based on workbook data.',
      },
      {
        label: 'Standalone image',
        command: 'officecli new img "Launch Visual" --prompt "A polished product launch hero image" --ratio landscape --reference-image ./reference.png',
        detail: 'Saves one local PNG. Add `--no-publish` for local-only output, repeat `--reference-image` for multiple references, or use `--size <WxH>` to override the ratio mapping.',
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
  {
    title: 'Anchor standalone IMG prompts with subject, mood, and constraints',
    detail: 'For `officecli new img`, lead with subject, scene, mood, and palette, and explicitly state aspect ratio or `--size`. Use repeated `--reference-image` flags to ground style instead of relying on prose alone.',
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
  {
    title: 'IMG: launch hero visual',
    format: 'img',
    directCommand: 'officecli new img "Launch Visual" --prompt "A polished product launch hero image for an enterprise collaboration platform" --ratio landscape --reference-image ./brand-keyframe.png',
    fileCommand: 'officecli new img "Launch Visual" --prompt-file ./docs/img-launch-prompt.txt --size 1280x720 --reference-image ./brand-keyframe.png --reference-image ./brand-mark.png',
    promptFileName: 'docs/img-launch-prompt.txt',
    prompt: `Create a launch hero image for an enterprise collaboration platform:\n\n- Aspect: landscape, 16:9\n- Mood: confident, modern, slightly cinematic\n- Subject: abstract collaboration motif with a soft horizon glow\n- Palette: deep navy and aqua highlights to match the brand reference\n- Avoid embedded text or hard logos so the image can be reused as a hero plate`,
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
      'Marketplace repository target: `officecli/officecli`.',
      'Install the general Office skill with `/plugin install officecli@officecli` after adding the marketplace.',
      'Install the OpenClaw-oriented variant with `/plugin install openclaw-officecli@officecli` when you want the OpenClaw package as well.',
    ],
  },
]

export const usageRules: UsageRule[] = [
  {
    title: 'External Mode is free and unlimited',
    detail: 'External Mode uses your own model endpoint and works without a hosted API key or OfficeCLI usage quota.',
  },
  {
    title: 'External and Hosted Mode share one binary',
    detail: 'PPTX, DOCX, XLSX, REPORT, and IMG generation all run from the same officecli binary. External IMG uses your local image provider; Hosted IMG uses OfficeCLI-managed runtime.',
  },
  {
    title: 'Hosted usage is API-key based',
    detail: 'Hosted users consume hosted credits through API keys managed by the platform. Checkout sells hosted credits only.',
  },
  {
    title: 'Hosted credits are consumed only after successful generation',
    detail: 'Availability is checked before Hosted Mode generation, but hosted credits are spent only after the document or image is generated successfully.',
  },
  {
    title: 'Platform and CLI surfaces reflect runtime status',
    detail: 'The CLI, app, and admin surfaces show hosted credits, usage events, orders, and growth credit grants while External Mode remains free and unlimited.',
  },
  {
    title: 'Invite rewards are available with current limits',
    detail: 'Invite codes, referral progress, and hosted credit grants are visible today. Each activated referral grants 20 hosted credits up to the current limit.',
  },
]

export const inviteRewardSteps: TipGroup[] = [
  {
    title: 'Find your invite code in the app',
    detail: 'Open the platform Overview page and look at Invite Credits. That card shows the invite code currently attached to your account.',
  },
  {
    title: 'Share the invite-bearing login link',
    detail: 'A referral is registered only after the invited user finishes the Google login flow through a link that carries your invite code.',
  },
  {
    title: 'Wait for activation before expecting quota',
    detail: 'Registration and activation are different states. Hosted credits are granted only after the invited account completes its first successful generation.',
  },
]

export const inviteRewardRules: UsageRule[] = [
  {
    title: 'Each account can invite up to 5 users',
    detail: 'The current backend limit is five captured referrals per inviter account.',
  },
  {
    title: 'Each activated referral adds 20 hosted credits',
    detail: 'Hosted credits increase only when the referral becomes activated, not when the invite link is merely shared.',
  },
  {
    title: 'Current app access policy still applies',
    detail: 'The invited user must finish Google sign-in and satisfy the current app access policy before the referral flow can continue.',
  },
  {
    title: 'Use app surfaces to verify progress',
    detail: 'Overview shows Invite Credits, Referral Progress, and the referral timeline. Admin growth surfaces show hosted credit grant detail and current reward state.',
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
      'Use hosted credit grant detail to confirm the invite credits that landed on the account.',
    ],
  },
]

export const troubleshootingTips: TipGroup[] = [
  {
    title: 'Online preview publishing',
    detail: 'Online preview publishing is what lets stakeholders open the generated file from a shareable, password-protected URL — without you uploading or hosting anything else. It is the differentiating share path that other terminal-first AI document CLIs do not ship out of the box.',
  },
  {
    title: 'PPT generated without images',
    detail: 'Run `officecli config set-generation` and verify the image model URL, credentials, and model name. Use `--no-images` when a text-only deck is intentional.',
  },
  {
    title: 'Standalone `new img` fails or skips quota',
    detail: 'External Mode standalone images use your local `config set-generation` image provider and do not consume OfficeCLI hosted credits. Hosted Mode images require `officecli config set-license` and spend hosted credits through the OfficeCLI-managed runtime.',
  },
  {
    title: 'No image preview link returned',
    detail: 'Online image previews need `officecli config set-publish`. Standalone images publish by default once publishing is configured; pass `--no-publish` if you want local-only output.',
  },
  {
    title: 'Publish link did not appear',
    detail: 'Run `officecli config set-publish` to turn on one-command online publish, or set `OFFICE_CLI_DEFAULT_PUBLISH=true`. If publishing is configured but the URL is missing, run `officecli config status` to verify the publish endpoint and license API key, and confirm the run did not include `--no-publish` and did not set `OFFICE_CLI_DEFAULT_PUBLISH=false`.',
  },
  {
    title: 'Report generation failed immediately',
    detail: 'The `report` format requires `--file <xlsx-path>`. The workbook is the source of truth for the generated HTML report.',
  },
  {
    title: 'Access or quota behavior looks wrong',
    detail: 'Run `officecli auth status` and confirm whether the machine is using External Mode or Hosted Mode. If you expect Hosted Mode usage, save the correct hosted key with `officecli auth set-key`.',
  },
]

export interface PublishGuide {
  intro: string
  highlights: TipGroup[]
  configCommands: CommandExample[]
  envVars: Array<{ name: string; detail: string }>
  outputShape: string[]
}

export const publishGuide: PublishGuide = {
  intro:
    'One-command online publish is the differentiating share path of OfficeCLI. After every successful generation, OfficeCLI calls the OfficeCLI publish service, returns a shareable `officecli.io/p/<id>` URL, and prints an auto-generated access password protecting the preview — no extra hosting, gateway, or upload step. Other terminal-first AI document CLIs stop at a local file.',
  highlights: [
    {
      title: 'On by default once configured',
      detail: 'After `officecli config set-publish`, every `officecli new` run for documents and standalone images publishes by default. The default can be flipped per machine by editing `set-defaults` or by setting `OFFICE_CLI_DEFAULT_PUBLISH=false`.',
    },
    {
      title: 'Per-command override',
      detail: 'Add `--no-publish` to any single command to keep that run fully local. The flag wins over the configured default and over `OFFICE_CLI_DEFAULT_PUBLISH`.',
    },
    {
      title: 'License-aware credential reuse',
      detail: 'If `set-publish` is enabled but no dedicated publish API key is set, OfficeCLI reuses the license API key configured by `officecli config set-license`, so most users only have to authenticate once.',
    },
    {
      title: 'Password-protected by default',
      detail: 'Every preview returns an auto-generated access password printed alongside the Preview URL. Treat the URL plus password as a single share unit — that is the package you hand to a reviewer.',
    },
    {
      title: 'Works for documents and standalone images',
      detail: 'PPTX, DOCX, XLSX, REPORT, and standalone IMG outputs all flow through the same publish path. Standalone `new img` publishes by default whenever publishing is configured.',
    },
  ],
  configCommands: [
    {
      label: 'Turn publish on for this machine',
      command: 'officecli config set-publish',
      detail: 'Configures the publish endpoint and credential. Subsequent `officecli new ...` runs will publish by default.',
    },
    {
      label: 'Inspect current publish state',
      command: 'officecli config status',
      detail: 'Shows whether publish is enabled, the publish base URL, and whether a publish API key or license key will be used to authenticate.',
    },
    {
      label: 'Generate and skip publish for one run',
      command: 'officecli new pptx "Internal Draft" --prompt-file ./brief.md --no-publish',
      detail: 'Use `--no-publish` for sensitive drafts or strict offline workflows; this run will only write the local file.',
    },
    {
      label: 'Generate and force publish on a single run',
      command: 'officecli new docx "Customer Proposal" --prompt-file ./proposal.md --publish',
      detail: 'Use `--publish` when the machine default is off but you want this specific run to produce a shareable preview URL.',
    },
  ],
  envVars: [
    {
      name: 'OFFICE_CLI_PUBLISH_ENABLED',
      detail: 'Force-enable or disable the publish channel without rerunning `set-publish`. Useful in CI images.',
    },
    {
      name: 'OFFICE_CLI_DEFAULT_PUBLISH',
      detail: 'Flip the default publish behavior of `officecli new` for documents. `--publish` and `--no-publish` still override per command.',
    },
    {
      name: 'OFFICE_CLI_PUBLISH_BASE_URL',
      detail: 'Override the publish service base URL when running against a staging or self-hosted publish gateway.',
    },
    {
      name: 'OFFICE_CLI_PUBLISH_API_KEY',
      detail: 'Use a dedicated publish API key. If unset, OfficeCLI falls back to the license API key configured by `set-license`.',
    },
    {
      name: 'OFFICE_CLI_PUBLISH_TIMEOUT_SEC',
      detail: 'Raise the publish HTTP timeout for large outputs or slower networks.',
    },
  ],
  outputShape: [
    '$ officecli new pptx "Q3 Business Review" --prompt-file ./brief.md',
    'Generation completed. Saved to output/Q3_BUSINESS_REVIEW.PPTX',
    'Preview URL: https://officecli.io/p/xyz123; password: abcdef',
    'Access: External Mode; free unlimited with your model endpoint',
  ],
}

export const docsLinks = {
  platformAppURL,
  platformQuotaURL: `${platformAppURL}/quota`,
  platformBillingURL,
  inviteLinkTemplate: `${platformAppURL}/login?invite=<your-invite-code>`,
  openClawInstallCommand: 'bash ./scripts/install-openclaw-skill.sh',
  codexBridgeCommand: 'officecli agent-bridge',
}
