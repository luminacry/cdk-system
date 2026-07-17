import { Moon, Sun } from "lucide-react"
import { useTheme } from "next-themes"

import { Button } from "@/components/ui/button"

/** 亮/暗主题切换按钮 */
export function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const isDark = theme !== "light"

  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={() => setTheme(isDark ? "light" : "dark")}
      title={isDark ? "切换到亮色主题" : "切换到暗色主题"}
    >
      {isDark ? <Sun className="size-4" /> : <Moon className="size-4" />}
      <span className="sr-only">切换主题</span>
    </Button>
  )
}
