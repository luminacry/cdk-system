/** 与后端 JSON API 对应的类型定义 */

export interface Batch {
  id: number
  name: string
  description: string
  payload_json: unknown
  prefix: string
  code_length: number
  expires_at: string | null
  max_uses_per_code: number
  max_redeems_per_user: number
  assign_credential: boolean
  credential_plan_type: string
  webhook_url: string
  webhook_secret: string
  status: "active" | "disabled"
  created_at: string
  code_count?: number
  used_count?: number
}

export interface BatchInput {
  name: string
  description: string
  payload_json: string
  count: number
  prefix: string
  code_length: number
  expires_at: string
  max_uses_per_code: number
  max_redeems_per_user: number
  assign_credential: boolean
  credential_plan_type: string
  webhook_url: string
  webhook_secret: string
}

export interface BatchCounts {
  total: number
  disabled: number | null
  usedup: number | null
  total_uses: number
}

export interface CodeItem {
  id: number
  code: string
  display: string
  status: "unused" | "disabled"
  use_count: number
  created_at: string
}

export interface LogItem {
  id: number
  created_at: string
  batch_name: string | null
  code: string
  user_id: string
  result: string
  message: string
  webhook_status: "none" | "pending" | "success" | "failed"
  webhook_response: string
  ip: string
}

export interface Stats {
  codes_total: number
  codes_used: number
  batches: number
  redeem_total: number
  today: number
  webhook_failed: number
}

export interface DailyPoint {
  date: string
  count: number
}

export interface RecentItem {
  id: number
  created_at: string
  batch_name: string | null
  code: string
  user_id: string
  result: string
  message: string
  webhook_status: string
}

export interface Credential {
  id: number
  email: string
  plan_type: string
  expired: string | null
  used: boolean
  redemption_id?: number | null
  created_at: string
  updated_at: string
}

export interface CredentialPlan {
  plan_type: string
  total: number
  available: number
  used: number
  expired: number
}

export interface RedeemSuccess {
  ok: true
  result: "success"
  message: string
  batch: string
  code: string
  user_id: string
  payload: unknown
  credential?: Record<string, unknown>
  redeemed_at: string
}

export interface FaqItem {
  id: number
  sort_order: number
  title: string
  content: string
  created_at: string
}

export interface Paged {
  total: number
  page: number
  pages: number
}
