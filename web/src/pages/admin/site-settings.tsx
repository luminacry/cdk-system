import { useEffect, useRef, useState } from "react"
import { CircleX, ImageUp, Loader2, RotateCcw, Save, Trash2, Zap } from "lucide-react"
import { toast } from "sonner"

import { SiteLogo } from "@/components/site-settings-provider"
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
import { Skeleton } from "@/components/ui/skeleton"
import { api, ApiError } from "@/lib/api"
import { useSiteSettings } from "@/lib/site-settings-context"

const MAX_LOGO_BYTES = 2 * 1024 * 1024
const MAX_LOGO_DIMENSION = 2048
const ALLOWED_LOGO_TYPES = new Set(["image/png", "image/jpeg"])

async function validateLogo(file: File): Promise<string | null> {
  if (!ALLOWED_LOGO_TYPES.has(file.type)) {
    return "请选择 PNG 或 JPEG 图片"
  }
  if (file.size > MAX_LOGO_BYTES) {
    return "图片不能超过 2 MiB"
  }

  try {
    const bitmap = await createImageBitmap(file)
    const oversized = bitmap.width > MAX_LOGO_DIMENSION || bitmap.height > MAX_LOGO_DIMENSION
    bitmap.close()
    return oversized ? "图片宽高不能超过 2048 × 2048 像素" : null
  } catch {
    return "图片无法读取，请重新选择有效的 PNG 或 JPEG 文件"
  }
}

export function SiteSettingsPage() {
  const { settings, status, error, refresh } = useSiteSettings()

  if (status === "loading") {
    return (
      <div className="space-y-6">
        <Skeleton className="h-12 w-64 rounded-lg" />
        <Skeleton className="h-80 max-w-3xl rounded-xl" />
      </div>
    )
  }

  if (status === "error") {
    return (
      <div className="space-y-6">
        <PageHeader title="站点设置" description="管理网站标题与 Logo" />
        <Alert variant="destructive" className="max-w-3xl">
          <CircleX className="size-4" />
          <AlertTitle>站点设置加载失败</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{error || "请稍后重试"}</span>
            <Button type="button" variant="outline" size="sm" onClick={() => void refresh()}>
              <RotateCcw className="size-3.5" />
              重新加载
            </Button>
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return <SiteSettingsForm key={settings.updated_at || settings.site_title} />
}

function SiteSettingsForm() {
  const { settings, replaceSettings } = useSiteSettings()
  const [siteTitle, setSiteTitle] = useState(settings.site_title)
  const [logoFile, setLogoFile] = useState<File | null>(null)
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [removeLogo, setRemoveLogo] = useState(false)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const fileValidationTokenRef = useRef(0)

  useEffect(() => {
    return () => {
      if (previewUrl) URL.revokeObjectURL(previewUrl)
    }
  }, [previewUrl])

  async function handleFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const validationToken = ++fileValidationTokenRef.current
    const file = event.target.files?.[0]
    event.target.value = ""
    if (!file) return

    setFormError(null)
    const validationError = await validateLogo(file)
    if (validationToken !== fileValidationTokenRef.current) return
    if (validationError) {
      setFormError(validationError)
      return
    }

    setLogoFile(file)
    setPreviewUrl(URL.createObjectURL(file))
    setRemoveLogo(false)
  }

  function handleRemoveLogo() {
    fileValidationTokenRef.current += 1
    setFormError(null)
    if (logoFile) {
      setLogoFile(null)
      setPreviewUrl(null)
      return
    }
    setRemoveLogo(true)
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const title = siteTitle.trim()
    if (!title) {
      setFormError("网站标题不能为空")
      return
    }
    if (Array.from(title).length > 100) {
      setFormError("网站标题不能超过 100 个字符")
      return
    }

    fileValidationTokenRef.current += 1
    setSaving(true)
    setFormError(null)
    try {
      const response = await api.updateSiteSettings({
        site_title: title,
        logo: logoFile || undefined,
        remove_logo: removeLogo,
      })
      setSiteTitle(response.settings.site_title)
      setLogoFile(null)
      setPreviewUrl(null)
      setRemoveLogo(false)
      replaceSettings(response.settings)
      toast.success(response.message || "站点设置已保存")
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : "保存失败，请稍后重试")
    } finally {
      setSaving(false)
    }
  }

  const trimmedTitle = siteTitle.trim()
  const titleLength = Array.from(trimmedTitle).length
  const titleInvalid = !trimmedTitle || titleLength > 100
  const hasChanges =
    trimmedTitle !== settings.site_title || logoFile !== null || removeLogo
  const hasVisibleCustomLogo = settings.has_logo && !removeLogo

  return (
    <div className="space-y-6">
      <PageHeader title="站点设置" description="管理网站标题与 Logo" />

      <Card className="max-w-3xl">
        <CardHeader>
          <CardTitle>网站标识</CardTitle>
          <CardDescription>保存后将同步显示在兑换页、管理登录页和后台导航。</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} noValidate>
            <fieldset disabled={saving} className="min-w-0 space-y-6 border-0 p-0">
            <div className="space-y-3">
              <Label htmlFor="site-logo">网站 Logo</Label>
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
                <div className="flex size-24 shrink-0 items-center justify-center overflow-hidden rounded-xl border bg-muted/40 text-muted-foreground">
                  {previewUrl ? (
                    <img src={previewUrl} alt="待上传的网站 Logo" className="size-full object-contain" />
                  ) : removeLogo ? (
                    <Zap className="size-10" aria-hidden="true" />
                  ) : (
                    <SiteLogo className="size-10" alt="当前网站 Logo" />
                  )}
                </div>

                <div className="min-w-0 space-y-2">
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => fileInputRef.current?.click()}
                    >
                      <ImageUp className="size-4" />
                      {hasVisibleCustomLogo || logoFile ? "更换图片" : "选择图片"}
                    </Button>
                    {logoFile || hasVisibleCustomLogo ? (
                      <Button type="button" variant="destructive" onClick={handleRemoveLogo}>
                        <Trash2 className="size-4" />
                        {logoFile ? "取消选择" : "移除 Logo"}
                      </Button>
                    ) : null}
                    {removeLogo ? (
                      <Button type="button" variant="outline" onClick={() => setRemoveLogo(false)}>
                        <RotateCcw className="size-4" />
                        撤销移除
                      </Button>
                    ) : null}
                  </div>
                  <p id="site-logo-help" className="text-xs text-muted-foreground">
                    支持 PNG、JPEG，最大 2 MiB，宽高不超过 2048 像素。
                  </p>
                  {removeLogo ? (
                    <p className="text-xs text-destructive">保存后将恢复为默认图标。</p>
                  ) : null}
                </div>
              </div>
              <input
                ref={fileInputRef}
                id="site-logo"
                type="file"
                accept="image/png,image/jpeg"
                aria-describedby="site-logo-help"
                className="sr-only"
                onChange={handleFileChange}
              />
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between gap-3">
                <Label htmlFor="site-title">网站标题</Label>
                <span className="text-xs tabular-nums text-muted-foreground">
                  {titleLength}/100
                </span>
              </div>
              <Input
                id="site-title"
                value={siteTitle}
                aria-invalid={titleInvalid || undefined}
                onChange={(event) => setSiteTitle(event.target.value)}
                placeholder="例如：CDK 兑换中心"
                autoComplete="organization"
              />
            </div>

            {formError ? (
              <Alert variant="destructive">
                <CircleX className="size-4" />
                <AlertTitle>无法保存设置</AlertTitle>
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            ) : null}

              <div className="flex justify-end">
                <Button type="submit" disabled={saving || titleInvalid || !hasChanges}>
                  {saving ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
                  {saving ? "保存中…" : "保存设置"}
                </Button>
              </div>
            </fieldset>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
