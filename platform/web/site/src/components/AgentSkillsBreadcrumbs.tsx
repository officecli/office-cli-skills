import { Link } from 'react-router-dom'

interface BreadcrumbItem {
  label: string
  to?: string
}

export default function AgentSkillsBreadcrumbs({ items }: { items: BreadcrumbItem[] }) {
  if (items.length === 0) return null

  return (
    <nav aria-label="Breadcrumb" className="mb-6 flex flex-wrap items-center gap-2 text-sm text-outline-variant">
      {items.map((item, index) => (
        <span key={`${item.label}-${index}`} className="flex items-center gap-2">
          {item.to ? <Link className="transition-colors hover:text-primary" to={item.to}>{item.label}</Link> : <span className="text-white">{item.label}</span>}
          {index < items.length - 1 && <span aria-hidden="true">/</span>}
        </span>
      ))}
    </nav>
  )
}
