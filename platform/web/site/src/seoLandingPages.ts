export interface SEOLandingFAQ {
  q: string
  a: string
}

export interface SEOLandingPage {
  path: string
  navLabel: string
  title: string
  seoTitle: string
  seoDescription: string
  eyebrow: string
  intro: string
  command: string
  sections: Array<{
    title: string
    body: string[]
  }>
  bestFor: string[]
  notFor: string[]
  faqs: SEOLandingFAQ[]
}

export const seoLandingPages: SEOLandingPage[] = [
  {
    path: '/ai-pptx-generator',
    navLabel: 'AI PPTX Generator',
    title: 'AI PPTX Generator for Agent Workflows',
    seoTitle: 'AI PPTX Generator | Create editable decks from agent workflows',
    seoDescription:
      'Use OfficeCLI as an AI PPTX generator for agent workflows. Generate editable PowerPoint decks with External Mode, Hosted Mode, images, review, and one-command online preview publishing.',
    eyebrow: 'PPTX automation',
    intro:
      'OfficeCLI turns a structured prompt into an editable PPTX deck from the terminal, a CI job, or an agent task. It is built for developers who need repeatable presentation generation without building a slide renderer, storage gateway, or publish workflow from scratch.',
    command:
      'officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review. Cover growth, retention, risks, and next-quarter actions."',
    sections: [
      {
        title: 'Generate decks where the agent already works',
        body: [
          'Most AI presentation workflows break when they leave chat and need a real .pptx file. OfficeCLI keeps the handoff close to the agent. A planner, coding assistant, operations bot, or build job can call one binary, pass a brief, and receive an editable deck that opens in PowerPoint, Keynote, or compatible office suites.',
          'The generator supports PPTX as a first-class output next to DOCX, XLSX, workbook-backed REPORT, and standalone IMG generation. That matters when a workflow needs more than slides: the same automation can draft the deck, create a workbook, turn the workbook into a report, and publish the resulting artifact through the same OfficeCLI preview channel.',
        ],
      },
      {
        title: 'External Mode and Hosted Mode',
        body: [
          'External Mode is free and unlimited when you bring your own LLM endpoint. Teams that already run a model gateway can keep prompts, routing, and spend policy inside their existing environment while still using OfficeCLI for document assembly and packaging.',
          'Hosted Mode uses hosted credits for the OfficeCLI-managed runtime. It is useful when a developer wants the quickest path to a working deck generator, or when an internal tool should avoid asking every user to configure a model endpoint before creating the first presentation.',
        ],
      },
      {
        title: 'Images, review, and preview publishing',
        body: [
          'PPTX generation can include images by default when the plan benefits from visuals. Add `--no-images` for a text-only deck. After generation, OfficeCLI can review local PPTX files, run structural checks, and use LibreOffice for a stronger visual review pass when it is available.',
          'The publish path is the main difference from a one-off slide script. A successful run can return a password-protected `officecli.io/p/<id>` preview URL in the same command. Agents can hand the user a link for inspection while keeping the editable PPTX available as a local file.',
        ],
      },
      {
        title: 'Operationalize PPTX generation',
        body: [
          'For production use, keep prompts explicit about audience, slide count, decision context, and any sections that must appear in the deck. Store longer prompts in files so agents can reuse the same template across product reviews, customer updates, incident summaries, and sales enablement workflows.',
          'A good implementation treats OfficeCLI as the file-production step after the agent has gathered facts and written the brief. The agent should call the command, return the local artifact path, and include the preview URL when publishing is enabled. That keeps the automation inspectable instead of hiding the presentation behind an opaque chat response.',
          'Teams can also add a simple review step after generation. Use the generated deck as the first draft, run PPTX review when quality matters, and keep the prompt file with the source material so future updates can be regenerated instead of hand-edited from scratch.',
        ],
      },
    ],
    bestFor: [
      'Agent products that need editable strategy decks, updates, sales materials, or executive reviews.',
      'Internal automation where prompts and generated files must be reproducible from a command.',
      'Teams that want local-first generation with optional hosted credits and online previews.',
    ],
    notFor: [
      'Pixel-perfect brand decks that require a human designer for every slide.',
      'Pure image slideshows where a standalone image generator is the real output.',
      'Browser-only workflows that cannot call a local CLI or server-side binary.',
    ],
    faqs: [
      {
        q: 'Does OfficeCLI generate real PowerPoint files?',
        a: 'Yes. The PPTX output is an editable Office file, not a screenshot-only export.',
      },
      {
        q: 'Can I generate text-only decks?',
        a: 'Yes. Use `--no-images` when you want the deck to avoid generated or embedded images.',
      },
      {
        q: 'Can an agent publish the generated deck online?',
        a: 'Yes. When publish is configured, OfficeCLI can return a password-protected online preview URL after a successful generation.',
      },
    ],
  },
  {
    path: '/docx-generator-cli',
    navLabel: 'DOCX Generator CLI',
    title: 'DOCX Generator CLI for Agent-Written Documents',
    seoTitle: 'DOCX Generator CLI | Generate editable Word documents with OfficeCLI',
    seoDescription:
      'Generate editable DOCX documents from terminal and agent workflows with OfficeCLI. Use External Mode, Hosted Mode, prompt files, and optional online preview publishing.',
    eyebrow: 'DOCX automation',
    intro:
      'OfficeCLI gives agents and automation scripts a practical DOCX generator CLI. Instead of asking a model for markdown and then hand-building Word automation, the workflow can request a structured document directly and keep the result editable.',
    command:
      'officecli new docx "Product Launch Brief" --prompt "Write a concise launch brief with audience, positioning, timeline, risks, and next steps."',
    sections: [
      {
        title: 'From prompt to editable Word document',
        body: [
          'DOCX is still the handoff format for launch briefs, customer memos, implementation plans, policy drafts, and stakeholder reports. OfficeCLI focuses on producing a real document artifact instead of leaving the user with chat text that must be copied, formatted, and checked manually.',
          'The CLI is useful inside agent workflows because it has a stable command surface. A task runner can pass a direct prompt for short jobs or use `--prompt-file` for longer reusable templates. The output can be stored, attached, reviewed, or published without adding a separate document-conversion service.',
        ],
      },
      {
        title: 'Use your own model or hosted credits',
        body: [
          'External Mode is free and unlimited for teams that bring their own model endpoint. This is the right default for developer platforms, consulting automation, and private workflows that already have an approved LLM route.',
          'Hosted Mode uses hosted credits and removes the setup step for users who just need the document generated. That makes it suitable for onboarding, demos, and product surfaces where the first successful file matters more than exposing model configuration.',
        ],
      },
      {
        title: 'Part of a broader Office runtime',
        body: [
          'DOCX generation sits beside PPTX, XLSX, REPORT, and IMG workflows. An agent can draft a customer brief, create the spreadsheet that supports it, generate a deck summary, and return preview links through the same OfficeCLI installation.',
          'The optional publish flow returns a password-protected preview URL. That gives users a quick inspection path while preserving the editable DOCX as the source artifact for revisions, comments, and downstream office workflows.',
        ],
      },
      {
        title: 'Use DOCX generation safely in automation',
        body: [
          'The strongest DOCX workflows separate content planning from file generation. Let the agent gather source material, decide the document outline, and write a complete brief. Then pass that brief to OfficeCLI with a direct prompt or prompt file so the generation step has a narrow, auditable job.',
          'For recurring documents, keep a prompt file per document family: launch brief, implementation memo, customer update, incident review, or internal proposal. That gives teams repeatable structure without forcing developers to maintain low-level Word document code for headings, sections, tables, and callouts.',
          'After generation, users can revise the DOCX in their normal editor while the agent keeps the original prompt and command as the reproducible recipe. That is usually cleaner than asking a chat model to remember every formatting decision across follow-up messages.',
          'For team workflows, record the command next to the generated file or ticket. That small habit makes document automation easier to audit, because reviewers can see what the agent asked OfficeCLI to produce and rerun the same generation path when source facts change.',
          'It also gives support teams a concrete artifact trail: source brief, CLI command, local DOCX output, runtime mode, install channel, account state, billing status, and optional preview link.',
        ],
      },
    ],
    bestFor: [
      'Agents that draft briefs, memos, proposals, reports, and implementation documents.',
      'Teams that want a command-line document generator instead of custom Word XML scripts.',
      'Workflows that need local files plus optional online preview sharing.',
    ],
    notFor: [
      'Long legal documents that require specialist template review before every release.',
      'Desktop-only macros that must run inside Microsoft Word itself.',
      'Markdown-only publishing pipelines where DOCX is not a required output.',
    ],
    faqs: [
      {
        q: 'Can OfficeCLI use prompt files for DOCX generation?',
        a: 'Yes. `--prompt-file` is the preferred path for longer reusable briefs and structured templates.',
      },
      {
        q: 'Do I need Microsoft Word installed?',
        a: 'No. Generation does not require Word, LibreOffice, Docker, or Kubernetes.',
      },
      {
        q: 'Can generated DOCX files be shared as previews?',
        a: 'Yes. When publishing is configured, OfficeCLI can create a password-protected online preview link.',
      },
    ],
  },
  {
    path: '/xlsx-report-generation',
    navLabel: 'XLSX Report Generation',
    title: 'XLSX and Workbook-Backed Report Generation',
    seoTitle: 'XLSX Report Generation | Spreadsheet and report automation with OfficeCLI',
    seoDescription:
      'Use OfficeCLI for XLSX generation and workbook-backed report workflows. Generate spreadsheets, create HTML reports from workbooks, and publish previews from agent automation.',
    eyebrow: 'Spreadsheet automation',
    intro:
      'OfficeCLI covers both spreadsheet creation and workbook-backed report generation. It is designed for agent workflows where data has to become an editable XLSX workbook or a readable report artifact without hand-writing spreadsheet plumbing.',
    command:
      'officecli new xlsx "Sales Pipeline" --prompt "Create a sales pipeline workbook with stages, owners, deal values, probability, and next action columns."',
    sections: [
      {
        title: 'Generate structured workbooks',
        body: [
          'Spreadsheet automation often starts with a model response that looks like a table but ends as manual cleanup. OfficeCLI gives the agent a direct route to XLSX so the output can contain sheets, headers, example rows, and workbook structure that users can keep editing.',
          'This works well for sales trackers, budget workbooks, research matrices, project plans, QA checklists, and operational dashboards. A prompt can define the sheets and fields, while OfficeCLI handles the file generation path and keeps the result available as a local artifact.',
        ],
      },
      {
        title: 'Turn workbooks into reports',
        body: [
          'The REPORT workflow is separate from simple XLSX creation. It uses an existing workbook as the source of truth and generates a report around that data. That shape is useful when the spreadsheet already contains metrics, tables, or operating evidence that an agent should summarize for stakeholders.',
          'A typical command passes `--file <xlsx-path>` plus a prompt that explains the decisions, audience, and level of detail. The output is a workbook-backed HTML report, which is easier to read than raw spreadsheet tabs while preserving the data source behind the analysis.',
        ],
      },
      {
        title: 'Local-first with optional hosted runtime',
        body: [
          'External Mode is free and unlimited with your own LLM endpoint, which fits private data workflows and internal model gateways. Hosted Mode uses hosted credits when you want OfficeCLI-managed generation without configuring a provider first.',
          'Generated XLSX and REPORT outputs can also use the publish channel. A successful run can produce a password-protected preview URL, letting an agent return a link for review while keeping the workbook or report artifact available locally.',
        ],
      },
      {
        title: 'Design workbook prompts for reliable outputs',
        body: [
          'Spreadsheet prompts should name sheets, columns, data types, sample-row expectations, and any fields that downstream users will filter or sort. That gives the generator a concrete workbook contract and makes the result easier for agents, analysts, and operators to inspect.',
          'For REPORT generation, treat the workbook as evidence rather than decoration. The prompt should explain the audience, the business question, and the decisions that the report should support. OfficeCLI then becomes the bridge from structured workbook data to a readable artifact that can be shared or revised.',
          'This pattern works especially well when a workflow has both machine-readable data and human-readable conclusions. Keep the workbook available for validation, then publish or attach the report for stakeholders who need the summary rather than the raw spreadsheet.',
          'In agent systems, this also reduces ambiguity. The workbook remains the source file, the prompt explains how to interpret it, and the generated report becomes the communication layer. Each part can be inspected separately when a number, conclusion, or recommendation needs review.',
          'That separation is helpful for regulated or finance-adjacent teams that need to preserve source evidence while still moving quickly.',
        ],
      },
    ],
    bestFor: [
      'Agents that create trackers, operating workbooks, and data review sheets.',
      'Reporting workflows where an XLSX file is the source of truth.',
      'Teams that want CLI-driven spreadsheet artifacts without building a custom generator.',
    ],
    notFor: [
      'Complex financial models that require audited formulas and manual controls.',
      'Realtime BI dashboards that should stay inside a database-backed analytics tool.',
      'Spreadsheet macros that must execute in a desktop Office environment.',
    ],
    faqs: [
      {
        q: 'What is the difference between XLSX and REPORT?',
        a: 'XLSX creates a workbook. REPORT uses a workbook as input and creates a readable report artifact from it.',
      },
      {
        q: 'Can I use an existing workbook?',
        a: 'Yes. Use `officecli new report ... --file ./source.xlsx` when the workbook is the data source.',
      },
      {
        q: 'Can the report be previewed online?',
        a: 'Yes. The publish flow can return a password-protected preview URL when publishing is enabled.',
      },
    ],
  },
  {
    path: '/office-automation-ai-agents',
    navLabel: 'Office Automation for AI Agents',
    title: 'Office Automation for AI Agents',
    seoTitle: 'Office Automation for AI Agents | PPTX, DOCX, XLSX, REPORT, and IMG',
    seoDescription:
      'OfficeCLI gives AI agents a local-first Office automation runtime for PPTX, DOCX, XLSX, REPORT, and IMG outputs with External Mode, Hosted Mode, and online preview publishing.',
    eyebrow: 'Agent runtime',
    intro:
      'OfficeCLI is an Office automation runtime for AI agents that need to create real files. It gives Claude Code, Codex-style local agents, OpenClaw, internal bots, and CI jobs a consistent way to generate Office artifacts without embedding document-generation logic into every agent.',
    command:
      'officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --prompt "Summarize regional revenue shifts, efficiency signals, and the board-level decisions this workbook implies."',
    sections: [
      {
        title: 'A stable tool surface for agents',
        body: [
          'Agents are good at planning, drafting, and reasoning, but office formats require a predictable runtime. OfficeCLI provides that runtime as one binary with a clear command surface. The agent can choose PPTX, DOCX, XLSX, REPORT, or IMG based on the task instead of inventing a new exporter every time.',
          'The public officecli repository also distributes skills, plugin wrappers, demos, install scripts, and agent-facing docs. That means a local agent can be taught when to call OfficeCLI, how to verify the environment, and how to return the generated artifact to the user.',
        ],
      },
      {
        title: 'External when you own the provider, hosted when you need speed',
        body: [
          'External Mode is free and unlimited with a bring-your-own LLM endpoint. It is the right fit for enterprise agents, developer workstations, and internal automation where model routing is already standardized.',
          'Hosted Mode uses hosted credits for the OfficeCLI-managed runtime. It keeps setup short for first-run users and product demos, while still using the same CLI commands and generated artifact flow as External Mode.',
        ],
      },
      {
        title: 'Artifacts, previews, and inspection',
        body: [
          'The output is not just a chat answer. OfficeCLI writes files that can be inspected, shared, revised, and stored. For presentations and documents, users keep the editable Office artifact. For reports, teams get a readable HTML output backed by workbook data. For standalone visuals, the IMG path saves a local PNG.',
          'Publishing adds a review channel. When configured, successful generations can return password-protected `officecli.io/p/<id>` preview URLs. That gives agents a user-friendly response without forcing every environment to host generated files itself.',
        ],
      },
      {
        title: 'A practical agent integration pattern',
        body: [
          'The cleanest integration is to make the agent responsible for intent and OfficeCLI responsible for the artifact. The agent decides the output format, writes or selects the prompt, calls the CLI, and reports the generated file path. OfficeCLI handles the generation runtime, document packaging, and optional preview publication.',
          'This boundary also makes failures easier to debug. If the agent selected the wrong format, the prompt can be fixed. If the runtime lacks access, `officecli whoami` exposes the current mode. If a user needs a shareable link, publish configuration can be checked without changing the agent plan itself.',
          'For product teams, that boundary keeps the agent surface small. The agent does not need to know the internals of PPTX, DOCX, XLSX, or report rendering. It only needs to choose the right OfficeCLI command and provide enough context for the document runtime to produce a usable artifact.',
          'For operators, it also creates a clear support path. They can verify the installed binary, runtime mode, hosted credit state, publish configuration, and output path without reverse-engineering a custom integration inside every agent or workflow.',
        ],
      },
    ],
    bestFor: [
      'Agent products that need real office files as task outputs.',
      'Local coding agents that should generate documents without browser automation.',
      'Internal workflows that combine model reasoning with file production and preview sharing.',
    ],
    notFor: [
      'Chat-only workflows where no file artifact is needed.',
      'Fully custom design systems that require a bespoke renderer for every output.',
      'Environments that forbid local binaries and server-side CLI execution.',
    ],
    faqs: [
      {
        q: 'Which agent runtimes can use OfficeCLI?',
        a: 'The public docs cover Claude Code, Codex-style local agents, OpenClaw, and other local agent hosts that can call a CLI.',
      },
      {
        q: 'Is OfficeCLI only a skill bundle?',
        a: 'No. The skill bundle routes tasks, but the OfficeCLI runtime is the binary that creates the files.',
      },
      {
        q: 'Can agents use both local and hosted generation?',
        a: 'Yes. External Mode and Hosted Mode use the same command surface, with different runtime configuration.',
      },
    ],
  },
  {
    path: '/officecli-vs-python-docx-openpyxl-libreoffice',
    navLabel: 'OfficeCLI vs Libraries',
    title: 'OfficeCLI vs python-docx, openpyxl, and LibreOffice',
    seoTitle: 'OfficeCLI vs python-docx, openpyxl, and LibreOffice | Agent Office automation',
    seoDescription:
      'Compare OfficeCLI with python-docx, openpyxl, and LibreOffice for AI agent document generation, spreadsheet automation, reports, previews, and local-first workflows.',
    eyebrow: 'Comparison',
    intro:
      'python-docx, openpyxl, and LibreOffice are strong tools, but they solve different layers of the Office automation problem. OfficeCLI is aimed at agent workflows that need a higher-level path from prompt to editable artifact and optional online preview.',
    command:
      'officecli new docx "Implementation Memo" --prompt-file ./memo-prompt.md --local-preview --no-publish',
    sections: [
      {
        title: 'Library control versus product workflow',
        body: [
          'python-docx is useful when a developer wants low-level control over Word document structure. openpyxl is useful for spreadsheet reads, writes, and formula-aware workbook manipulation. LibreOffice is useful for conversion, rendering, and desktop-compatible document operations. These tools are still valuable when you are building your own document system.',
          'OfficeCLI sits above that layer. It gives an agent or automation job a productized command for generating PPTX, DOCX, XLSX, REPORT, and IMG outputs. Instead of writing custom code for every format, the agent can call the same binary and use prompts, prompt files, runtime configuration, review, and publish behavior consistently.',
        ],
      },
      {
        title: 'Where OfficeCLI is stronger',
        body: [
          'OfficeCLI is stronger when the input is a natural-language brief and the required result is an editable office artifact. It handles the document-generation workflow, not just a single file library. It also includes the hosted and external runtime split, which lets teams choose between their own model endpoint and OfficeCLI-managed hosted credits.',
          'The built-in preview publishing path is another difference. A script based on python-docx or openpyxl usually stops after writing a local file unless you build upload, auth, storage, and preview infrastructure. OfficeCLI can publish a password-protected preview link as part of the generation flow when publishing is configured.',
        ],
      },
      {
        title: 'Where lower-level tools still fit',
        body: [
          'Use python-docx or openpyxl when your application needs exact programmatic control over every paragraph, cell, style, or formula. Use LibreOffice when conversion fidelity or visual rendering is the main job. Use OfficeCLI when an agent needs to turn a brief, workbook, or task into a finished artifact quickly.',
          'Many teams can use both. OfficeCLI can own first-draft generation and preview sharing, while specialized scripts handle audited templates, data validation, or final production formatting. The boundary is practical: use the lowest-level tool only when the extra control is worth the extra code.',
        ],
      },
      {
        title: 'Choose the right layer for the job',
        body: [
          'If your system already has exact document templates and every field is deterministic, low-level libraries may be the right layer. They give you programmatic control and predictable output when the document shape is known before the model runs.',
          'If your system starts with an agent task, a user brief, or a workbook that needs interpretation, OfficeCLI is usually a better first layer. It reduces custom glue code, supports multiple Office outputs, and gives teams a path from local artifact to password-protected preview without building that product surface themselves.',
          'The comparison is not about replacing every tool. It is about choosing the smallest reliable layer. Use OfficeCLI for prompt-driven artifact generation, then bring in python-docx, openpyxl, or LibreOffice only where exact template editing, formula manipulation, or conversion fidelity is the central requirement.',
          'That approach keeps the system easier to maintain. Product teams get a single agent-facing document command surface, while specialist code stays reserved for places where a low-level library is genuinely necessary and tested.',
        ],
      },
    ],
    bestFor: [
      'Agent workflows that need prompt-to-file generation across multiple Office formats.',
      'Teams that do not want to maintain separate PPTX, DOCX, XLSX, report, and preview pipelines.',
      'Products that need local artifacts plus optional hosted runtime and online previews.',
    ],
    notFor: [
      'Applications that require complete low-level manipulation of every OOXML node.',
      'Spreadsheet engines that need custom formula auditing and deterministic financial controls.',
      'Document conversion systems where LibreOffice rendering is the core requirement.',
    ],
    faqs: [
      {
        q: 'Does OfficeCLI replace python-docx or openpyxl?',
        a: 'No. It is a higher-level workflow tool. Use the libraries when you need low-level document or spreadsheet control.',
      },
      {
        q: 'When is OfficeCLI the better fit?',
        a: 'Use OfficeCLI when an agent needs to generate a finished Office artifact from a prompt or workbook without maintaining format-specific code.',
      },
      {
        q: 'Can I still use LibreOffice with OfficeCLI?',
        a: 'Yes. OfficeCLI can use LibreOffice for stronger visual review when it is available, but generation does not require it.',
      },
    ],
  },
]

export const seoLandingPaths = seoLandingPages.map((page) => page.path)

export function getSEOLandingPage(pathname: string) {
  return seoLandingPages.find((page) => page.path === pathname)
}
