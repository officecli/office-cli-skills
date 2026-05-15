export type InstallTabID = 'macos' | 'linux' | 'manual'

export interface InstallCommand {
  label: string
  command: string
  detail: string
}

export interface FirstRunCommand {
  label: string
  command: string
}

export interface InstallTabContent {
  id: InstallTabID
  label: string
  eyebrow: string
  title: string
  description: string
  commands: InstallCommand[]
  notes: string[]
}

export const stableInstallScript =
  'curl -fsSL https://raw.githubusercontent.com/officecli/officecli-dist/main/scripts/install-officecli.sh | bash'

export const firstRunCommands: FirstRunCommand[] = [
  {
    label: 'Verify',
    command: 'officecli --version',
  },
  {
    label: 'PPTX',
    command:
      'officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review. Cover growth, retention, risks, and next-quarter actions."',
  },
  {
    label: 'DOCX',
    command:
      'officecli new docx "Product Launch Brief" --prompt "Write a concise launch brief with audience, positioning, timeline, risks, and next steps."',
  },
  {
    label: 'XLSX',
    command:
      'officecli new xlsx "Sales Pipeline" --prompt "Create a sales pipeline workbook with stages, owners, deal values, probability, and next action columns."',
  },
  {
    label: 'Access',
    command: 'officecli whoami',
  },
]

export const installTabs: InstallTabContent[] = [
  {
    id: 'macos',
    label: 'macOS',
    eyebrow: 'Recommended for Mac',
    title: 'Install OfficeCLI on macOS',
    description: 'Use Homebrew for the fastest setup, or npm if you prefer a Node-managed CLI workflow. Pick one channel and keep future updates on that same channel.',
    commands: [
      {
        label: 'Homebrew',
        command: 'brew tap officecli/officecli && brew install officecli',
        detail: 'Best default for Apple Silicon and Intel Macs.',
      },
      {
        label: 'npm',
        command: 'npm install -g officecli',
        detail: 'Supported on macOS x64 and arm64.',
      },
    ],
    notes: [
      'Supported CPUs: Apple Silicon arm64 and Intel x64.',
      'The npm package installs the same published OfficeCLI binary.',
      'If OfficeCLI was previously installed with Homebrew, do not run the npm install on top of it. To switch to npm, first run `brew uninstall officecli/homebrew-officecli/officecli` or `brew uninstall officecli`.',
    ],
  },
  {
    id: 'linux',
    label: 'Linux',
    eyebrow: 'Recommended for Linux',
    title: 'Install OfficeCLI on Linux',
    description: 'Use the official stable install script for a direct binary install, or npm for Node-based environments. Pick one channel and keep future updates on that same channel.',
    commands: [
      {
        label: 'Shell script',
        command: stableInstallScript,
        detail: 'Installs the latest stable public release.',
      },
      {
        label: 'npm',
        command: 'npm install -g officecli',
        detail: 'Supported on Linux x64 and arm64.',
      },
    ],
    notes: [
      'Supported CPUs: x64 and arm64.',
      'The install script defaults to the latest stable GitHub release.',
    ],
  },
  {
    id: 'manual',
    label: 'Manual',
    eyebrow: 'Manual binaries',
    title: 'Download a release archive directly',
    description: 'Use the public GitHub release assets if you want a manual install or need full control over the binary location. Once the binary is on PATH, run the same first commands below.',
    commands: [
      {
        label: 'Release page',
        command: 'https://github.com/officecli/officecli-dist/releases/latest',
        detail: 'Pick the archive for your OS and CPU, then move `officecli` into your PATH.',
      },
      {
        label: 'Checksums',
        command: 'https://github.com/officecli/officecli-dist/releases/latest/download/checksums.txt',
        detail: 'Verify the archive before installing.',
      },
    ],
    notes: [
      'Available targets: macOS arm64, macOS amd64, Linux arm64, Linux amd64.',
      'Windows is not supported in the current public release flow.',
    ],
  },
]

export function detectOperatingSystem(userAgent?: string): InstallTabID {
  const source = (userAgent ?? (typeof navigator !== 'undefined' ? navigator.userAgent : '')).toLowerCase()
  if (source.includes('mac os x') || source.includes('macintosh') || source.includes('darwin')) {
    return 'macos'
  }
  if (source.includes('linux') || source.includes('x11')) {
    return 'linux'
  }
  return 'manual'
}
