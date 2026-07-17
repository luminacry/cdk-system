import { useEffect, useState } from "react"
import { ArrowDown, ArrowUp, CircleHelp, Pencil, Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { FaqItem } from "@/lib/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
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
import { Textarea } from "@/components/ui/textarea"

interface FormState {
  open: boolean
  item: FaqItem | null
  title: string
  content: string
}

export function FaqPage() {
  const [items, setItems] = useState<FaqItem[] | null>(null)
  const [form, setForm] = useState<FormState>({
    open: false,
    item: null,
    title: "",
    content: "",
  })

  function load() {
    api.faq().then((d) => setItems(d.items)).catch((err) => toast.error(err.message))
  }

  useEffect(load, [])

  function openCreate() {
    setForm({ open: true, item: null, title: "", content: "" })
  }

  function openEdit(item: FaqItem) {
    setForm({ open: true, item, title: item.title, content: item.content })
  }

  function closeForm() {
    setForm((f) => ({ ...f, open: false }))
  }

  async function handleSave() {
    const title = form.title.trim()
    const content = form.content.trim()
    if (!title || !content) {
      toast.error("标题和内容不能为空")
      return
    }
    try {
      if (form.item) {
        await api.updateFaq(form.item.id, { title, content })
        toast.success("条目已更新")
      } else {
        await api.createFaq({ title, content })
        toast.success("条目已添加")
      }
      closeForm()
      load()
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "操作失败")
    }
  }

  async function handleDelete(id: number) {
    try {
      await api.deleteFaq(id)
      toast.success("条目已删除")
      load()
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "操作失败")
    }
  }

  async function move(index: number, direction: "up" | "down") {
    if (!items) return
    const newIndex = direction === "up" ? index - 1 : index + 1
    if (newIndex < 0 || newIndex >= items.length) return
    const next = [...items]
    const [removed] = next.splice(index, 1)
    next.splice(newIndex, 0, removed)
    setItems(next) // 乐观更新
    try {
      await api.reorderFaq(next.map((i) => i.id))
      toast.success("排序已更新")
      load()
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "排序失败")
      load()
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="兑换须知"
        description="管理兑换页下方展示的常见问题"
        actions={
          <Button onClick={openCreate}>
            <Plus className="size-4" />
            新增条目
          </Button>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>条目列表</CardTitle>
          <CardDescription>共 {items?.length ?? "…"} 条，支持上下移动调整顺序</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {items === null ? (
            <div className="space-y-3">
              <Skeleton className="h-16 rounded-lg" />
              <Skeleton className="h-16 rounded-lg" />
              <Skeleton className="h-16 rounded-lg" />
            </div>
          ) : items.length === 0 ? (
            <EmptyState
              icon={CircleHelp}
              title="还没有兑换须知条目"
              description="点击右上角「新增条目」创建第一条"
            />
          ) : (
            items.map((item, idx) => (
              <div
                key={item.id}
                className="flex items-start justify-between gap-4 rounded-lg border p-4"
              >
                <div className="min-w-0 flex-1">
                  <div className="font-medium">
                    {idx + 1}. {item.title}
                  </div>
                  <div className="mt-1 line-clamp-2 text-sm text-muted-foreground">
                    {item.content}
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    title="上移"
                    disabled={idx === 0}
                    onClick={() => move(idx, "up")}
                  >
                    <ArrowUp className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    title="下移"
                    disabled={idx === items.length - 1}
                    onClick={() => move(idx, "down")}
                  >
                    <ArrowDown className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    title="编辑"
                    onClick={() => openEdit(item)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <ConfirmDialog
                    trigger={
                      <Button variant="ghost" size="icon-xs" title="删除">
                        <Trash2 className="size-4 text-destructive" />
                      </Button>
                    }
                    title={`确定删除「${item.title}」吗？`}
                    description="删除后将不再在兑换页显示。"
                    confirmText="删除"
                    destructive
                    onConfirm={() => handleDelete(item.id)}
                  />
                </div>
              </div>
            ))
          )}
        </CardContent>
      </Card>

      <Dialog open={form.open} onOpenChange={(open) => !open && closeForm()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{form.item ? "编辑条目" : "新增条目"}</DialogTitle>
            <DialogDescription>编辑兑换须知的标题和正文内容</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="faq-title">标题</Label>
              <Input
                id="faq-title"
                placeholder="例如：如何获取兑换码？"
                value={form.title}
                onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="faq-content">内容</Label>
              <Textarea
                id="faq-content"
                rows={6}
                placeholder="输入详细说明…"
                value={form.content}
                onChange={(e) => setForm((f) => ({ ...f, content: e.target.value }))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={closeForm}>取消</Button>
            <Button onClick={handleSave}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
