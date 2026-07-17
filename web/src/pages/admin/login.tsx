import { useState } from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { CircleX, Loader2 } from "lucide-react"

import { api, ApiError } from "@/lib/api"
import { useSiteSettings } from "@/lib/site-settings-context"
import { SiteLogo } from "@/components/site-settings-provider"
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

export function LoginPage() {
  const { settings } = useSiteSettings()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string } | null)?.from || "/admin"

  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!username || !password) {
      setError("请输入用户名和密码")
      return
    }
    setLoading(true)
    setError(null)
    try {
      await api.login(username, password)
      navigate(from, { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "网络异常，请稍后再试")
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden px-4">
      <div
        aria-hidden
        className="pointer-events-none absolute -top-32 -left-32 size-[28rem] rounded-full bg-primary/25 blur-[120px]"
      />
      <div
        aria-hidden
        className="pointer-events-none absolute -right-32 -bottom-32 size-[26rem] rounded-full bg-cyan-500/20 blur-[120px]"
      />

      <Card className="relative z-10 w-full max-w-sm shadow-2xl">
        <CardHeader className="items-center text-center">
          <div className="mb-2 flex size-12 items-center justify-center overflow-hidden rounded-xl bg-muted text-foreground">
            <SiteLogo className="size-7" />
          </div>
          <CardTitle className="max-w-full text-xl break-words">{settings.site_title}</CardTitle>
          <CardDescription>管理后台 · 请使用管理员账号登录</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">用户名</Label>
              <Input
                id="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
              />
            </div>
            {error ? (
              <Alert variant="destructive">
                <CircleX className="size-4" />
                <AlertTitle>登录失败</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? <Loader2 className="size-4 animate-spin" /> : null}
              登 录
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
