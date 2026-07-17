import { createContext, useContext } from "react"

import type { SiteSettings } from "@/lib/api"

export const DEFAULT_SITE_SETTINGS: SiteSettings = {
  site_title: "CDK 兑换中心",
  has_logo: false,
  logo_url: "",
  updated_at: "",
}

export type SiteSettingsStatus = "loading" | "ready" | "error"

export interface SiteSettingsContextValue {
  settings: SiteSettings
  status: SiteSettingsStatus
  error: string | null
  refresh: () => Promise<void>
  replaceSettings: (settings: SiteSettings) => void
}

export const SiteSettingsContext = createContext<SiteSettingsContextValue | null>(null)

export function useSiteSettings(): SiteSettingsContextValue {
  const context = useContext(SiteSettingsContext)
  if (!context) {
    throw new Error("useSiteSettings 必须在 SiteSettingsProvider 内使用")
  }
  return context
}
