import { useCallback, useEffect, useRef, useState } from "react"
import {
  CircleX,
  ChevronLeft,
  ChevronRight,
  Download,
  Ellipsis,
  Eye,
  FileJson,
  Loader2,
  RefreshCw,
  Trash2,
  Upload,
} from "lucide-react"
import { toast } from "sonner"

import { api } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import type { Credential, CredentialPlan } from "@/lib/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { JsonViewer } from "@/components/json-viewer"
import { PageHeader } from "@/components/page-header"
import { StatCard } from "@/components/stat-card"
import { Badge } from "@/components/ui/badge"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

const STATE_TABS = [
  { value: "all", label: "全部" },
  { value: "unused", label: "未使用" },
  { value: "used", label: "已使用" },
]

const PAGE_SIZE_OPTIONS = [20, 50, 100, 200, 500] as const

type CredentialState = "all" | "used" | "unused"
type CredentialPageSize = (typeof PAGE_SIZE_OPTIONS)[number]
type PageItem = number | "start-ellipsis" | "end-ellipsis"
type UploadPlanMode = "existing" | "new"

function containsControlCharacter(value: string): boolean {
  return Array.from(value).some((character) => {
    const codePoint = character.codePointAt(0) ?? 0
    return codePoint <= 0x1f || codePoint === 0x7f
  })
}

function compactPageItems(page: number, pages: number): PageItem[] {
  if (pages <= 0) return []
  if (pages <= 5) return Array.from({ length: pages }, (_, index) => index + 1)
  if (page <= 3) return [1, 2, 3, "end-ellipsis", pages]
  if (page >= pages - 2) return [1, "start-ellipsis", pages - 2, pages - 1, pages]
  return [1, "start-ellipsis", page, "end-ellipsis", pages]
}

function formatExpired(value: string | null): string {
  if (!value) return "—"
  try {
    return new Date(value).toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    })
  } catch {
    return value
  }
}

export function CredentialsPage() {
  const [state, setState] = useState<CredentialState>("all")
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<CredentialPageSize>(50)
  const [items, setItems] = useState<Credential[] | null>(null)
  const [paged, setPaged] = useState({ total: 0, page: 1, pages: 1 })
  const [stats, setStats] = useState({ total: 0, used: 0, unused: 0 })
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [uploading, setUploading] = useState(false)
  const [plans, setPlans] = useState<CredentialPlan[]>([])
  const [plansLoading, setPlansLoading] = useState(true)
  const [plansError, setPlansError] = useState<string | null>(null)
  const [pendingUpload, setPendingUpload] = useState<File | null>(null)
  const [uploadPlanMode, setUploadPlanMode] = useState<UploadPlanMode>("existing")
  const [selectedPlan, setSelectedPlan] = useState("")
  const [newPlan, setNewPlan] = useState("")
  const [uploadError, setUploadError] = useState<string | null>(null)
  const [detail, setDetail] = useState<Credential | null>(null)
  const [loadingDetailId, setLoadingDetailId] = useState<number | null>(null)
  const [downloading, setDownloading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)

  const loadStats = useCallback(() => {
    api.credentialStats().then((d) => setStats(d)).catch((err) => toast.error(err.message))
  }, [])

  const loadPlans = useCallback(async () => {
    setPlansLoading(true)
    setPlansError(null)
    try {
      const response = await api.credentialPlans()
      setPlans(response.plans ?? [])
    } catch (err) {
      setPlans([])
      setPlansError(err instanceof Error ? err.message : "套餐列表加载失败")
    } finally {
      setPlansLoading(false)
    }
  }, [])

  const load = useCallback(() => {
    setItems(null)
    api
      .credentials(state, page, pageSize)
      .then((d) => {
        if (d.total > 0 && page > d.pages) {
          setPage(d.pages)
          return
        }
        setItems(d.items)
        setPaged({ total: d.total, page: d.page, pages: d.pages })
        setSelected(new Set())
      })
      .catch((err) => toast.error(err.message))
  }, [state, page, pageSize])

  useEffect(() => {
    loadStats()
    void loadPlans()
  }, [loadPlans, loadStats])

  useEffect(() => {
    load()
  }, [load])

  function stageUpload(file: File) {
    if (plansLoading || plansError || uploading) return
    if (!file.name.toLowerCase().endsWith(".json")) {
      toast.error("请上传 .json 文件")
      return
    }
    setPendingUpload(file)
    setUploadPlanMode(plans.length > 0 ? "existing" : "new")
    setSelectedPlan("")
    setNewPlan("")
    setUploadError(null)
  }

  function closeUploadDialog() {
    if (uploading) return
    setPendingUpload(null)
    setUploadError(null)
  }

  async function handleUpload() {
    if (!pendingUpload) return
    const planType = (uploadPlanMode === "existing" ? selectedPlan : newPlan).trim()
    if (!planType) {
      setUploadError(uploadPlanMode === "existing" ? "请选择套餐" : "请输入新套餐名称")
      return
    }
    if (Array.from(planType).length > 100 || containsControlCharacter(planType)) {
      setUploadError("套餐名称必须为 1 至 100 个字符，且不能包含控制字符")
      return
    }
    setUploading(true)
    setUploadError(null)
    try {
      const res = await api.uploadCredentials(pendingUpload, planType)
      toast.success(res.message)
      setPendingUpload(null)
      loadStats()
      load()
      void loadPlans()
    } catch (err) {
      setUploadError(err instanceof Error ? err.message : "上传失败")
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ""
    }
  }

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (file) stageUpload(file)
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files?.[0]
    if (file) stageUpload(file)
  }

  async function handleDelete(id: number) {
    try {
      const res = await api.deleteCredential(id)
      toast.success(res.message)
      loadStats()
      load()
      void loadPlans()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败")
    }
  }

  async function handleViewDetail(id: number) {
    setLoadingDetailId(id)
    try {
      const res = await api.credential(id)
      setDetail(res.credential)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "获取凭证详情失败")
    } finally {
      setLoadingDetailId(null)
    }
  }

  async function handleDownload(id: number) {
    setDownloading(true)
    try {
      const blob = await api.downloadCredential(id)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = `credential-${id}.json`
      document.body.appendChild(anchor)
      anchor.click()
      document.body.removeChild(anchor)
      URL.revokeObjectURL(url)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "下载凭证失败")
    } finally {
      setDownloading(false)
    }
  }

  async function handleBatchDelete() {
    if (selected.size === 0) return
    try {
      const res = await api.batchDeleteCredentials(Array.from(selected))
      toast.success(res.message)
      loadStats()
      load()
      void loadPlans()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "批量删除失败")
    }
  }

  function toggleSelect(id: number) {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setSelected(next)
  }

  function toggleSelectAll() {
    if (!items) return
    if (selected.size === items.length && items.length > 0) {
      setSelected(new Set())
    } else {
      setSelected(new Set(items.map((i) => i.id)))
    }
  }

  const allSelected = !!items && items.length > 0 && selected.size === items.length
  const rangeStart = paged.total === 0 ? 0 : (paged.page - 1) * pageSize + 1
  const rangeEnd = Math.min(paged.page * pageSize, paged.total)
  const pageItems = compactPageItems(paged.page, paged.pages)
  const pageControlsDisabled = items === null

  return (
    <div className="space-y-6">
      <PageHeader
        title="凭证管理"
        description="批量上传 JSON 凭证，兑换成功后会自动分配未使用的凭证。"
        actions={
          <>
            <input
              ref={fileInputRef}
              type="file"
              accept=".json,application/json"
              className="hidden"
              onChange={handleFileChange}
            />
            <Button
              variant="outline"
              disabled={uploading || plansLoading || !!plansError}
              onClick={() => fileInputRef.current?.click()}
            >
              {uploading ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
              上传 JSON
            </Button>
            {selected.size > 0 ? (
              <ConfirmDialog
                trigger={
                  <Button variant="destructive">
                    <Trash2 className="size-4" />
                    批量删除 ({selected.size})
                  </Button>
                }
                title="确定要批量删除选中的凭证吗？"
                description={`将删除 ${selected.size} 条凭证，此操作不可恢复。`}
                confirmText="删除"
                destructive
                onConfirm={handleBatchDelete}
              />
            ) : null}
          </>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard title="凭证总数" value={stats.total} icon={FileJson} tone="default" />
        <StatCard title="未使用" value={stats.unused} icon={Upload} tone="primary" />
        <StatCard title="已使用" value={stats.used} icon={FileJson} tone="destructive" />
      </div>

      {plansError ? (
        <Alert variant="destructive">
          <CircleX className="size-4" />
          <AlertTitle>套餐列表加载失败</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{plansError}</span>
            <Button type="button" size="sm" variant="outline" onClick={() => void loadPlans()}>
              <RefreshCw className="size-3.5" />
              重新加载
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}

      <Card
        className={`border-dashed transition-colors ${plansLoading || plansError ? "opacity-60" : "cursor-pointer"} ${dragOver ? "border-primary bg-primary/5" : ""}`}
        onClick={() => {
          if (!plansLoading && !plansError && !uploading) fileInputRef.current?.click()
        }}
        onDragOver={(e) => {
          e.preventDefault()
          setDragOver(true)
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
      >
        <CardContent className="flex flex-col items-center justify-center gap-2 py-10 text-center">
          <div className="flex size-12 items-center justify-center rounded-full bg-muted">
            <Upload className="size-6 text-muted-foreground" />
          </div>
          <p className="text-sm font-medium">
            {plansLoading ? "正在加载套餐…" : uploading ? "正在导入凭证…" : "点击或拖拽 JSON 文件到此处上传"}
          </p>
          <p className="text-xs text-muted-foreground">上传前需选择已有套餐，或创建一个新套餐</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <CardTitle>凭证列表</CardTitle>
              <CardDescription>共 {paged.total} 条</CardDescription>
            </div>
            <Tabs
              value={state}
              onValueChange={(v) => {
                setState(v as CredentialState)
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
          {items === null ? (
            <Skeleton className="h-64 w-full rounded-lg" />
          ) : items.length === 0 ? (
            <EmptyState title="暂无凭证" description="上传 JSON 文件后在此处管理。" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <input
                        type="checkbox"
                        className="size-4"
                        checked={allSelected}
                        onChange={toggleSelectAll}
                        aria-label="选择当前页全部凭证"
                      />
                    </TableHead>
                    <TableHead>ID</TableHead>
                    <TableHead>邮箱</TableHead>
                    <TableHead>套餐</TableHead>
                    <TableHead>过期时间</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((c) => (
                    <TableRow key={c.id}>
                      <TableCell>
                        <input
                          type="checkbox"
                          className="size-4"
                          checked={selected.has(c.id)}
                          onChange={() => toggleSelect(c.id)}
                          aria-label={`选择凭证 ${c.id}`}
                        />
                      </TableCell>
                      <TableCell className="font-mono text-xs">{c.id}</TableCell>
                      <TableCell className="max-w-[16rem] truncate">{c.email}</TableCell>
                      <TableCell>
                        <Badge variant="outline">{c.plan_type}</Badge>
                      </TableCell>
                      <TableCell className="font-mono text-xs whitespace-nowrap">
                        {formatExpired(c.expired)}
                      </TableCell>
                      <TableCell>
                        {c.used ? (
                          <Badge variant="secondary">已使用</Badge>
                        ) : (
                          <Badge variant="default">未使用</Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            disabled={loadingDetailId === c.id}
                            onClick={() => handleViewDetail(c.id)}
                            aria-label={`查看凭证 ${c.id}`}
                          >
                            {loadingDetailId === c.id ? (
                              <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                              <Eye className="size-3.5" />
                            )}
                          </Button>
                          <ConfirmDialog
                            trigger={
                              <Button variant="ghost" size="icon-xs" aria-label={`删除凭证 ${c.id}`}>
                                <Trash2 className="size-3.5 text-destructive" />
                              </Button>
                            }
                            title="确定要删除该凭证吗？"
                            description={`邮箱：${c.email}，操作不可恢复。`}
                            confirmText="删除"
                            destructive
                            onConfirm={() => handleDelete(c.id)}
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
          <div className="mt-4 flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
              <span className="whitespace-nowrap tabular-nums">
                显示 {rangeStart}-{rangeEnd} / 共 {paged.total} 条
              </span>
              <span aria-hidden="true" className="hidden text-border sm:inline">·</span>
              <div className="flex items-center gap-1.5 whitespace-nowrap">
                <span>每页</span>
                <Select
                  value={String(pageSize)}
                  onValueChange={(value) => {
                    const next = Number(value) as CredentialPageSize
                    if (!PAGE_SIZE_OPTIONS.includes(next)) return
                    setPageSize(next)
                    setPage(1)
                  }}
                >
                  <SelectTrigger
                    size="sm"
                    className="w-[4.75rem]"
                    disabled={pageControlsDisabled}
                    aria-label="每页显示数量"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent align="start">
                    {PAGE_SIZE_OPTIONS.map((option) => (
                      <SelectItem key={option} value={String(option)}>
                        {option} 条
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {paged.total > 0 ? (
              <nav className="flex min-w-0 items-center justify-between gap-1 sm:justify-end" aria-label="凭证列表分页">
                <Button
                  variant="outline"
                  size="icon-sm"
                  disabled={pageControlsDisabled || paged.page <= 1}
                  onClick={() => setPage((current) => current - 1)}
                  aria-label="上一页"
                >
                  <ChevronLeft className="size-4" />
                </Button>

                <div className="flex min-w-0 items-center gap-1">
                  {pageItems.map((item) =>
                    typeof item === "number" ? (
                      <Button
                        key={item}
                        variant={item === paged.page ? "default" : "outline"}
                        size="icon-sm"
                        disabled={pageControlsDisabled}
                        onClick={() => setPage(item)}
                        aria-label={`第 ${item} 页`}
                        aria-current={item === paged.page ? "page" : undefined}
                        className="font-mono tabular-nums"
                      >
                        {item}
                      </Button>
                    ) : (
                      <span
                        key={item}
                        className="flex size-7 shrink-0 items-center justify-center text-muted-foreground"
                        aria-hidden="true"
                      >
                        <Ellipsis className="size-4" />
                      </span>
                    ),
                  )}
                </div>

                <Button
                  variant="outline"
                  size="icon-sm"
                  disabled={pageControlsDisabled || paged.page >= paged.pages}
                  onClick={() => setPage((current) => current + 1)}
                  aria-label="下一页"
                >
                  <ChevronRight className="size-4" />
                </Button>
              </nav>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <Dialog open={!!pendingUpload} onOpenChange={(open) => !open && closeUploadDialog()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>上传凭证</DialogTitle>
            <DialogDescription className="break-all">
              {pendingUpload?.name} · 上传内容将统一归入所选套餐
            </DialogDescription>
          </DialogHeader>

          <Tabs
            value={uploadPlanMode}
            onValueChange={(value) => {
              setUploadPlanMode(value as UploadPlanMode)
              setUploadError(null)
            }}
          >
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="existing" disabled={plans.length === 0}>已有套餐</TabsTrigger>
              <TabsTrigger value="new">新建套餐</TabsTrigger>
            </TabsList>
            <TabsContent value="existing" className="space-y-2">
              <Label htmlFor="upload-plan">选择套餐</Label>
              <Select value={selectedPlan} onValueChange={(value) => setSelectedPlan(value ?? "")}>
                <SelectTrigger id="upload-plan" className="w-full">
                  <SelectValue placeholder="请选择套餐" />
                </SelectTrigger>
                <SelectContent>
                  {plans.map((plan) => (
                    <SelectItem key={plan.plan_type} value={plan.plan_type}>
                      {plan.plan_type} · 可用 {plan.available}/{plan.total}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </TabsContent>
            <TabsContent value="new" className="space-y-2">
              <Label htmlFor="new-upload-plan">新套餐名称</Label>
              <Input
                id="new-upload-plan"
                value={newPlan}
                onChange={(event) => setNewPlan(event.target.value)}
                placeholder="例如：专业版年卡"
                autoComplete="off"
              />
              <p className="text-xs text-muted-foreground">首次上传或需要新的库存分组时使用。</p>
            </TabsContent>
          </Tabs>

          {uploadError ? (
            <Alert variant="destructive">
              <CircleX className="size-4" />
              <AlertTitle>上传失败</AlertTitle>
              <AlertDescription>{uploadError}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="button" variant="outline" disabled={uploading} onClick={closeUploadDialog}>
              取消
            </Button>
            <Button type="button" disabled={uploading} onClick={() => void handleUpload()}>
              {uploading ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
              {uploading ? "上传中…" : "确认上传"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!detail} onOpenChange={(open) => !open && setDetail(null)}>
        <DialogContent className="sm:max-w-lg">
          {detail && (
            <>
              <DialogHeader>
                <DialogTitle>凭证详情 #{detail.id}</DialogTitle>
                <DialogDescription>
                  {detail.email} · {detail.plan_type} · {detail.used ? "已使用" : "未使用"}
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-3">
                <div className="grid grid-cols-[5rem_1fr] gap-y-1 text-sm">
                  <span className="text-muted-foreground">过期时间</span>
                  <span>{formatExpired(detail.expired)}</span>
                  <span className="text-muted-foreground">兑换记录</span>
                  <span className="font-mono">{detail.redemption_id ?? "—"}</span>
                </div>
                <div className="flex gap-2">
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() =>
                      copyText(JSON.stringify(detail, null, 2), "凭证 JSON 已复制")
                    }
                  >
                    复制 JSON
                  </Button>
                  <Button
                    size="xs"
                    variant="outline"
                    disabled={downloading}
                    onClick={() => handleDownload(detail.id)}
                  >
                    {downloading ? (
                      <Loader2 className="mr-1 size-3 animate-spin" />
                    ) : (
                      <Download className="mr-1 size-3" />
                    )}
                    下载 JSON
                  </Button>
                </div>
                <JsonViewer data={detail} className="max-h-96 overflow-y-auto" />
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
