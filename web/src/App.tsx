import { lazy, Suspense } from "react"
import { Navigate, Route, Routes } from "react-router-dom"

import { SiteSettingsProvider } from "@/components/site-settings-provider"
import { RedeemPage } from "@/pages/redeem"

const AdminLayout = lazy(() =>
  import("@/layouts/admin-layout").then((module) => ({ default: module.AdminLayout })),
)
const LoginPage = lazy(() =>
  import("@/pages/admin/login").then((module) => ({ default: module.LoginPage })),
)
const DashboardPage = lazy(() =>
  import("@/pages/admin/dashboard").then((module) => ({ default: module.DashboardPage })),
)
const BatchesPage = lazy(() =>
  import("@/pages/admin/batches").then((module) => ({ default: module.BatchesPage })),
)
const BatchNewPage = lazy(() =>
  import("@/pages/admin/batch-new").then((module) => ({ default: module.BatchNewPage })),
)
const BatchDetailPage = lazy(() =>
  import("@/pages/admin/batch-detail").then((module) => ({ default: module.BatchDetailPage })),
)
const LogsPage = lazy(() =>
  import("@/pages/admin/logs").then((module) => ({ default: module.LogsPage })),
)
const FaqPage = lazy(() =>
  import("@/pages/admin/faq").then((module) => ({ default: module.FaqPage })),
)
const CredentialsPage = lazy(() =>
  import("@/pages/admin/credentials").then((module) => ({ default: module.CredentialsPage })),
)
const SiteSettingsPage = lazy(() =>
  import("@/pages/admin/site-settings").then((module) => ({ default: module.SiteSettingsPage })),
)

function RouteFallback() {
  return (
    <div className="min-h-svh bg-background p-6" role="status" aria-label="页面加载中">
      <div className="mx-auto max-w-6xl space-y-5">
        <div className="h-8 w-48 animate-pulse rounded-md bg-muted" />
        <div className="h-24 animate-pulse rounded-lg bg-muted" />
        <div className="h-72 animate-pulse rounded-lg bg-muted" />
      </div>
    </div>
  )
}

export default function App() {
  return (
    <SiteSettingsProvider>
      <Suspense fallback={<RouteFallback />}>
        <Routes>
          <Route path="/" element={<RedeemPage />} />
          <Route path="/admin/login" element={<LoginPage />} />
          <Route path="/admin" element={<AdminLayout />}>
            <Route index element={<DashboardPage />} />
            <Route path="batches" element={<BatchesPage />} />
            <Route path="batches/new" element={<BatchNewPage />} />
            <Route path="batches/:id" element={<BatchDetailPage />} />
            <Route path="logs" element={<LogsPage />} />
            <Route path="faq" element={<FaqPage />} />
            <Route path="credentials" element={<CredentialsPage />} />
            <Route path="settings" element={<SiteSettingsPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </SiteSettingsProvider>
  )
}
