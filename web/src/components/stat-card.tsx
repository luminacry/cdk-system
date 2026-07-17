import type { LucideIcon } from "lucide-react"

import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"

interface StatCardProps {
  title: string
  value: number | string
  icon: LucideIcon
  hint?: string
  tone?: "default" | "primary" | "destructive"
}

/** 仪表盘统计卡片 */
export function StatCard({ title, value, icon: Icon, hint, tone = "default" }: StatCardProps) {
  return (
    <Card
      className={cn(
        tone === "primary" && "border-primary/40",
        tone === "destructive" && "border-destructive/40",
      )}
    >
      <CardContent className="flex items-center gap-4 p-5">
        <div
          className={cn(
            "flex size-11 shrink-0 items-center justify-center rounded-lg",
            tone === "primary" && "bg-primary/15 text-primary",
            tone === "destructive" && "bg-destructive/15 text-destructive",
            tone === "default" && "bg-muted text-muted-foreground",
          )}
        >
          <Icon className="size-5" />
        </div>
        <div className="min-w-0">
          <div className="truncate text-sm text-muted-foreground">{title}</div>
          <div className="font-mono text-2xl font-semibold tabular-nums">{value}</div>
          {hint ? <div className="text-xs text-muted-foreground">{hint}</div> : null}
        </div>
      </CardContent>
    </Card>
  )
}
