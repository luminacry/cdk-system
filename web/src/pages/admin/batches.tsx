import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { Package, Plus } from "lucide-react"
import { toast } from "sonner"

import { api } from "@/lib/api"
import type { Batch } from "@/lib/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { BatchStatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export function BatchesPage() {
  const [batches, setBatches] = useState<Batch[] | null>(null)

  function load() {
    api.batches().then((d) => setBatches(d.batches)).catch((err) => toast.error(err.message))
  }

  useEffect(load, [])

  async function handleToggle(b: Batch) {
    try {
      const res = await api.toggleBatch(b.id)
      toast.success(res.message)
      load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败")
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="批次管理"
        description="按批次生成与管理兑换码"
        actions={
          <Button
            nativeButton={false}
            render={
              <Link to="/admin/batches/new">
                <Plus className="size-4" />
                新建批次
              </Link>
            }
          />
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>全部批次</CardTitle>
          <CardDescription>共 {batches?.length ?? "…"} 个批次</CardDescription>
        </CardHeader>
        <CardContent>
          {batches === null ? (
            <Skeleton className="h-48 rounded-lg" />
          ) : batches.length === 0 ? (
            <EmptyState
              icon={Package}
              title="还没有批次"
              description="点击右上角「新建批次」开始生成兑换码"
            />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>ID</TableHead>
                    <TableHead>名称</TableHead>
                    <TableHead>前缀</TableHead>
                    <TableHead className="text-right">码数量</TableHead>
                    <TableHead className="text-right">已使用</TableHead>
                    <TableHead>过期时间</TableHead>
                    <TableHead>Webhook</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {batches.map((b) => (
                    <TableRow key={b.id}>
                      <TableCell className="font-mono text-xs">#{b.id}</TableCell>
                      <TableCell>
                        <Link
                          to={`/admin/batches/${b.id}`}
                          className="font-medium hover:underline"
                        >
                          {b.name}
                        </Link>
                        {b.description ? (
                          <div className="text-xs text-muted-foreground">{b.description}</div>
                        ) : null}
                      </TableCell>
                      <TableCell className="font-mono text-xs">{b.prefix || "—"}</TableCell>
                      <TableCell className="text-right font-mono">{b.code_count}</TableCell>
                      <TableCell className="text-right font-mono">{b.used_count}</TableCell>
                      <TableCell className="font-mono text-xs whitespace-nowrap">
                        {b.expires_at ? b.expires_at.replace("T", " ") : "永久"}
                      </TableCell>
                      <TableCell>
                        {b.webhook_url ? (
                          <Badge variant="outline">已配置</Badge>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <BatchStatusBadge status={b.status} />
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            nativeButton={false}
                            render={<Link to={`/admin/batches/${b.id}`}>详情</Link>}
                          />
                          <ConfirmDialog
                            trigger={
                              <Button
                                variant={b.status === "active" ? "destructive" : "default"}
                                size="sm"
                              >
                                {b.status === "active" ? "停用" : "启用"}
                              </Button>
                            }
                            title={`确定要${b.status === "active" ? "停用" : "启用"}批次「${b.name}」吗？`}
                            description={
                              b.status === "active"
                                ? "停用后该批次所有兑换码将无法兑换。"
                                : "启用后该批次兑换码恢复正常兑换。"
                            }
                            confirmText={b.status === "active" ? "停用" : "启用"}
                            destructive={b.status === "active"}
                            onConfirm={() => handleToggle(b)}
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
