import { toast } from "sonner"

/** 复制文本到剪贴板（含非安全上下文回退），成功时弹出提示 */
export async function copyText(text: string, tip = "已复制到剪贴板") {
  try {
    if (navigator.clipboard && window.isSecureContext !== false) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement("textarea")
      ta.value = text
      ta.style.position = "fixed"
      ta.style.opacity = "0"
      document.body.appendChild(ta)
      ta.select()
      document.execCommand("copy")
      document.body.removeChild(ta)
    }
    toast.success(tip)
  } catch {
    toast.error("复制失败，请手动选择复制")
  }
}
