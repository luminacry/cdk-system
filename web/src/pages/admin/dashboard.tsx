import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import {
  CircleCheck,
  Package,
  Plus,
  Ticket,
  TrendingUp,
  TriangleAlert,
} from "lucide-react"
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts"

import { api } from "@/lib/api"
import type { DailyPoint, RecentItem, Stats } from "@/lib/types"
import { toast } from "sonner"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { StatCard } from "@/components/stat-card"
import { ResultBadge, WebhookBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

const chartConfig = {
  count: { label: "兑换数", color: "var(--chart-1)" },
} satisfies ChartConfig

export function DashboardPage() {
  const [data, setData] = useState<{
    stats: Stats
    daily: DailyPoint[]
    recent: RecentItem[]
  } | null>(null)

  useEffect(() => {
    api.stats().then(setData).catch((err) => toast.error(err.message))
  }, [])

  if (!data) {
    return (
      <div className="space-y-6">
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-24 rounded-xl" />
          ))}
        </div>
        <Skeleton className="h-72 rounded-xl" />
      </div>
    )
  }

  const { stats, daily, recent } = data
  const recentItems = recent ?? []
  const dailyPoints = daily ?? []

  return (
    <div className="space-y-6">
      <PageHeader
        title="仪表盘"
        description="系统整体兑换情况一览"
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

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
        <StatCard title="兑换码总数" value={stats.codes_total} icon={Ticket} />
        <StatCard title="成功兑换" value={stats.redeem_total} icon={CircleCheck} />
        <StatCard title="今日兑换" value={stats.today} icon={TrendingUp} tone="primary" />
        <StatCard title="批次数" value={stats.batches} icon={Package} />
        <StatCard
          title="Webhook 失败"
          value={stats.webhook_failed}
          icon={TriangleAlert}
          tone={stats.webhook_failed > 0 ? "destructive" : "default"}
          hint={stats.webhook_failed > 0 ? "可在兑换记录中重推" : undefined}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>近 14 天兑换趋势</CardTitle>
          <CardDescription>每日成功兑换次数</CardDescription>
        </CardHeader>
        <CardContent>
          <ChartContainer config={chartConfig} className="h-56 w-full">
            <AreaChart data={dailyPoints} margin={{ left: -20, right: 8, top: 4 }}>
              <defs>
                <linearGradient id="fillCount" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="var(--color-count)" stopOpacity={0.5} />
                  <stop offset="95%" stopColor="var(--color-count)" stopOpacity={0.05} />
                </linearGradient>
              </defs>
              <CartesianGrid vertical={false} strokeDasharray="3 3" />
              <XAxis dataKey="date" tickLine={false} axisLine={false} tickMargin={8} fontSize={12} />
              <YAxis tickLine={false} axisLine={false} allowDecimals={false} fontSize={12} />
              <ChartTooltip content={<ChartTooltipContent />} />
              <Area
                type="monotone"
                dataKey="count"
                stroke="var(--color-count)"
                strokeWidth={2}
                fill="url(#fillCount)"
              />
            </AreaChart>
          </ChartContainer>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>最近兑换记录</CardTitle>
          <CardDescription>最新 8 条兑换尝试</CardDescription>
        </CardHeader>
        <CardContent>
          {recentItems.length === 0 ? (
            <EmptyState
              title="还没有兑换记录"
              description="创建批次并把兑换码分发给用户后，记录会出现在这里"
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>时间</TableHead>
                  <TableHead>批次</TableHead>
                  <TableHead>兑换码</TableHead>
                  <TableHead>邮箱</TableHead>
                  <TableHead>结果</TableHead>
                  <TableHead>Webhook</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {recentItems.map((r) => (
                  <TableRow key={r.id}>
                    <TableCell className="font-mono text-xs whitespace-nowrap">
                      {r.created_at}
                    </TableCell>
                    <TableCell>{r.batch_name ?? "—"}</TableCell>
                    <TableCell className="font-mono text-xs">{r.code}</TableCell>
                    <TableCell className="max-w-32 truncate">{r.user_id}</TableCell>
                    <TableCell>
                      <ResultBadge result={r.result} message={r.message} />
                    </TableCell>
                    <TableCell>
                      <WebhookBadge status={r.webhook_status} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
