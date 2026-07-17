import { ChevronLeft, ChevronRight } from "lucide-react"

import { Button } from "@/components/ui/button"

interface DataPaginationProps {
  page: number
  pages: number
  total: number
  onPage: (page: number) => void
}

/** 表格分页条 */
export function DataPagination({ page, pages, total, onPage }: DataPaginationProps) {
  if (total === 0) return null
  return (
    <div className="flex items-center justify-between gap-4 pt-4">
      <span className="text-sm text-muted-foreground">共 {total} 条</span>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="icon-sm"
          disabled={page <= 1}
          onClick={() => onPage(page - 1)}
          aria-label="上一页"
        >
          <ChevronLeft className="size-4" />
        </Button>
        <span className="min-w-16 text-center font-mono text-sm tabular-nums">
          {page} / {pages}
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          disabled={page >= pages}
          onClick={() => onPage(page + 1)}
          aria-label="下一页"
        >
          <ChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  )
}
