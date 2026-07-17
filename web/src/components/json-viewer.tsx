import { useMemo } from "react"

import { cn } from "@/lib/utils"

const TOKEN_RE =
  /("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*"(?:\s*:)?|\b(?:true|false)\b|\bnull\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g

function tokenClass(token: string): string | null {
  if (!token) return null
  if (token.startsWith('"')) {
    return /:\s*$/.test(token)
      ? "text-sky-600 dark:text-sky-400"
      : "text-emerald-600 dark:text-emerald-400"
  }
  if (token === "true" || token === "false") return "text-amber-600 dark:text-amber-400"
  if (token === "null") return "text-purple-600 dark:text-purple-400"
  if (/^-?\d/.test(token)) return "text-rose-600 dark:text-rose-400"
  return null
}

/** JSON 语法高亮查看器 */
export function JsonViewer({ data, className }: { data: unknown; className?: string }) {
  const text = useMemo(() => {
    try {
      return JSON.stringify(data, null, 2) ?? "null"
    } catch {
      return String(data)
    }
  }, [data])

  const parts = useMemo(() => text.split(TOKEN_RE), [text])

  return (
    <pre
      className={cn(
        "overflow-x-auto rounded-lg border bg-muted/50 p-4 font-mono text-[13px] leading-relaxed",
        className,
      )}
    >
      <code>
        {parts.map((part, i) => {
          const cls = tokenClass(part)
          return cls ? (
            <span key={i} className={cls}>
              {part}
            </span>
          ) : (
            <span key={i}>{part}</span>
          )
        })}
      </code>
    </pre>
  )
}
