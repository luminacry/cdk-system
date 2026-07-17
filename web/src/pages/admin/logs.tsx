import { useCallback, useEffect, useState } from "react"
import { RotateCcw, Search } from "lucide-react"
import { toast } from "sonner"

import { api } from "@/lib/api"
import type { LogItem } from "@/lib/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataPagination } from "@/components/data-pagination"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { ResultBadge, WebhookBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"

const RESULT_OPTIONS = [
  { value: "success", label: "成功" },
  { value: "invalid_code", label: "码无效" },
  { value: "code_used_up", label: "已用完" },
  { value: "code_disabled", label: "码被禁用" },
  { value: "batch_disabled", label: "批次停用" },
  { value: "expired", label: "已过期" },
  { value: "user_limit", label: "邮箱超限" },
  { value: "invalid_input", label: "输入无效" },
]

const WEBHOOK_OPTIONS = [
  { value: "pending", label: "推送中" },
  { value: "success", label: "已送达" },
  { value: "failed", label: "失败" },
  { value: "dead_letter", label: "重试耗尽" },
]

export function LogsPage() {
  const [logs, setLogs] = useState<LogItem[] | null>(null)
  const [paged, setPaged] = useState({ total: 0, page: 1, pages: 1 })
  const [batchOptions, setBatchOptions] = useState<{ id: number; name: string }[]>([])

  const [batchId, setBatchId] = useState("")
  const [result, setResult] = useState("")
  const [webhook, setWebhook] = useState("")
  const [userId, setUserId] = useState("")
  const [page, setPage] = useState(1)

  const load = useCallback(() => {
    setLogs(null)
    api
      .logs({
        batch_id: batchId ? Number(batchId) : undefined,
        result: result || undefined,
        webhook: webhook || undefined,
        user_id: userId.trim() || undefined,
        page,
      })
      .then((d) => {
        setLogs(d.logs)
        setPaged({ total: d.total, page: d.page, pages: d.pages })
        setBatchOptions(d.batches)
      })
      .catch((err) => toast.error(err.message))
  }, [batchId, result, webhook, userId, page])

  useEffect(load, [load])

  function resetFilters() {
    setBatchId("")
    setResult("")
    setWebhook("")
    setUserId("")
    setPage(1)
  }

  async function handleResend(id: number) {
    try {
      const res = await api.resendWebhook(id)
      toast.success(res.message)
      setTimeout(load, 1500) // 等待后台线程写回结果再刷新
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败")
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title="兑换记录" description="每一次兑换尝试（成功与失败）都留有记录" />

      <Card>
        <CardHeader>
          <CardTitle>筛选</CardTitle>
          <CardDescription>按批次、结果、Webhook 状态或邮箱搜索</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={batchId}
              onValueChange={(v) => {
                setBatchId(!v || v === "__all__" ? "" : v)
                setPage(1)
              }}
            >
              <SelectTrigger className="w-48">
                <SelectValue placeholder="全部批次" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">全部批次</SelectItem>
                {batchOptions.map((b) => (
                  <SelectItem key={b.id} value={String(b.id)}>
                    #{b.id} {b.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={result}
              onValueChange={(v) => {
                setResult(!v || v === "__all__" ? "" : v)
                setPage(1)
              }}
            >
              <SelectTrigger className="w-36">
                <SelectValue placeholder="全部结果" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">全部结果</SelectItem>
                {RESULT_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={webhook}
              onValueChange={(v) => {
                setWebhook(!v || v === "__all__" ? "" : v)
                setPage(1)
              }}
            >
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Webhook 全部" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">Webhook 全部</SelectItem>
                {WEBHOOK_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Input
              className="w-48"
              type="email"
              inputMode="email"
              autoComplete="off"
              placeholder="按邮箱搜索"
              value={userId}
              onChange={(e) => {
                setUserId(e.target.value)
                setPage(1)
              }}
            />
            <Button variant="outline" onClick={resetFilters}>
              <RotateCcw className="size-4" />
              重置
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>记录列表</CardTitle>
          <CardDescription>共 {paged.total} 条</CardDescription>
        </CardHeader>
        <CardContent>
          {logs === null ? (
            <Skeleton className="h-64 rounded-lg" />
          ) : logs.length === 0 ? (
            <EmptyState icon={Search} title="没有符合条件的记录" description="调整筛选条件试试" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>时间</TableHead>
                    <TableHead>批次</TableHead>
                    <TableHead>兑换码</TableHead>
                    <TableHead>邮箱</TableHead>
                    <TableHead>结果</TableHead>
                    <TableHead>Webhook</TableHead>
                    <TableHead>回执</TableHead>
                    <TableHead>IP</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {logs.map((r) => (
                    <TableRow key={r.id}>
                      <TableCell className="font-mono text-xs whitespace-nowrap">
                        {r.created_at}
                      </TableCell>
                      <TableCell className="whitespace-nowrap">{r.batch_name ?? "—"}</TableCell>
                      <TableCell className="font-mono text-xs whitespace-nowrap">{r.code}</TableCell>
                      <TableCell className="max-w-28 truncate">{r.user_id}</TableCell>
                      <TableCell>
                        <ResultBadge result={r.result} message={r.message} />
                      </TableCell>
                      <TableCell>
                        <WebhookBadge status={r.webhook_status} />
                      </TableCell>
                      <TableCell className="max-w-40">
                        {r.webhook_response ? (
                          <Tooltip>
                            <TooltipTrigger
                              render={
                                <span className="block cursor-help truncate text-xs text-muted-foreground">
                                  {r.webhook_response}
                                </span>
                              }
                            />
                            <TooltipContent className="max-w-sm break-all">
                              {r.webhook_response}
                            </TooltipContent>
                          </Tooltip>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="font-mono text-xs">{r.ip}</TableCell>
                      <TableCell className="text-right">
                        {r.result === "success" &&
                        ["success", "failed", "dead_letter"].includes(r.webhook_status) ? (
                          <ConfirmDialog
                            trigger={
                              <Button variant="outline" size="sm">
                                重推
                              </Button>
                            }
                            title="确定重新推送该记录的 Webhook 吗？"
                            description="接收端应根据 redemption_id 做幂等去重。"
                            confirmText="重推"
                            onConfirm={() => handleResend(r.id)}
                          />
                        ) : null}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
          <DataPagination page={paged.page} pages={paged.pages} total={paged.total} onPage={setPage} />
        </CardContent>
      </Card>
    </div>
  )
}
