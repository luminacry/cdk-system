import { Badge } from "@/components/ui/badge"

/** 兑换结果徽标 */
export function ResultBadge({ result, message }: { result: string; message?: string }) {
  const ok = result === "success"
  return (
    <Badge variant={ok ? "default" : "destructive"} className={ok ? "bg-emerald-600 text-white dark:bg-emerald-700" : undefined}>
      {message || result}
    </Badge>
  )
}

const WEBHOOK_META: Record<string, { label: string; variant: "default" | "secondary" | "destructive" | "outline" }> = {
  pending: { label: "推送中", variant: "secondary" },
  success: { label: "已送达", variant: "default" },
  failed: { label: "失败", variant: "destructive" },
  dead_letter: { label: "重试耗尽", variant: "destructive" },
}

/** Webhook 推送状态徽标 */
export function WebhookBadge({ status }: { status: string }) {
  if (status === "none") return <span className="text-muted-foreground">—</span>
  const meta = WEBHOOK_META[status] ?? { label: status, variant: "outline" as const }
  return (
    <Badge
      variant={meta.variant}
      className={status === "success" ? "bg-emerald-600 text-white dark:bg-emerald-700" : undefined}
    >
      {meta.label}
    </Badge>
  )
}

/** 批次状态徽标 */
export function BatchStatusBadge({ status }: { status: string }) {
  return status === "active" ? (
    <Badge className="bg-emerald-600 text-white dark:bg-emerald-700">启用中</Badge>
  ) : (
    <Badge variant="destructive">已停用</Badge>
  )
}

/** 单个兑换码状态徽标 */
export function CodeStatusBadge({
  status,
  useCount,
  maxUses,
}: {
  status: string
  useCount: number
  maxUses: number
}) {
  if (status === "disabled") return <Badge variant="destructive">已禁用</Badge>
  if (useCount >= maxUses) return <Badge variant="secondary">已用完</Badge>
  if (useCount > 0) return <Badge className="bg-amber-500 text-white dark:bg-amber-600">部分使用</Badge>
  return <Badge className="bg-emerald-600 text-white dark:bg-emerald-700">可用</Badge>
}
