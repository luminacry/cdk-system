import type {
  Batch,
  BatchCounts,
  BatchInput,
  CodeItem,
  Credential,
  CredentialPlan,
  DailyPoint,
  FaqItem,
  LogItem,
  Paged,
  RecentItem,
  RedeemSuccess,
  Stats,
} from "./types"

export class ApiError extends Error {
  status: number
  errors?: string[]

  constructor(message: string, status: number, errors?: string[]) {
    super(message)
    this.status = status
    this.errors = errors
  }
}

function getCSRFToken(): string | null {
  const match = document.cookie.match(/(?:^|; )csrf_token=([^;]+)/)
  return match ? decodeURIComponent(match[1]) : null
}

function isMutatingMethod(method?: string): boolean {
  return method === "POST" || method === "PUT" || method === "PATCH" || method === "DELETE"
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {}
  if (init?.body && !(init.body instanceof FormData)) {
    headers["Content-Type"] = "application/json"
  }
  if (isMutatingMethod(init?.method)) {
    const token = getCSRFToken()
    if (token) {
      headers["X-CSRF-Token"] = token
    }
  }
  if (init?.headers) {
    Object.assign(headers, init.headers)
  }

  const resp = await fetch(url, {
    credentials: "same-origin",
    ...init,
    headers,
  })
  const data = await resp
    .json()
    .catch(() => ({ ok: false, message: "服务器响应异常，请稍后再试" }))
  if (!resp.ok) {
    throw new ApiError(data.message || `请求失败（${resp.status}）`, resp.status, data.errors)
  }
  return data as T
}

async function download(url: string): Promise<Blob> {
  const resp = await fetch(url, { credentials: "same-origin" })
  if (!resp.ok) {
    const data = await resp
      .json()
      .catch(() => ({ message: "服务器响应异常，请稍后再试" }))
    throw new ApiError(data.message || `请求失败（${resp.status}）`, resp.status, data.errors)
  }
  return resp.blob()
}

function post<T>(url: string, body?: unknown): Promise<T> {
  return request<T>(url, {
    method: "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

export interface LogsParams {
  batch_id?: number
  result?: string
  webhook?: string
  user_id?: string
  page?: number
}

export interface SiteSettings {
  site_title: string
  has_logo: boolean
  logo_url: string
  updated_at: string
}

export interface UpdateSiteSettingsInput {
  site_title: string
  logo?: File
  remove_logo?: boolean
}

export const api = {
  siteSettings: () =>
    request<{ ok: boolean; settings: SiteSettings }>("/api/site-settings"),
  updateSiteSettings: ({ site_title, logo, remove_logo }: UpdateSiteSettingsInput) => {
    const formData = new FormData()
    formData.append("site_title", site_title)
    if (logo) formData.append("logo", logo)
    if (remove_logo) formData.append("remove_logo", "true")
    return request<{ ok: boolean; message: string; settings: SiteSettings }>(
      "/api/admin/site-settings",
      { method: "POST", body: formData },
    )
  },

  // 用户兑换（失败时 HTTP 400/429，抛 ApiError，message 即为用户可读提示）
  redeem: (user_id: string, code: string) =>
    request<RedeemSuccess>("/api/redeem", {
      method: "POST",
      body: JSON.stringify({ user_id, code }),
    }),

  // 兑换须知（公开只读 + 管理）
  publicFaq: () => request<{ ok: boolean; items: { title: string; content: string }[] }>("/api/faq"),
  faq: () => request<{ ok: boolean; items: FaqItem[] }>("/api/admin/faq"),
  createFaq: (input: { title: string; content: string }) =>
    post<{ ok: boolean; id: number; message: string }>("/api/admin/faq", input),
  updateFaq: (id: number, input: { title: string; content: string }) =>
    request<{ ok: boolean; message: string }>(`/api/admin/faq/${id}`, {
      method: "PUT",
      body: JSON.stringify(input),
    }),
  deleteFaq: (id: number) =>
    request<{ ok: boolean; message: string }>(`/api/admin/faq/${id}`, { method: "DELETE" }),
  reorderFaq: (ids: number[]) =>
    post<{ ok: boolean; message: string }>("/api/admin/faq/reorder", { ids }),

  // 认证
  login: (username: string, password: string) =>
    post<{ ok: boolean; username: string }>("/api/admin/login", { username, password }),
  logout: () => post<{ ok: boolean }>("/api/admin/logout"),
  me: () => request<{ ok: boolean; username: string }>("/api/admin/me"),

  // 仪表盘
  stats: () =>
    request<{ ok: boolean; stats: Stats; daily: DailyPoint[]; recent: RecentItem[] }>(
      "/api/admin/stats",
    ),

  // 批次
  batches: () => request<{ ok: boolean; batches: Batch[] }>("/api/admin/batches"),
  createBatch: (input: BatchInput) =>
    post<{ ok: boolean; batch_id: number; message: string }>("/api/admin/batches", input),
  batch: (id: number | string) =>
    request<{ ok: boolean; batch: Batch; counts: BatchCounts }>(`/api/admin/batches/${id}`),
  batchCodes: (id: number | string, state: string, page: number) =>
    request<{ ok: boolean; codes: CodeItem[]; max_uses: number } & Paged>(
      `/api/admin/batches/${id}/codes?state=${encodeURIComponent(state)}&page=${page}`,
    ),
  toggleBatch: (id: number) =>
    post<{ ok: boolean; status: string; message: string }>(`/api/admin/batches/${id}/toggle`),
  toggleCode: (id: number) =>
    post<{ ok: boolean; status: string; message: string }>(`/api/admin/codes/${id}/toggle`),
  exportUrl: (id: number, fmt: "txt" | "csv") =>
    `/api/admin/batches/${id}/export?fmt=${fmt}`,

  // 兑换记录
  logs: (params: LogsParams) => {
    const qs = new URLSearchParams()
    if (params.batch_id) qs.set("batch_id", String(params.batch_id))
    if (params.result) qs.set("result", params.result)
    if (params.webhook) qs.set("webhook", params.webhook)
    if (params.user_id) qs.set("user_id", params.user_id)
    if (params.page) qs.set("page", String(params.page))
    return request<{ ok: boolean; logs: LogItem[]; batches: { id: number; name: string }[] } & Paged>(
      `/api/admin/logs?${qs.toString()}`,
    )
  },
  resendWebhook: (id: number) =>
    post<{ ok: boolean; message: string }>(`/api/admin/logs/${id}/resend`),

  // 凭证管理
  credentials: (state: "all" | "used" | "unused" = "all", page = 1, pageSize = 50) => {
    const qs = new URLSearchParams({
      state,
      page: String(page),
      page_size: String(pageSize),
    })
    return request<{ ok: boolean; items: Credential[]; page_size: number } & Paged>(
      `/api/admin/credentials?${qs.toString()}`,
    )
  },
  credential: (id: number) =>
    request<{ ok: boolean; credential: Credential }>(`/api/admin/credentials/${id}`),
  downloadCredential: (id: number) => download(`/api/admin/credentials/${id}/download`),
  credentialStats: () =>
    request<{ ok: boolean; total: number; used: number; unused: number }>(
      "/api/admin/credentials/stats",
    ),
  credentialPlans: () =>
    request<{ ok: boolean; plans: CredentialPlan[] }>("/api/admin/credentials/plans"),
  uploadCredentials: (file: File, planType: string) => {
    const formData = new FormData()
    formData.append("file", file)
    formData.append("plan_type", planType)
    return request<{ ok: boolean; imported: number; skipped: number; message: string }>(
      "/api/admin/credentials/upload",
      { method: "POST", body: formData },
    )
  },
  deleteCredential: (id: number) =>
    request<{ ok: boolean; message: string }>(`/api/admin/credentials/${id}`, { method: "DELETE" }),
  batchDeleteCredentials: (ids: number[]) =>
    post<{ ok: boolean; deleted: number; message: string }>("/api/admin/credentials/batch-delete", {
      ids,
    }),
}
