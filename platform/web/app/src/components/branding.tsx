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
  const bg = `officecli-bg-${id}`
  const frame = `officecli-frame-${id}`
  const frameEdge = `officecli-frame-edge-${id}`
  const doc = `officecli-doc-${id}`
  const side = `officecli-side-${id}`
  const screen = `officecli-screen-${id}`
  const glowSoft = `officecli-glow-soft-${id}`
  const glowHard = `officecli-glow-hard-${id}`
  const shadow = `officecli-shadow-${id}`

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
        <radialGradient id={bg} cx="0" cy="0" r="1" gradientUnits="userSpaceOnUse" gradientTransform="translate(113 42) rotate(135) scale(159)">
          <stop stopColor="#142B31" />
          <stop offset="0.45" stopColor="#0C1822" />
          <stop offset="1" stopColor="#060B11" />
        </radialGradient>
        <linearGradient id={frame} x1="37" y1="38" x2="118" y2="128" gradientUnits="userSpaceOnUse">
          <stop stopColor="#FFFFFF" />
          <stop offset="0.2" stopColor="#EAF7FF" />
          <stop offset="0.58" stopColor="#B8DFFF" />
          <stop offset="1" stopColor="#69B8FF" />
        </linearGradient>
        <linearGradient id={frameEdge} x1="45" y1="126" x2="89" y2="140" gradientUnits="userSpaceOnUse">
          <stop stopColor="#B8E2FF" />
          <stop offset="1" stopColor="#479CFF" />
        </linearGradient>
        <linearGradient id={doc} x1="106" y1="30" x2="156" y2="144" gradientUnits="userSpaceOnUse">
          <stop stopColor="#E7FFF9" />
          <stop offset="0.3" stopColor="#8AF9E8" />
          <stop offset="0.72" stopColor="#32E3D0" />
          <stop offset="1" stopColor="#19B7FF" />
        </linearGradient>
        <linearGradient id={side} x1="126" y1="74" x2="156" y2="146" gradientUnits="userSpaceOnUse">
          <stop stopColor="#64F8EA" />
          <stop offset="1" stopColor="#1882FF" />
        </linearGradient>
        <linearGradient id={screen} x1="56" y1="58" x2="110" y2="114" gradientUnits="userSpaceOnUse">
          <stop stopColor="#0E1822" />
          <stop offset="1" stopColor="#04090F" />
        </linearGradient>
        <filter id={glowSoft} x="0" y="0" width="176" height="176" filterUnits="userSpaceOnUse">
          <feGaussianBlur stdDeviation="12" />
        </filter>
        <filter id={glowHard} x="20" y="18" width="140" height="142" filterUnits="userSpaceOnUse">
          <feGaussianBlur stdDeviation="4.2" />
        </filter>
        <filter id={shadow} x="20" y="20" width="144" height="138" filterUnits="userSpaceOnUse">
          <feDropShadow dx="0" dy="8" stdDeviation="10" floodColor="#02111A" floodOpacity="0.42" />
        </filter>
      </defs>

      <rect x="6" y="6" width="164" height="164" rx="34" fill="url(#bg)" />
      <rect x="6" y="6" width="164" height="164" rx="34" stroke="#0E1B26" strokeOpacity="0.78" />

      <ellipse cx="118" cy="42" rx="34" ry="28" fill="#2AF1DE" fillOpacity="0.28" filter={`url(#${glowSoft})`} />
      <ellipse cx="58" cy="115" rx="38" ry="26" fill="#4AA4FF" fillOpacity="0.22" filter={`url(#${glowSoft})`} />
      <ellipse cx="104" cy="86" rx="54" ry="40" fill="#59D5FF" fillOpacity="0.08" filter={`url(#${glowSoft})`} />

      <g filter={`url(#${shadow})`}>
        <path
          d="M38 68L101 43C108 40 116 41.5 121.8 46.8L136 59L110 131H56V121H38V68Z"
          fill="url(#frame)"
        />
        <path
          d="M40.5 69.5L101.5 45.4C107.4 43.1 114.1 44.1 119 48.5L131.4 59.7"
          stroke="#F7FEFF"
          strokeOpacity="0.96"
          strokeWidth="2.8"
          strokeLinecap="round"
        />
        <path
          d="M56 121H84L70.5 133.2H42.5L56 121Z"
          fill="url(#frameEdge)"
        />
        <path
          d="M107 29H119.5C123.8 29 128 30.7 131 33.8L147.2 50C150.2 53 151.9 57.1 151.9 61.4V133.5C151.9 139.5 147 144.4 141 144.4H108.8C102.8 144.4 97.9 139.5 97.9 133.5V38.1C97.9 33.1 102 29 107 29Z"
          fill="url(#doc)"
        />
        <path
          d="M125 33.2L146.2 54.2V129.8L125 144.4V33.2Z"
          fill="url(#side)"
          fillOpacity="0.44"
        />
        <path
          d="M123.7 29.8V47.5C123.7 51.1 126.6 54 130.2 54H147.1"
          stroke="#F3FFFD"
          strokeOpacity="0.98"
          strokeWidth="3.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M103 31H119.2C123.3 31 127.2 32.6 130.1 35.5L145.5 50.8C148.3 53.7 149.9 57.6 149.9 61.7V131.7C149.9 137.2 145.4 141.7 139.9 141.7H109.6C104.1 141.7 99.6 137.2 99.6 131.7V34.4C99.6 32.5 101.1 31 103 31Z"
          stroke="#77FFF0"
          strokeOpacity="0.56"
          strokeWidth="1.7"
        />
        <path
          d="M56 59H109.2C112.4 59 115 61.6 115 64.8V102.2C115 105.4 112.4 108 109.2 108H56C52.8 108 50.2 105.4 50.2 102.2V64.8C50.2 61.6 52.8 59 56 59Z"
          fill="url(#screen)"
        />
        <path
          d="M56 59H109.2C112.4 59 115 61.6 115 64.8V102.2C115 105.4 112.4 108 109.2 108H56C52.8 108 50.2 105.4 50.2 102.2V64.8C50.2 61.6 52.8 59 56 59Z"
          stroke="#112231"
          strokeWidth="1.8"
        />
      </g>

      <g filter={`url(#${glowHard})`}>
        <path
          d="M38 68L101 43C108 40 116 41.5 121.8 46.8L136 59"
          stroke="#EAFBFF"
          strokeOpacity="0.92"
          strokeWidth="4.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M38 68V121H56"
          stroke="#D9F3FF"
          strokeOpacity="0.9"
          strokeWidth="4.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M56 121H84"
          stroke="#78BEFF"
          strokeOpacity="0.86"
          strokeWidth="4.4"
          strokeLinecap="round"
        />
        <path
          d="M108 30H118.6C123 30 127.2 31.8 130.2 34.9L146.1 50.7C149.2 53.8 150.9 57.9 150.9 62.3V132.9"
          stroke="#66FFF0"
          strokeOpacity="0.5"
          strokeWidth="3.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </g>

      <path d="M66 72L79.5 83L66 94" stroke="#FFFFFF" strokeWidth="7.2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M91 89H106" stroke="#FFFFFF" strokeWidth="7.2" strokeLinecap="round" />

      <g fill="#2E839C" fillOpacity="0.38">
        <circle cx="149" cy="92" r="1.2" />
        <circle cx="153" cy="92" r="1.2" />
        <circle cx="157" cy="92" r="1.2" />
        <circle cx="145" cy="96" r="1.2" />
        <circle cx="149" cy="96" r="1.2" />
        <circle cx="153" cy="96" r="1.2" />
        <circle cx="157" cy="96" r="1.2" />
        <circle cx="145" cy="100" r="1.2" />
        <circle cx="149" cy="100" r="1.2" />
        <circle cx="153" cy="100" r="1.2" />
        <circle cx="157" cy="100" r="1.2" />
      </g>
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
