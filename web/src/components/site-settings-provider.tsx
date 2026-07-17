import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react"
import { Zap } from "lucide-react"
import { useLocation } from "react-router-dom"

import { api, type SiteSettings } from "@/lib/api"
import {
  DEFAULT_SITE_SETTINGS,
  SiteSettingsContext,
  useSiteSettings,
  type SiteSettingsStatus,
} from "@/lib/site-settings-context"

export function SiteSettingsProvider({ children }: { children: ReactNode }) {
  const location = useLocation()
  const [settings, setSettings] = useState<SiteSettings>(DEFAULT_SITE_SETTINGS)
  const [status, setStatus] = useState<SiteSettingsStatus>("loading")
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setStatus("loading")
    setError(null)
    try {
      const response = await api.siteSettings()
      setSettings(response.settings)
      setStatus("ready")
    } catch (err) {
      setError(err instanceof Error ? err.message : "站点设置加载失败")
      setStatus("error")
    }
  }, [])

  const replaceSettings = useCallback((nextSettings: SiteSettings) => {
    setSettings(nextSettings)
    setStatus("ready")
    setError(null)
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => {
    const title = settings.site_title.trim() || DEFAULT_SITE_SETTINGS.site_title
    document.title = location.pathname.startsWith("/admin")
      ? `${title} - 管理后台`
      : title

    let favicon = document.querySelector<HTMLLinkElement>('link[rel~="icon"]')
    if (!favicon) {
      favicon = document.createElement("link")
      favicon.rel = "icon"
      document.head.appendChild(favicon)
    }
    if (!favicon.dataset.defaultHref) {
      favicon.dataset.defaultHref = favicon.getAttribute("href") || "data:,"
    }
    favicon.href = settings.has_logo && settings.logo_url
      ? settings.logo_url
      : favicon.dataset.defaultHref
  }, [location.pathname, settings.has_logo, settings.logo_url, settings.site_title])

  const value = useMemo(
    () => ({
      settings,
      status,
      error,
      refresh,
      replaceSettings,
    }),
    [error, refresh, replaceSettings, settings, status],
  )

  return <SiteSettingsContext.Provider value={value}>{children}</SiteSettingsContext.Provider>
}

interface SiteLogoProps {
  className?: string
  alt?: string
}

export function SiteLogo({ className = "size-6", alt = "" }: SiteLogoProps) {
  const { settings } = useSiteSettings()
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  const showImage = settings.has_logo && settings.logo_url && failedUrl !== settings.logo_url

  if (showImage) {
    return (
      <img
        src={settings.logo_url}
        alt={alt}
        className={`${className} object-contain`}
        onError={() => setFailedUrl(settings.logo_url)}
      />
    )
  }

  return <Zap className={className} aria-hidden="true" />
}
