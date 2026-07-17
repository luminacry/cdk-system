import { Inbox, type LucideIcon } from "lucide-react"

interface EmptyStateProps {
  icon?: LucideIcon
  title: string
  description?: string
}

/** 空数据占位 */
export function EmptyState({ icon: Icon = Inbox, title, description }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-14 text-center">
      <Icon className="size-10 text-muted-foreground/50" />
      <p className="text-sm font-medium text-muted-foreground">{title}</p>
      {description ? <p className="text-xs text-muted-foreground/70">{description}</p> : null}
    </div>
  )
}
