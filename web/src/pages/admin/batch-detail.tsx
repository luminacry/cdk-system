import { useCallback, useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { ArrowLeft, Copy, Download } from "lucide-react"
import { toast } from "sonner"

import { api } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import type { Batch, BatchCounts, CodeItem } from "@/lib/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataPagination } from "@/components/data-pagination"
import { EmptyState } from "@/components/empty-state"
import { JsonViewer } from "@/components/json-viewer"
import { PageHeader } from "@/components/page-header"
import { BatchStatusBadge, CodeStatusBadge } from "@/components/status-badge"
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
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"

const STATE_TABS = [
  { value: "all", label: "全部" },
  { value: "active", label: "可用" },
  { value: "partused", label: "部分使用" },
  { value: "used", label: "已用完" },
  { value: "disabled", label: "已禁用" },
]

function InfoItem({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 text-sm font-medium break-all">{children}</dd>
    </div>
  )
}

export function BatchDetailPage() {
  const { id } = useParams<{ id: string }>()
  const batchId = Number(id)

  const [batch, setBatch] = useState<Batch | null>(null)
  const [counts, setCounts] = useState<BatchCounts | null>(null)
  const [notFound, setNotFound] = useState(false)

  const [state, setState] = useState("all")
  const [page, setPage] = useState(1)
  const [codes, setCodes] = useState<CodeItem[] | null>(null)
  const [paged, setPaged] = useState({ total: 0, page: 1, pages: 1 })
  const [maxUses, setMaxUses] = useState(1)

  const loadBatch = useCallback(() => {
    api
      .batch(batchId)
      .then((d) => {
        setBatch(d.batch)
        setCounts(d.counts)
      })
      .catch((err) => {
        toast.error(err.message)
        setNotFound(true)
      })
  }, [batchId])

  const loadCodes = useCallback(() => {
    setCodes(null)
    api
      .batchCodes(batchId, state, page)
      .then((d) => {
        setCodes(d.codes)
        setPaged({ total: d.total, page: d.page, pages: d.pages })
        setMaxUses(d.max_uses)
      })
      .catch((err) => toast.error(err.message))
  }, [batchId, state, page])

  useEffect(loadBatch, [loadBatch])
  useEffect(loadCodes, [loadCodes])

  async function handleToggleBatch() {
    if (!batch) return
    try {
      const res = await api.toggleBatch(batch.id)
      toast.success(res.message)
      loadBatch()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败")
    }
  }

  async function handleToggleCode(codeId: number) {
    try {
      const res = await api.toggleCode(codeId)
      toast.success(res.message)
      loadCodes()
      loadBatch()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败")
    }
  }

  if (notFound) {
    return (
      <EmptyState
        title="批次不存在"
        description="它可能已被删除，返回批次列表查看"
      />
    )
  }

  if (!batch || !counts) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-9 w-64" />
        <Skeleton className="h-64 rounded-xl" />
        <Skeleton className="h-96 rounded-xl" />
      </div>
    )
  }

	const payload = batch.payload_json

  return (
    <div className="space-y-6">
      <PageHeader
        title={batch.name}
        description={batch.description || `批次 #${batch.id}`}
        actions={
          <>
            <Button
              variant="outline"
              nativeButton={false}
              render={
                <Link to="/admin/batches">
                  <ArrowLeft className="size-4" />
                  返回列表
                </Link>
              }
            />
            <Button
              variant="outline"
              nativeButton={false}
              render={
                <a href={api.exportUrl(batch.id, "txt")}>
                  <Download className="size-4" />
                  导出 TXT
                </a>
              }
            />
            <Button
              variant="outline"
              nativeButton={false}
              render={
                <a href={api.exportUrl(batch.id, "csv")}>
                  <Download className="size-4" />
                  导出 CSV
                </a>
              }
            />
            <ConfirmDialog
              trigger={
                <Button variant={batch.status === "active" ? "destructive" : "default"}>
                  {batch.status === "active" ? "停用批次" : "启用批次"}
                </Button>
              }
              title={`确定要${batch.status === "active" ? "停用" : "启用"}该批次吗？`}
              description={
                batch.status === "active"
                  ? "停用后该批次所有兑换码将无法兑换。"
                  : "启用后该批次兑换码恢复正常兑换。"
              }
              confirmText={batch.status === "active" ? "停用" : "启用"}
              destructive={batch.status === "active"}
              onConfirm={handleToggleBatch}
            />
          </>
        }
      />

      {/* 批次信息 */}
      <Card>
        <CardHeader>
          <CardTitle>批次信息</CardTitle>
          <CardDescription>规则、用量与 Webhook 配置</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
            <InfoItem label="状态">
              <BatchStatusBadge status={batch.status} />
            </InfoItem>
            <InfoItem label="码总数">
              <span className="font-mono">{counts.total}</span>
            </InfoItem>
            <InfoItem label="已用完">
              <span className="font-mono">{counts.usedup ?? 0}</span>
            </InfoItem>
            <InfoItem label="累计兑换次数">
              <span className="font-mono">{counts.total_uses}</span>
            </InfoItem>
            <InfoItem label="已禁用">
              <span className="font-mono">{counts.disabled ?? 0}</span>
            </InfoItem>
            <InfoItem label="前缀">
              <span className="font-mono">{batch.prefix || "—"}</span>
            </InfoItem>
            <InfoItem label="过期时间">
              <span className="font-mono text-xs">
                {batch.expires_at ? batch.expires_at.replace("T", " ") : "永久"}
              </span>
            </InfoItem>
            <InfoItem label="单码次数">
              <span className="font-mono">{batch.max_uses_per_code}</span>
            </InfoItem>
            <InfoItem label="每邮箱限兑">
              <span className="font-mono">{batch.max_redeems_per_user}</span>
            </InfoItem>
            <InfoItem label="凭证分配">
              <span>{batch.assign_credential ? batch.credential_plan_type : "未启用"}</span>
            </InfoItem>
            <InfoItem label="创建时间">
              <span className="font-mono text-xs">{batch.created_at}</span>
            </InfoItem>
          </dl>

          {batch.webhook_url ? (
            <div className="rounded-lg border bg-muted/40 p-3 text-sm">
              <div className="mb-1 flex items-center gap-2">
                <Badge variant="outline">Webhook</Badge>
                <span className="font-mono text-xs break-all">{batch.webhook_url}</span>
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                签名密钥：
                <code className="font-mono">{batch.webhook_secret}</code>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  title="复制密钥"
                  onClick={() => copyText(batch.webhook_secret, "密钥已复制")}
                >
                  <Copy className="size-3" />
                </Button>
              </div>
            </div>
          ) : null}

          <div>
            <div className="mb-2 flex items-center justify-between">
              <span className="text-sm font-medium">分发内容（JSON）</span>
              <Button
                variant="outline"
                size="xs"
                onClick={() => copyText(JSON.stringify(payload, null, 2), "JSON 已复制")}
              >
                <Copy className="size-3.5" />
                复制
              </Button>
            </div>
            <JsonViewer data={payload} className="max-h-64 overflow-y-auto" />
          </div>
        </CardContent>
      </Card>

      {/* 兑换码列表 */}
      <Card className="min-w-[44rem]">
        <CardHeader className="gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <CardTitle>兑换码</CardTitle>
              <CardDescription>共 {paged.total} 个</CardDescription>
            </div>
            <Tabs
              value={state}
              onValueChange={(v) => {
                setState(v)
                setPage(1)
              }}
            >
              <TabsList>
                {STATE_TABS.map((t) => (
                  <TabsTrigger key={t.value} value={t.value}>
                    {t.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </div>
        </CardHeader>
        <CardContent>
          {codes === null ? (
            <Skeleton className="h-64 w-full rounded-lg" />
          ) : codes.length === 0 ? (
            <EmptyState title="该筛选条件下没有兑换码" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>兑换码</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>使用次数</TableHead>
                    <TableHead>创建时间</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {codes.map((c) => (
                    <TableRow key={c.id}>
                      <TableCell>
                        <button
                          className="font-mono text-sm hover:text-primary"
                          title="点击复制"
                          onClick={() => copyText(c.code, "已复制：" + c.code)}
                        >
                          {c.code}
                        </button>
                      </TableCell>
                      <TableCell>
                        <CodeStatusBadge
                          status={c.status}
                          useCount={c.use_count}
                          maxUses={maxUses}
                        />
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        {c.use_count} / {maxUses}
                      </TableCell>
                      <TableCell className="font-mono text-xs whitespace-nowrap">
                        {c.created_at}
                      </TableCell>
                      <TableCell className="text-right">
                        <ConfirmDialog
                          trigger={
                            <Button
                              variant={c.status === "unused" ? "outline" : "default"}
                              size="sm"
                            >
                              {c.status === "unused" ? "禁用" : "恢复"}
                            </Button>
                          }
                          title={`确定要${c.status === "unused" ? "禁用" : "恢复"}兑换码 ${c.display} 吗？`}
                          confirmText={c.status === "unused" ? "禁用" : "恢复"}
                          destructive={c.status === "unused"}
                          onConfirm={() => handleToggleCode(c.id)}
                        />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
          <DataPagination
            page={paged.page}
            pages={paged.pages}
            total={paged.total}
            onPage={setPage}
          />
        </CardContent>
      </Card>
    </div>
  )
}
