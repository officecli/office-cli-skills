import { useId, type ReactNode } from 'react'

function joinClasses(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(' ')
}

interface OfficeCliMarkProps {
  className?: string
  decorative?: boolean
  title?: string
}

export function OfficeCliMark({
  className,
  decorative = true,
  title = 'OfficeCLI logo',
}: OfficeCliMarkProps) {
  const id = useId().replace(/:/g, '')
  const glow = `officecli-glow-${id}`
  const frameGlow = `officecli-frame-glow-${id}`
  const frameGradient = `officecli-frame-gradient-${id}`
  const docGradient = `officecli-doc-gradient-${id}`
  const bgGradient = `officecli-bg-gradient-${id}`

  return (
    <svg
      viewBox="0 0 176 176"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-hidden={decorative}
      role={decorative ? undefined : 'img'}
    >
      {!decorative ? <title>{title}</title> : null}
      <defs>
        <linearGradient id={bgGradient} x1="18" y1="22" x2="164" y2="154" gradientUnits="userSpaceOnUse">
          <stop stopColor="#0A1522" />
          <stop offset="1" stopColor="#081018" />
        </linearGradient>
        <linearGradient id={frameGradient} x1="40" y1="42" x2="141" y2="137" gradientUnits="userSpaceOnUse">
          <stop stopColor="#F7FCFF" />
          <stop offset="0.32" stopColor="#BFE0FF" />
          <stop offset="0.72" stopColor="#5CB9FF" />
          <stop offset="1" stopColor="#4E8FFF" />
        </linearGradient>
        <linearGradient id={docGradient} x1="97" y1="24" x2="155" y2="144" gradientUnits="userSpaceOnUse">
          <stop stopColor="#C9FFF6" />
          <stop offset="0.4" stopColor="#52F2DA" />
          <stop offset="1" stopColor="#19C8FF" />
        </linearGradient>
        <filter id={glow} x="0" y="0" width="176" height="176" filterUnits="userSpaceOnUse">
          <feGaussianBlur stdDeviation="10" />
        </filter>
        <filter id={frameGlow} x="16" y="10" width="150" height="150" filterUnits="userSpaceOnUse">
          <feGaussianBlur stdDeviation="5" />
        </filter>
      </defs>

      <rect x="8" y="8" width="160" height="160" rx="36" fill="url(#bgGradient)" />
      <rect x="8" y="8" width="160" height="160" rx="36" fill="#050B12" fillOpacity="0.2" stroke="#132333" strokeOpacity="0.75" />

      <ellipse cx="124" cy="52" rx="32" ry="28" fill="#23F0D8" fillOpacity="0.2" filter={`url(#${glow})`} />
      <ellipse cx="72" cy="126" rx="42" ry="28" fill="#3D98FF" fillOpacity="0.18" filter={`url(#${glow})`} />

      <path
        d="M42 64L105 39C112 36 120 38 126 43L139 54"
        stroke="url(#frameGradient)"
        strokeWidth="12"
        strokeLinecap="round"
        strokeLinejoin="round"
        filter={`url(#${frameGlow})`}
      />
      <path
        d="M42 64L105 39C112 36 120 38 126 43L139 54"
        stroke="url(#frameGradient)"
        strokeWidth="11"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M42 64V118H68"
        stroke="url(#frameGradient)"
        strokeWidth="11"
        strokeLinecap="round"
        strokeLinejoin="round"
        filter={`url(#${frameGlow})`}
      />
      <path
        d="M42 64V118H68"
        stroke="url(#frameGradient)"
        strokeWidth="10"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M58 118H82"
        stroke="url(#frameGradient)"
        strokeWidth="11"
        strokeLinecap="round"
        filter={`url(#${frameGlow})`}
      />
      <path
        d="M58 118H82"
        stroke="url(#frameGradient)"
        strokeWidth="10"
        strokeLinecap="round"
      />

      <path
        d="M104 30H122C126 30 130 31.6 132.9 34.5L148.5 50.1C151.4 53 153 56.9 153 61V134C153 140.1 148.1 145 142 145H108C101.9 145 97 140.1 97 134V37C97 32.6 100.6 29 105 29Z"
        fill="url(#docGradient)"
        fillOpacity="0.88"
        stroke="#77F8E5"
        strokeOpacity="0.78"
        strokeWidth="1.8"
      />
      <path
        d="M123 30V48C123 51.3 125.7 54 129 54H147"
        stroke="#E8FFFB"
        strokeOpacity="0.92"
        strokeWidth="3.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />

      <rect x="54" y="56" width="58" height="50" rx="9" fill="#060D15" fillOpacity="0.95" />
      <rect x="54" y="56" width="58" height="50" rx="9" stroke="#12202E" />
      <path d="M68 72L80 82L68 92" stroke="#FFFFFF" strokeWidth="7" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M92 88H106" stroke="#FFFFFF" strokeWidth="7" strokeLinecap="round" />
    </svg>
  )
}

interface OfficeCliBrandProps {
  className?: string
  markClassName?: string
  titleClassName?: string
  subtitleClassName?: string
  subtitle?: ReactNode
  title?: string
}

export function OfficeCliBrand({
  className,
  markClassName,
  titleClassName,
  subtitleClassName,
  subtitle,
  title = 'OfficeCLI',
}: OfficeCliBrandProps) {
  return (
    <div className={joinClasses('flex items-center gap-3', className)}>
      <OfficeCliMark className={joinClasses('h-11 w-11 shrink-0', markClassName)} />
      <div className="min-w-0">
        <div className={joinClasses('font-headline text-lg font-bold tracking-tight text-white', titleClassName)}>
          {title}
        </div>
        {subtitle ? (
          <div className={joinClasses('info-eyebrow text-outline', subtitleClassName)}>
            {subtitle}
          </div>
        ) : null}
      </div>
    </div>
  )
}
