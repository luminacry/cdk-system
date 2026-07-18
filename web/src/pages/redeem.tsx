import { useEffect, useRef, useState } from "react"
import {
  CircleCheck,
  CircleMinus,
  CircleX,
  ClipboardPaste,
  Copy,
  Download,
  FileJson,
  History,
  Loader2,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import {
  buildSummary,
  createMergedJson,
  createMergedZip,
  createSingleZip,
  detectAndParse,
  type ConvertSummary,
} from "@/lib/converter"
import type { RedeemBatchItem, RedeemSuccess } from "@/lib/types"
import { SiteLogo } from "@/components/site-settings-provider"
import { ThemeToggle } from "@/components/theme-toggle"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { JsonViewer } from "@/components/json-viewer"
import { Skeleton } from "@/components/ui/skeleton"
import { ThemeBackground } from "@/components/ui/theme-background"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { useSiteSettings } from "@/lib/site-settings-context"

const MAX_BATCH_REDEEM_CODES = 20

function normalizeCdk(value: string): string {
  return value.replace(/[^a-zA-Z0-9]/g, "").toUpperCase()
}

function parseCdkList(value: string): string[] {
  const seen = new Set<string>()
  const codes: string[] = []
  for (const part of value.split(/[\s,，;；]+/)) {
    const code = normalizeCdk(part)
    if (!code || seen.has(code)) continue
    seen.add(code)
    codes.push(code)
  }
  return codes
}

function isValidEmail(value: string): boolean {
  return value.length <= 254 && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)
}

const REDEEM_HISTORY_KEY = "cdk-redeem-history-v2"
const LEGACY_REDEEM_HISTORY_KEY = "cdk-redeem-history-v1"
const REDEEM_HISTORY_LIMIT = 20

interface RedeemHistoryEntry {
  id: string
  saved_at: string
  result: RedeemSuccess
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function isRedeemSuccess(value: unknown): value is RedeemSuccess {
  if (!isRecord(value)) return false
  return (
    value.ok === true &&
    value.result === "success" &&
    typeof value.message === "string" &&
    typeof value.batch === "string" &&
    typeof value.code === "string" &&
    typeof value.user_id === "string" &&
    typeof value.redeemed_at === "string" &&
    Object.prototype.hasOwnProperty.call(value, "payload") &&
    (value.credential === undefined || isRecord(value.credential))
  )
}

function loadRedeemHistory(): RedeemHistoryEntry[] {
  try {
    localStorage.removeItem(LEGACY_REDEEM_HISTORY_KEY)
  } catch {
    // Storage may be disabled; history remains available in memory for this page view.
  }

  try {
    const raw = sessionStorage.getItem(REDEEM_HISTORY_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter((entry): entry is RedeemHistoryEntry =>
        isRecord(entry) &&
        typeof entry.id === "string" &&
        typeof entry.saved_at === "string" &&
        isRedeemSuccess(entry.result),
      )
      .slice(0, REDEEM_HISTORY_LIMIT)
  } catch {
    return []
  }
}

function persistRedeemHistory(entries: RedeemHistoryEntry[]): boolean {
  try {
    sessionStorage.setItem(REDEEM_HISTORY_KEY, JSON.stringify(entries))
    return true
  } catch {
    return false
  }
}

function createHistoryEntry(result: RedeemSuccess): RedeemHistoryEntry {
  return {
    id: globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(36).slice(2)}`,
    saved_at: new Date().toISOString(),
    result,
  }
}

interface FaqItem {
  title: string
  content: string
}

const FORMAT_OPTIONS = [
  { value: "single_zip", label: "sub2api 单账号 ZIP（每个账号一个 JSON）" },
  { value: "merged_zip", label: "sub2api 合并 ZIP（一个合并 JSON）" },
  { value: "merged_json", label: "sub2api 合并 JSON（直接下载 .json）" },
]

export function RedeemPage() {
  const { settings } = useSiteSettings()
  const [faq, setFaq] = useState<FaqItem[] | null>(null)
  const [faqError, setFaqError] = useState<string | null>(null)
  const [history, setHistory] = useState<RedeemHistoryEntry[]>(loadRedeemHistory)
  const historyRef = useRef(history)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [selectedResult, setSelectedResult] = useState<RedeemSuccess | null>(null)

  useEffect(() => {
    api.publicFaq()
      .then((d) => setFaq(d.items))
      .catch((err) => setFaqError(err instanceof Error ? err.message : "加载失败"))
  }, [])

  function commitHistory(next: RedeemHistoryEntry[]) {
    historyRef.current = next
    setHistory(next)
    if (!persistRedeemHistory(next)) {
      toast.warning("历史记录未能保存在当前标签页，但不影响本次兑换")
    }
  }

  function handleRedeemSuccess(result: RedeemSuccess, reveal = true) {
    const next = [createHistoryEntry(result), ...historyRef.current].slice(0, REDEEM_HISTORY_LIMIT)
    commitHistory(next)
    if (reveal) setSelectedResult(result)
  }

  function handleDeleteHistory(id: string) {
    commitHistory(historyRef.current.filter((entry) => entry.id !== id))
  }

  function handleClearHistory() {
    commitHistory([])
  }

  return (
    <ThemeBackground className="flex items-center justify-center px-4 py-10" shaderVariant="mesh">
      <div className="w-[30rem] max-w-full">
        <Card className="w-full bg-card/80 shadow-2xl backdrop-blur-sm">
          <CardHeader className="items-center text-center">
            <div className="mb-1 flex w-full items-center justify-between">
              <div className="flex size-11 items-center justify-center overflow-hidden rounded-xl bg-muted text-foreground">
                <SiteLogo className="size-6" />
              </div>
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  title="兑换历史"
                  onClick={() => setHistoryOpen(true)}
                >
                  <History className="size-4" />
                  <span className="sr-only">打开兑换历史</span>
                </Button>
                <ThemeToggle />
              </div>
            </div>
            <CardTitle className="max-w-full break-words text-2xl">{settings.site_title}</CardTitle>
            <CardDescription>兑换 CDK 或转换账号格式</CardDescription>
          </CardHeader>

          <CardContent>
            <Tabs defaultValue="redeem" className="w-full">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="redeem">兑换 CDK</TabsTrigger>
                <TabsTrigger value="convert">格式转换</TabsTrigger>
              </TabsList>

              <TabsContent value="redeem">
                <RedeemTab onSuccess={handleRedeemSuccess} />
              </TabsContent>

              <TabsContent value="convert">
                <ConvertTab />
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>

        {/* 兑换须知 */}
        <Card className="mt-6 w-full bg-card/80 backdrop-blur-sm">
          <CardHeader>
            <CardTitle className="text-base">兑换须知</CardTitle>
          </CardHeader>
          <CardContent>
            {faq === null ? (
              <div className="space-y-2">
                <Skeleton className="h-8 w-full" />
                <Skeleton className="h-8 w-full" />
                <Skeleton className="h-8 w-full" />
              </div>
            ) : faqError ? (
              <p className="text-sm text-destructive">{faqError}</p>
            ) : (
              <Accordion className="w-full">
                {faq.map((item, i) => (
                  <AccordionItem key={i} value={`item-${i}`}>
                    <AccordionTrigger className="text-sm">{item.title}</AccordionTrigger>
                    <AccordionContent className="text-muted-foreground">{item.content}</AccordionContent>
                  </AccordionItem>
                ))}
              </Accordion>
            )}
          </CardContent>
        </Card>

        <p className="mt-6 text-center text-xs text-muted-foreground">
          {settings.site_title} · 每个兑换码请在有效期内使用
        </p>
      </div>

      <RedeemHistoryDialog
        open={historyOpen}
        entries={history}
        onOpenChange={setHistoryOpen}
        onSelect={(result) => {
          setHistoryOpen(false)
          setSelectedResult(result)
        }}
        onDelete={handleDeleteHistory}
        onClear={handleClearHistory}
      />
      <RedeemResultDialog result={selectedResult} onOpenChange={(open) => !open && setSelectedResult(null)} />
    </ThemeBackground>
  )
}

/** 兑换 CDK 子模块 */
function RedeemTab({ onSuccess }: { onSuccess: (result: RedeemSuccess, reveal?: boolean) => void }) {
  const [userId, setUserId] = useState("")
  const [codeInput, setCodeInput] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [batchResults, setBatchResults] = useState<RedeemBatchItem[]>([])

  const codes = parseCdkList(codeInput)

  async function handlePaste() {
    try {
      const text = await navigator.clipboard.readText()
      if (text) setCodeInput(text)
    } catch {
      setError("无法读取剪贴板，请手动粘贴")
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const uid = userId.trim()
    const redeemCodes = parseCdkList(codeInput)
    if (!uid) {
      setError("请输入邮箱")
      return
    }
    if (!isValidEmail(uid)) {
      setError("邮箱格式不正确，请检查后重试")
      return
    }
    if (redeemCodes.length === 0) {
      setError("请输入兑换码")
      return
    }
    if (redeemCodes.length > MAX_BATCH_REDEEM_CODES) {
      setError(`单次最多兑换 ${MAX_BATCH_REDEEM_CODES} 个兑换码`)
      return
    }
    setLoading(true)
    setError(null)
    setBatchResults([])
    try {
      if (redeemCodes.length === 1) {
        const data = await api.redeem(uid, redeemCodes[0])
        setCodeInput("")
        onSuccess(data)
      } else {
        const data = await api.redeemBatch(uid, redeemCodes)
        setBatchResults(data.results)
        for (const item of data.results) {
          if (item.redemption) onSuccess(item.redemption, false)
        }
        if (data.failed === 0) {
          setCodeInput("")
          toast.success(`${data.succeeded} 个兑换码全部兑换成功`)
        } else {
          const failedCodes = data.results.filter((item) => !item.ok).map((item) => item.code)
          setCodeInput(failedCodes.join("\n"))
        }
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "网络异常，请检查网络后重试")
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4" noValidate>
        <div className="space-y-2">
          <Label htmlFor="user-id">邮箱 *</Label>
          <Input
            id="user-id"
            name="email"
            type="email"
            inputMode="email"
            placeholder="name@example.com"
            value={userId}
            maxLength={254}
            onChange={(e) => {
              setUserId(e.target.value)
              if (error) setError(null)
            }}
            autoComplete="email"
          />
          <p className="text-xs text-muted-foreground">仅用于记录和核对兑换</p>
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <Label htmlFor="cdk-code">兑换码（支持批量）</Label>
            <Button
              type="button"
              variant="ghost"
              size="xs"
              onClick={handlePaste}
              className="text-muted-foreground"
            >
              <ClipboardPaste className="size-3.5" />
              粘贴
            </Button>
          </div>
          <Textarea
            id="cdk-code"
            placeholder={"每行一个兑换码\nXXXX-XXXX-XXXX\nYYYY-YYYY-YYYY"}
            value={codeInput}
            rows={codes.length > 1 ? Math.min(8, Math.max(4, codes.length)) : 3}
            maxLength={MAX_BATCH_REDEEM_CODES * 140}
            onChange={(e) => {
              setCodeInput(e.target.value)
              if (error) setError(null)
              if (batchResults.length) setBatchResults([])
            }}
            className="resize-y font-mono text-sm uppercase"
            autoComplete="off"
            spellCheck={false}
          />
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>每行、空格或逗号分隔，重复兑换码自动去重</span>
            <span className={codes.length > MAX_BATCH_REDEEM_CODES ? "text-destructive" : ""}>
              {codes.length}/{MAX_BATCH_REDEEM_CODES}
            </span>
          </div>
        </div>

        {error ? (
          <Alert variant="destructive">
            <CircleX className="size-4" />
            <AlertTitle>兑换失败</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        {batchResults.length > 0 ? (
          <div className="space-y-2" aria-live="polite">
            <div className="flex items-center justify-between text-sm font-medium">
              <span>批量兑换结果</span>
              <span className="text-xs text-muted-foreground">
                成功 {batchResults.filter((item) => item.ok).length} · 失败 {batchResults.filter((item) => !item.ok).length}
              </span>
            </div>
            <div className="max-h-64 overflow-y-auto rounded-lg border">
              {batchResults.map((item) => (
                <div key={item.code} className="flex items-start gap-2 border-b px-3 py-2.5 last:border-b-0">
                  {item.ok ? (
                    <CircleCheck className="mt-0.5 size-4 shrink-0 text-emerald-500" />
                  ) : (
                    <CircleMinus className="mt-0.5 size-4 shrink-0 text-destructive" />
                  )}
                  <div className="min-w-0 flex-1">
                    <div className="break-all font-mono text-xs font-medium">{item.code}</div>
                    <div className="mt-0.5 text-xs text-muted-foreground">{item.message}</div>
                  </div>
                  {item.redemption ? (
                    <Button type="button" size="xs" variant="ghost" onClick={() => onSuccess(item.redemption!)}>
                      查看
                    </Button>
                  ) : null}
                </div>
              ))}
            </div>
          </div>
        ) : null}

        <Button type="submit" size="lg" className="w-full" disabled={loading}>
          {loading ? <Loader2 className="size-4 animate-spin" /> : null}
          {loading ? "兑换中…" : codes.length > 1 ? `批量兑换 ${codes.length} 个` : "立即兑换"}
        </Button>
    </form>
  )
}

interface RedeemHistoryDialogProps {
  open: boolean
  entries: RedeemHistoryEntry[]
  onOpenChange: (open: boolean) => void
  onSelect: (result: RedeemSuccess) => void
  onDelete: (id: string) => void
  onClear: () => void
}

function RedeemHistoryDialog({
  open,
  entries,
  onOpenChange,
  onSelect,
  onDelete,
  onClear,
}: RedeemHistoryDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>兑换历史</DialogTitle>
          <DialogDescription>
            仅保存在当前标签页，刷新后仍可查看，关闭标签页后自动清除。最多保留最近 20 条。
          </DialogDescription>
        </DialogHeader>

        {entries.length === 0 ? (
          <div className="flex min-h-44 flex-col items-center justify-center gap-2 text-center text-muted-foreground">
            <History className="size-8" aria-hidden="true" />
            <p className="text-sm font-medium text-foreground">暂无兑换历史</p>
            <p className="text-xs">成功兑换后，记录会显示在这里。</p>
          </div>
        ) : (
          <div className="max-h-[60svh] overflow-y-auto rounded-lg border">
            {entries.map((entry) => (
              <div key={entry.id} className="flex items-center border-b last:border-b-0">
                <button
                  type="button"
                  className="min-w-0 flex-1 px-3 py-3 text-left outline-none transition-colors hover:bg-muted/60 focus-visible:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50"
                  onClick={() => onSelect(entry.result)}
                >
                  <div className="flex min-w-0 items-center justify-between gap-3">
                    <span className="truncate font-mono text-sm font-medium">{entry.result.code}</span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {entry.result.redeemed_at || entry.saved_at}
                    </span>
                  </div>
                  <p className="mt-1 truncate text-sm">{entry.result.batch}</p>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">
                    {entry.result.user_id}
                  </p>
                </button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="mr-2 shrink-0 text-muted-foreground hover:text-destructive"
                  title="删除这条历史记录"
                  onClick={() => onDelete(entry.id)}
                >
                  <Trash2 className="size-4" />
                  <span className="sr-only">删除这条历史记录</span>
                </Button>
              </div>
            ))}
          </div>
        )}

        <DialogFooter className="gap-2 sm:justify-between">
          {entries.length > 0 ? (
            <ConfirmDialog
              trigger={
                <Button type="button" variant="destructive">
                  <Trash2 className="size-4" />
                  清空历史
                </Button>
              }
              title="清空全部兑换历史？"
              description="这只会删除当前浏览器中保存的历史记录，且无法恢复。"
              confirmText="确认清空"
              destructive
              onConfirm={onClear}
            />
          ) : (
            <span />
          )}
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function serializeJson(data: unknown): string {
  return JSON.stringify(data, null, 2) ?? "null"
}

function downloadJson(data: unknown, filename: string) {
  const blob = new Blob([serializeJson(data)], { type: "application/json" })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement("a")
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}

function formatExpiry(value: unknown): string {
  if (!value) return "—"
  const date = new Date(String(value))
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString("zh-CN")
}

function RedeemResultDialog({
  result,
  onOpenChange,
}: {
  result: RedeemSuccess | null
  onOpenChange: (open: boolean) => void
}) {
  const hasPayload = !!result && result.payload !== undefined

  return (
    <Dialog open={!!result} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-md">
        {result ? (
          <>
            <DialogHeader className="items-center">
              <div className="mb-1 flex size-14 items-center justify-center rounded-full bg-emerald-500/15">
                <CircleCheck className="size-8 text-emerald-500" />
              </div>
              <DialogTitle className="text-xl">兑换成功</DialogTitle>
              <DialogDescription>
                {result.batch} · {result.redeemed_at}
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4">
              <div className="grid grid-cols-[5rem_minmax(0,1fr)] gap-y-1.5 rounded-lg border bg-muted/40 p-3 text-sm">
                <span className="text-muted-foreground">兑换码</span>
                <span className="break-all font-mono">{result.code}</span>
                <span className="text-muted-foreground">邮箱</span>
                <span className="break-all">{result.user_id}</span>
              </div>

              {hasPayload ? (
                <div className="space-y-2">
                  <div className="text-sm font-medium">兑换内容</div>
                  <JsonViewer data={result.payload} className="max-h-64 overflow-y-auto" />
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        copyText(serializeJson(result.payload), "兑换内容已复制")
                      }
                    >
                      <Copy className="size-4" />
                      复制内容
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => downloadJson(result.payload, `redemption-${result.code}.json`)}
                    >
                      <Download className="size-4" />
                      下载内容
                    </Button>
                  </div>
                </div>
              ) : null}

              {result.credential ? (
                <div className="space-y-2">
                  <div className="text-sm font-medium">分配到的凭证</div>
                  <div className="grid grid-cols-[5rem_minmax(0,1fr)] gap-y-1.5 rounded-lg border bg-muted/40 p-3 text-sm">
                    <span className="text-muted-foreground">邮箱</span>
                    <span className="break-all">{String(result.credential.email ?? "—")}</span>
                    <span className="text-muted-foreground">套餐</span>
                    <span>{String(result.credential.plan_type ?? "—")}</span>
                    <span className="text-muted-foreground">过期时间</span>
                    <span className="break-all font-mono text-xs">
                      {formatExpiry(result.credential.expired)}
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        copyText(JSON.stringify(result.credential, null, 2), "凭证已复制")
                      }
                    >
                      <Copy className="size-4" />
                      复制凭证
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        downloadJson(result.credential, `credential-${result.code}.json`)
                      }
                    >
                      <Download className="size-4" />
                      下载凭证
                    </Button>
                  </div>
                </div>
              ) : null}
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                完成
              </Button>
            </DialogFooter>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

/** 格式转换子模块 */
function ConvertTab() {
  const [text, setText] = useState("")
  const [format, setFormat] = useState("single_zip")
  const [preview, setPreview] = useState<{
    format: string
    total: number
    summary: ConvertSummary[]
  } | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  async function detect() {
    setError(null)
    setPreview(null)
    if (!text.trim()) return
    try {
      const { accounts, format } = detectAndParse(text)
      setPreview({
        format,
        total: accounts.length,
        summary: buildSummary(accounts),
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : "识别失败")
    }
  }

  async function handleDownload() {
    if (!text.trim() || !preview || preview.total === 0) {
      setError("请先输入有效内容并完成识别")
      return
    }
    setLoading(true)
    setError(null)
    try {
      const { accounts } = detectAndParse(text)
      let blob: Blob
      let filename: string
      switch (format) {
        case "single_zip":
          ({ blob, filename } = await createSingleZip(accounts))
          break
        case "merged_zip":
          ({ blob, filename } = await createMergedZip(accounts))
          break
        case "merged_json":
          ;({ blob, filename } = createMergedJson(accounts))
          break
        default:
          throw new Error("不支持的输出格式")
      }
      triggerDownload(blob, filename)
    } catch (err) {
      setError(err instanceof Error ? err.message : "下载失败")
    } finally {
      setLoading(false)
    }
  }

  function triggerDownload(blob: Blob, filename: string) {
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      setText(String(reader.result || ""))
    }
    reader.onerror = () => setError("文件读取失败")
    reader.readAsText(file)
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    const file = e.dataTransfer.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => setText(String(reader.result || ""))
    reader.readAsText(file)
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <Label htmlFor="convert-input">原始账号数据</Label>
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="xs"
              onClick={() => fileInputRef.current?.click()}
              className="text-muted-foreground"
            >
              <FileJson className="size-3.5" />
              上传文件
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="xs"
              onClick={async () => {
                try {
                  const t = await navigator.clipboard.readText()
                  if (t) setText(t)
                } catch {
                  setError("无法读取剪贴板")
                }
              }}
              className="text-muted-foreground"
            >
              <ClipboardPaste className="size-3.5" />
              粘贴
            </Button>
          </div>
        </div>
        <input
          ref={fileInputRef}
          type="file"
          accept=".json,.txt"
          className="hidden"
          onChange={handleFileChange}
        />
        <Textarea
          id="convert-input"
          ref={textareaRef}
          placeholder={`支持 JSON 数组 或 JSONL（每行一个对象），例如：\n[{"email":"a@b.com","access_token":"eyJ...",...}]\n或\n{"email":"a@b.com","access_token":"eyJ..."}\n{"email":"c@d.com","access_token":"eyJ..."}`}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onDrop={handleDrop}
          onDragOver={(e) => e.preventDefault()}
          className="font-mono text-sm h-48 max-h-48 resize-y overflow-auto"
        />
        <p className="text-xs text-muted-foreground">可拖拽 .json / .txt 文件到上方文本框，或直接粘贴内容</p>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Button type="button" variant="outline" onClick={detect}>自动识别格式</Button>
        <Select value={format} onValueChange={(v) => v && setFormat(v)}>
          <SelectTrigger className="min-w-[16rem]">
            <SelectValue placeholder="选择输出格式" />
          </SelectTrigger>
          <SelectContent>
            {FORMAT_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {preview ? (
        <Alert>
          <FileJson className="size-4" />
          <AlertTitle>识别成功</AlertTitle>
          <AlertDescription className="break-all">
            格式：{preview.format} · 共 {preview.total} 个账号
            {preview.summary.length > 0 ? (
              <>
                <br />
                前 {Math.min(preview.summary.length, 10)} 条：
                {preview.summary.slice(0, 10).map((s) => s.email || s.account_id || `#${s.index}`).join(", ")}
              </>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}

      {error ? (
        <Alert variant="destructive">
          <CircleX className="size-4" />
          <AlertTitle>转换失败</AlertTitle>
          <AlertDescription className="break-all">{error}</AlertDescription>
        </Alert>
      ) : null}

      <Button
        type="button"
        className="w-full"
        onClick={handleDownload}
        disabled={loading || !preview || preview.total === 0}
      >
        {loading ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
        {loading ? "转换中…" : "转换并下载"}
      </Button>
    </div>
  )
}
