import { Link } from 'react-router-dom'
import { agentSkillsRoutes } from '../agentSkillsData'

export default function AgentSkillsSecondaryNav({ currentPath }: { currentPath: string }) {
  return (
    <nav aria-label="officecli sections" className="rounded-[28px] border border-outline-variant/10 bg-surface-low/80 p-4">
      <div className="mb-3 font-headline text-xs uppercase tracking-[0.24em] text-tertiary">officecli sections</div>
      <div className="grid gap-3 md:grid-cols-3 xl:grid-cols-6">
        {agentSkillsRoutes.map((route) => {
          const isActive = route.path === currentPath
          return (
            <Link
              key={route.path}
              className={
                isActive
                  ? 'rounded-2xl border border-primary/30 bg-primary/10 px-4 py-3 text-sm font-semibold text-primary'
                  : 'rounded-2xl border border-outline-variant/10 bg-background/70 px-4 py-3 text-sm font-semibold text-white transition-colors hover:border-primary/20 hover:text-primary'
              }
              to={route.path}
            >
              <div>{route.label}</div>
              <div className="mt-2 text-xs font-normal leading-relaxed text-outline-variant">{route.title}</div>
            </Link>
          )
        })}
      </div>
    </nav>
  )
}
