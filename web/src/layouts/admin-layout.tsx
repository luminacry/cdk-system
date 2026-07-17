import { useEffect, useState } from "react"
import { Link, Navigate, Outlet, useLocation, useNavigate } from "react-router-dom"
import {
  CircleHelp,
  ExternalLink,
  FileJson,
  LayoutDashboard,
  LogOut,
  Package,
  ScrollText,
  Settings as SettingsIcon,
  UserRound,
} from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import { useSiteSettings } from "@/lib/site-settings-context"
import { SiteLogo } from "@/components/site-settings-provider"
import { ThemeToggle } from "@/components/theme-toggle"
import { Separator } from "@/components/ui/separator"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { Skeleton } from "@/components/ui/skeleton"

const NAV_ITEMS = [
  { title: "仪表盘", url: "/admin", icon: LayoutDashboard, end: true },
  { title: "批次管理", url: "/admin/batches", icon: Package, end: false },
  { title: "凭证管理", url: "/admin/credentials", icon: FileJson, end: false },
  { title: "兑换记录", url: "/admin/logs", icon: ScrollText, end: false },
  { title: "兑换须知", url: "/admin/faq", icon: CircleHelp, end: false },
  { title: "站点设置", url: "/admin/settings", icon: SettingsIcon, end: false },
]

const PAGE_TITLES: [RegExp, string][] = [
  [/^\/admin\/?$/, "仪表盘"],
  [/^\/admin\/batches\/new/, "新建批次"],
  [/^\/admin\/batches\/\d+/, "批次详情"],
  [/^\/admin\/batches/, "批次管理"],
  [/^\/admin\/credentials/, "凭证管理"],
  [/^\/admin\/logs/, "兑换记录"],
  [/^\/admin\/faq/, "兑换须知"],
  [/^\/admin\/settings/, "站点设置"],
]

function currentTitle(pathname: string): string {
  for (const [re, title] of PAGE_TITLES) {
    if (re.test(pathname)) return title
  }
  return "管理后台"
}

export function AdminLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const { settings } = useSiteSettings()
  const [auth, setAuth] = useState<"loading" | "ok" | "no">("loading")

  useEffect(() => {
    let alive = true
    api
      .me()
      .then(() => alive && setAuth("ok"))
      .catch((err) => {
        if (!alive) return
        if (err instanceof ApiError && err.status === 401) setAuth("no")
        else {
          toast.error(err.message)
          setAuth("no")
        }
      })
    return () => {
      alive = false
    }
  }, [])

  if (auth === "no") {
    return <Navigate to="/admin/login" state={{ from: location.pathname }} replace />
  }

  if (auth === "loading") {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="flex flex-col items-center gap-3">
          <Skeleton className="size-10 rounded-xl" />
          <Skeleton className="h-4 w-28" />
        </div>
      </div>
    )
  }

  async function handleLogout() {
    try {
      await api.logout()
    } finally {
      navigate("/admin/login", { replace: true })
    }
  }

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                size="lg"
                render={
                  <Link to="/admin">
                    <div className="flex size-8 items-center justify-center overflow-hidden rounded-lg bg-muted text-foreground">
                      <SiteLogo className="size-5" />
                    </div>
                    <div className="grid flex-1 text-left text-sm leading-tight">
                      <span className="truncate font-semibold">{settings.site_title}</span>
                      <span className="truncate text-xs text-muted-foreground">管理后台</span>
                    </div>
                  </Link>
                }
              />
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>

        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>管理菜单</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {NAV_ITEMS.map((item) => {
                  const active = item.end
                    ? location.pathname === item.url || location.pathname === item.url + "/"
                    : location.pathname.startsWith(item.url)
                  return (
                    <SidebarMenuItem key={item.url}>
                      <SidebarMenuButton
                        isActive={active}
                        tooltip={item.title}
                        render={
                          <Link to={item.url}>
                            <item.icon />
                            <span>{item.title}</span>
                          </Link>
                        }
                      />
                    </SidebarMenuItem>
                  )
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip="打开用户兑换页"
                render={
                  <Link to="/" target="_blank" rel="noopener">
                    <ExternalLink />
                    <span>用户兑换页</span>
                  </Link>
                }
              />
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

      <SidebarInset>
        <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b bg-background/95 px-4 backdrop-blur">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mx-1 h-4" />
          <span className="text-sm font-medium">{currentTitle(location.pathname)}</span>
          <div className="ml-auto flex items-center gap-1">
            <ThemeToggle />
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <button className="flex size-8 items-center justify-center rounded-full border bg-muted text-sm font-medium outline-none hover:bg-accent">
                    <UserRound className="size-4" />
                    <span className="sr-only">用户菜单</span>
                  </button>
                }
              />
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>管理员</DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={handleLogout}>
                  <LogOut className="mr-2 size-4" />
                  退出登录
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        <main className="flex-1 p-4 md:p-6">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
