import { useCallback, useEffect, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { ArrowLeft, CircleX, Loader2, Plus, RefreshCw } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { BatchInput, CredentialPlan } from "@/lib/types"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"

function randomSecret(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("")
}

const INITIAL: BatchInput = {
  name: "",
  description: "",
  payload_json: "",
  count: 100,
  prefix: "",
  code_length: 12,
  expires_at: "",
  max_uses_per_code: 1,
  max_redeems_per_user: 1,
  webhook_url: "",
  webhook_secret: randomSecret(),
  assign_credential: false,
  credential_plan_type: "",
}

export function BatchNewPage() {
  const navigate = useNavigate()
  const [form, setForm] = useState<BatchInput>(INITIAL)
  const [loading, setLoading] = useState(false)
  const [errors, setErrors] = useState<string[]>([])
  const [plans, setPlans] = useState<CredentialPlan[]>([])
  const [plansLoading, setPlansLoading] = useState(true)
  const [plansError, setPlansError] = useState<string | null>(null)

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

  useEffect(() => {
    void loadPlans()
  }, [loadPlans])

  function set<K extends keyof BatchInput>(key: K, value: BatchInput[K]) {
    setForm((f) => ({ ...f, [key]: value }))
  }

  function validate(): string[] {
    const errs: string[] = []
    if (!form.name.trim()) errs.push("批次名称不能为空")
    if (form.payload_json.trim()) {
      try {
        JSON.parse(form.payload_json)
      } catch (e) {
        errs.push(`JSON 内容格式错误：${(e as Error).message}`)
      }
    }
    if (form.webhook_url && !/^https:\/\//.test(form.webhook_url)) {
      errs.push("Webhook 地址必须以 https:// 开头")
    }
    if (form.assign_credential) {
      const planType = form.credential_plan_type.trim()
      if (plansLoading) errs.push("套餐列表仍在加载，请稍后再试")
      else if (plansError) errs.push("套餐列表加载失败，请重新加载")
      else if (!planType) errs.push("启用凭证分配时必须选择套餐")
      else if (!plans.some((plan) => plan.plan_type === planType && plan.available > 0)) {
        errs.push("所选套餐没有可用凭证，请刷新库存后重试")
      }
    }
    return errs
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const errs = validate()
    if (errs.length) {
      setErrors(errs)
      return
    }
    setLoading(true)
    setErrors([])
    try {
      const res = await api.createBatch({
        ...form,
        payload_json: form.payload_json.trim() || "{}",
        name: form.name.trim(),
        credential_plan_type: form.assign_credential ? form.credential_plan_type.trim() : "",
      })
      toast.success(res.message)
      navigate(`/admin/batches/${res.batch_id}`)
    } catch (err) {
      if (err instanceof ApiError) {
        setErrors(err.errors ?? [err.message])
      } else {
        setErrors(["网络异常，请稍后再试"])
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="新建批次"
        description="配置规则并批量生成兑换码"
        actions={
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
        }
      />

      <form onSubmit={handleSubmit} noValidate>
        <div className="grid gap-6 lg:grid-cols-2">
          {/* 基本信息 */}
          <Card>
            <CardHeader>
              <CardTitle>基本信息</CardTitle>
              <CardDescription>批次名称与要分发的 JSON 内容</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="name">批次名称 *</Label>
                <Input
                  id="name"
                  placeholder="例如：新手礼包 / 会员月卡 / 活动补偿"
                  value={form.name}
                  maxLength={100}
                  onChange={(e) => set("name", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="description">备注说明</Label>
                <Input
                  id="description"
                  placeholder="内部备注，仅管理员可见"
                  value={form.description}
                  maxLength={200}
                  onChange={(e) => set("description", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="payload">分发内容（JSON）*</Label>
                <Textarea
                  id="payload"
                  rows={8}
                  className="font-mono text-sm"
                  placeholder='{"coins": 100, "items": ["sword_01"]}'
                  value={form.payload_json}
                  onChange={(e) => set("payload_json", e.target.value)}
                  spellCheck={false}
                />
                <p className="text-xs text-muted-foreground">
                  用户兑换成功后看到这段 JSON，并随 Webhook 推送给您的服务器
                </p>
              </div>
            </CardContent>
          </Card>

          {/* 生成规则 + Webhook */}
          <div className="space-y-6">
            <Card>
              <CardHeader>
                <CardTitle>生成与使用规则</CardTitle>
                <CardDescription>数量、格式、有效期与限兑规则</CardDescription>
              </CardHeader>
              <CardContent className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="count">生成数量</Label>
                  <Input
                    id="count"
                    type="number"
                    min={1}
                    max={10000}
                    value={form.count}
                    onChange={(e) => set("count", Number(e.target.value))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="prefix">码前缀</Label>
                  <Input
                    id="prefix"
                    placeholder="如 VIP（留空无前缀）"
                    maxLength={8}
                    value={form.prefix}
                    onChange={(e) => set("prefix", e.target.value.toUpperCase())}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="code-length">随机部分长度</Label>
                  <Input
                    id="code-length"
                    type="number"
                    min={8}
                    max={20}
                    value={form.code_length}
                    onChange={(e) => set("code_length", Number(e.target.value))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="expires">过期时间</Label>
                  <Input
                    id="expires"
                    type="datetime-local"
                    value={form.expires_at}
                    onChange={(e) => set("expires_at", e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">留空表示永久有效</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="max-uses">单码可兑换次数</Label>
                  <Input
                    id="max-uses"
                    type="number"
                    min={1}
                    value={form.max_uses_per_code}
                    onChange={(e) => set("max_uses_per_code", Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">通常为 1（一码一兑）</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="max-per-user">每邮箱限兑次数</Label>
                  <Input
                    id="max-per-user"
                    type="number"
                    min={1}
                    value={form.max_redeems_per_user}
                    onChange={(e) => set("max_redeems_per_user", Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">同一邮箱在本批次的限次</p>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>凭证分配</CardTitle>
                <CardDescription>兑换时从对应套餐的可用库存中分配一条凭证</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center justify-between gap-4">
                  <Label htmlFor="assign-credential">自动分配凭证</Label>
                  <Switch
                    id="assign-credential"
                    checked={form.assign_credential}
                    onCheckedChange={(checked) =>
                      setForm((current) => ({
                        ...current,
                        assign_credential: checked,
                        credential_plan_type: checked ? current.credential_plan_type : "",
                      }))
                    }
                  />
                </div>
                {form.assign_credential ? (
                  <div className="space-y-2">
                    <Label htmlFor="credential-plan">套餐类型 *</Label>
                    {plansLoading ? (
                      <Skeleton className="h-8 w-full rounded-lg" />
                    ) : plansError ? (
                      <Alert variant="destructive">
                        <CircleX className="size-4" />
                        <AlertTitle>套餐列表加载失败</AlertTitle>
                        <AlertDescription className="space-y-2">
                          <p>{plansError}</p>
                          <Button type="button" size="sm" variant="outline" onClick={() => void loadPlans()}>
                            <RefreshCw className="size-3.5" />
                            重新加载
                          </Button>
                        </AlertDescription>
                      </Alert>
                    ) : plans.length === 0 ? (
                      <Alert>
                        <AlertTitle>还没有凭证套餐</AlertTitle>
                        <AlertDescription className="space-y-2">
                          <p>请先上传凭证并创建套餐，再配置自动分配。</p>
                          <Button size="sm" variant="outline" nativeButton={false} render={<Link to="/admin/credentials">前往凭证管理</Link>} />
                        </AlertDescription>
                      </Alert>
                    ) : (
                      <>
                        <Select
                          value={form.credential_plan_type}
                          onValueChange={(value) => set("credential_plan_type", value ?? "")}
                        >
                          <SelectTrigger id="credential-plan" className="w-full">
                            <SelectValue placeholder="选择有可用库存的套餐" />
                          </SelectTrigger>
                          <SelectContent>
                            {plans.map((plan) => (
                              <SelectItem
                                key={plan.plan_type}
                                value={plan.plan_type}
                                disabled={plan.available === 0}
                              >
                                {plan.plan_type} · 可用 {plan.available}/{plan.total}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        {plans.every((plan) => plan.available === 0) ? (
                          <p className="text-xs text-destructive">
                            当前没有可用库存，请先前往凭证管理上传凭证。
                          </p>
                        ) : null}
                      </>
                    )}
                  </div>
                ) : null}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Webhook 通知</CardTitle>
                <CardDescription>兑换成功后推送给您服务器（可留空）</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="webhook-url">Webhook 地址</Label>
                  <Input
                    id="webhook-url"
                    type="url"
                    placeholder="https://your-server.com/api/cdk-callback"
                    value={form.webhook_url}
                    onChange={(e) => set("webhook_url", e.target.value)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="webhook-secret">签名密钥</Label>
                  <div className="flex gap-2">
                    <Input
                      id="webhook-secret"
                      className="font-mono"
                      maxLength={64}
                      value={form.webhook_secret}
                      onChange={(e) => set("webhook_secret", e.target.value)}
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      title="重新生成密钥"
                      onClick={() => set("webhook_secret", randomSecret())}
                    >
                      <RefreshCw className="size-4" />
                    </Button>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    用于计算 X-CDK-Signature 签名，请在接收端验签
                  </p>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>

        {errors.length ? (
          <Alert variant="destructive" className="mt-6">
            <CircleX className="size-4" />
            <AlertTitle>请修正以下问题</AlertTitle>
            <AlertDescription>
              <ul className="list-inside list-disc">
                {errors.map((e, i) => (
                  <li key={i}>{e}</li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        ) : null}

        <div className="mt-6 flex items-center gap-3">
          <Button type="submit" disabled={loading}>
            {loading ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
            创建批次并生成兑换码
          </Button>
          <Button variant="outline" nativeButton={false} render={<Link to="/admin/batches">取消</Link>} />
        </div>
      </form>
    </div>
  )
}
