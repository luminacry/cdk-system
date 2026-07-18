# API 契约

本文档冻结 CDK 系统前后端接口，用于 Go 后端重写时保持兼容。

## 基础约定

- 基础路径：`/api`
- 请求体默认 `Content-Type: application/json`
- 成功响应：HTTP 2xx + JSON，通常包含 `ok: true`
- 错误响应：非 2xx + JSON `{ message: string, errors?: string[] }`
- 管理端使用 Cookie 会话；写操作需额外携带 `X-CSRF-Token` 请求头

## 公开接口

### GET /api/site-settings
返回当前站点品牌设置，无需登录。

响应：
```json
{
  "ok": true,
  "settings": {
    "site_title": "CDK 兑换中心",
    "has_logo": true,
    "logo_url": "/api/site-logo?v=...",
    "updated_at": "2026-07-18T02:00:00Z"
  }
}
```

### GET /api/site-logo
返回当前 PNG 或 JPEG Logo。未配置 Logo 时返回 404。成功响应包含 `ETag`、缓存策略和 `X-Content-Type-Options: nosniff`；前端应优先使用 `site-settings` 返回的带版本 `logo_url`。

### GET /api/faq
返回兑换须知列表。

响应：
```json
{
  "ok": true,
  "items": [
    { "title": "...", "content": "..." }
  ]
}
```

### POST /api/redeem
兑换 CDK。

请求：
```json
{ "user_id": "user@example.com", "code": "string" }
```

为兼容现有数据结构，请求字段仍名为 `user_id`。前端仅收集邮箱并用于兑换记录；系统不验证邮箱归属，也不会发送验证邮件。

成功响应：
```json
{
  "ok": true,
  "result": "success",
  "message": "兑换成功 🎉",
  "batch": "批次名称",
  "code": "PREFIX-XXXX-XXXX",
  "user_id": "...",
  "payload": {},
  "redeemed_at": "2026-07-17 12:00:00"
}
```

失败响应（HTTP 400/429）：
```json
{ "ok": false, "message": "...", "result": "invalid_code|code_disabled|batch_disabled|expired|code_used_up|user_limit|credential_unavailable|invalid_input" }
```

可选请求头：`Idempotency-Key: <uuid>`。同 key 同请求返回原结果；同 key 不同请求返回 409。

### POST /api/redeem/batch
为同一邮箱批量兑换 CDK。接口不设置兑换码条数上限；服务端按数组顺序逐条处理，每条使用独立事务，因此普通业务失败不会中断其他兑换码。每个兑换仍受单码使用次数、凭证库存和 Webhook 规则约束。请求整体仍受 64 KiB 请求体安全限制。

请求：
```json
{
  "user_id": "user@example.com",
  "codes": ["AAAA-BBBB-CCCC", "DDDD-EEEE-FFFF"]
}
```

响应（请求已完成处理时为 HTTP 200，即使其中部分兑换失败）：
```json
{
  "ok": true,
  "message": "批量兑换处理完成",
  "total": 2,
  "succeeded": 1,
  "failed": 1,
  "results": [
    {
      "code": "AAAA-BBBB-CCCC",
      "ok": true,
      "result": "success",
      "message": "兑换成功 🎉",
      "redemption": {
        "ok": true,
        "result": "success",
        "message": "兑换成功 🎉",
        "batch": "批次名称",
        "code": "AAAA-BBBB-CCCC",
        "user_id": "user@example.com",
        "payload": {},
        "redeemed_at": "2026-07-18 12:00:00"
      }
    },
    {
      "code": "DDDD-EEEE-FFFF",
      "ok": false,
      "result": "code_used_up",
      "message": "该兑换码已被使用完"
    }
  ]
}
```

同一请求中重复的兑换码不会再次执行，对应项返回 `invalid_input`。空数组、非法 JSON 或请求体过大等请求级错误返回 HTTP 400；超过限流额度返回 HTTP 429。批量接口按整次请求分别计入 IP 和邮箱限流，不按兑换码数量限制单批条数。

网页端收到批量结果后，会将所有成功项自动打包为一个 ZIP 下载。分配到凭证的成功项保存为独立凭证 JSON；未分配凭证的成功项保存为完整兑换结果 JSON。部分失败不会影响成功项打包，失败兑换码会保留供用户重试。

可选请求头：`Idempotency-Key`。同 key 同请求返回相同的整组结果；同 key 不同请求返回 409。前端应在网络重试时复用原 key，新的批量操作使用新 key。

### POST /api/convert
> ⚠️ 服务端转换器已移除，此端点不再存在。浏览器本地完成格式转换。

### POST /api/convert/download
> ⚠️ 服务端转换器已移除，此端点不再存在。浏览器本地完成格式转换后通过 FileSaver 等方式下载。

## 认证接口

### POST /api/admin/login
请求：`{ username: string, password: string }`
响应：`{ ok: true, username: string }`

### POST /api/admin/logout
响应：`{ ok: true }`

### GET /api/admin/me
响应：`{ ok: true, username: string }`（未登录返回 401）

## 管理接口

### GET /api/admin/stats
响应：
```json
{
  "ok": true,
  "stats": {
    "codes_total": 0,
    "codes_used": 0,
    "batches": 0,
    "redeem_total": 0,
    "today": 0,
    "webhook_failed": 0
  },
  "daily": [
    { "date": "2026-07-17", "count": 0 }
  ],
  "recent": [
    {
      "id": 1,
      "batch_name": "...",
      "code": "...",
      "user_id": "...",
      "result": "success",
      "message": "...",
      "created_at": "..."
    }
  ]
}
```
`daily` 固定返回最近 14 天。

### GET /api/admin/batches
响应：
```json
{
  "ok": true,
  "batches": [
    {
      "id": 1,
      "name": "...",
      "description": "...",
      "payload_json": {},
      "prefix": "",
      "code_length": 12,
      "expires_at": "2026-07-17T12:00",
      "max_uses_per_code": 1,
      "max_redeems_per_user": 1,
      "webhook_url": "",
      "webhook_secret": "abcd****wxyz",
      "assign_credential": false,
      "credential_plan_type": "",
      "status": "active",
      "created_at": "...",
      "code_count": 0,
      "used_count": 0
    }
  ]
}
```

### POST /api/admin/batches
创建批次。

请求字段：
- `name`: string, required
- `description`: string
- `payload_json`: JSON string, default `"{}"`
- `prefix`: string, max 8 alphanum, uppercased
- `code_length`: int, 8-20
- `count`: int, 1-10000
- `max_uses_per_code`: int, 1-100000
- `max_redeems_per_user`: 保留的兼容字段；邮箱仅用于记录，不再据此限制兑换次数
- `expires_at`: ISO datetime string or null
- `webhook_url`: 仅允许公网 HTTPS URL 或空字符串
- `webhook_secret`: string, max 512；为空且 webhook_url 非空时后端生成
- `assign_credential`: bool；启用后兑换必须成功分配凭证才会扣减额度
- `credential_plan_type`: string, max 100；启用凭证分配时必填，且对应套餐必须存在至少一条可用库存；否则返回 400，批次与兑换码均不会创建

验证失败返回 400 + `{ message, errors: string[] }`。

### GET /api/admin/batches/:id
响应：`{ ok: true, batch: {...}, counts: { total, disabled, usedup, total_uses } }`

### GET /api/admin/batches/:id/codes?state=all|active|partused|used|disabled&page=1
响应：`{ ok: true, codes: [...], max_uses, total, page, pages }`

### POST /api/admin/batches/:id/toggle
响应：`{ ok: true, status: "active|disabled", message }`

### POST /api/admin/codes/:id/toggle
响应：`{ ok: true, status: "unused|disabled", message }`

### GET /api/admin/batches/:id/export?fmt=txt|csv
直接返回文件下载。
- `txt`: 每行一个码，无 BOM
- `csv`: UTF-8 BOM，Excel 兼容

### GET /api/admin/logs?batch_id=&result=&webhook=&user_id=&page=1
响应：`{ ok: true, logs: [...], batches: [{id,name}], total, page, pages }`

### POST /api/admin/logs/:id/resend
响应：`{ ok: true, message }`

### GET /api/admin/faq
响应：`{ ok: true, items: [{id, sort_order, title, content, created_at}] }`

### POST /api/admin/faq
请求：`{ title, content }`
响应：`{ ok: true, id }`

### PUT /api/admin/faq/:id
请求：`{ title, content }`
响应：`{ ok: true }`

### DELETE /api/admin/faq/:id
响应：`{ ok: true }`

### POST /api/admin/faq/reorder
请求：`{ ids: number[] }`
响应：`{ ok: true }`

### GET /api/admin/credentials?state=all|used|unused&page=1&page_size=50
返回凭证元数据列表，不返回原始凭证 JSON。`page_size` 默认为 `50`，只接受 `20`、`50`、`100`、`200`、`500`，非法值返回 400。

响应包含 `{ items, total, page, pages, page_size }`，供管理端显示当前条目范围和页码。

### GET /api/admin/credentials/plans
返回按套餐汇总的凭证库存，供上传凭证和创建批次时选择套餐。`available` 表示未使用且未过期、当前可以分配的凭证数量。

响应：
```json
{
  "ok": true,
  "plans": [
    {
      "plan_type": "pro",
      "total": 10,
      "available": 6,
      "used": 3,
      "expired": 1
    }
  ]
}
```

### GET /api/admin/credentials/:id
返回单条凭证元数据，不返回原始 `data`。

### GET /api/admin/credentials/:id/download
以 `no-store` 附件返回原始凭证 JSON。

### POST /api/admin/credentials/upload
使用 `multipart/form-data` 上传 JSON 数组，最大 16 MiB：
- `file`：必填，待导入的 JSON 文件
- `plan_type`：必填，本次导入使用的套餐类型

服务端以表单中的 `plan_type` 为准，并覆盖上传 JSON 每一项原有的 `plan_type`；上传内容不应依赖或混用每项自带的套餐值。无效邮箱或过期时间的记录会被跳过。

### DELETE /api/admin/credentials/:id
删除单条凭证。

### POST /api/admin/credentials/batch-delete
请求：`{ ids: number[] }`。

### POST /api/admin/site-settings
使用 `multipart/form-data` 原子保存站点设置，且需要管理员会话和 CSRF 请求头。

- `site_title`：必填，去除首尾空白后为 1-100 个字符，不允许控制字符
- `logo`：可选，只接受内容真实有效的 PNG/JPEG，最大 2 MiB、最大 2048×2048 像素
- `remove_logo`：可选布尔值；为 `true` 时删除当前 Logo，不能与 `logo` 同时提交

响应格式与 `GET /api/site-settings` 相同，并额外包含 `message`。

## 状态码

- `invalid_input`: 邮箱或兑换码缺失，或字段长度不合法
- `invalid_code`: 码不存在
- `code_disabled`: 码被禁用
- `batch_disabled`: 批次被停用
- `expired`: 批次过期
- `code_used_up`: 码次数已用完
- `user_limit`: 用户在该批次已达上限
- `credential_unavailable`: 指定套餐没有未使用且未过期的凭证

## Webhook 状态

- `none`: 无 webhook
- `pending`: 待投递
- `success`: 投递成功
- `failed`: 投递失败
- `dead_letter`: 已耗尽自动重试次数，可由管理员重新入队

## 变更记录

- 2026-07-17: 冻结初始契约；移除服务端 `/api/convert` 和 `/api/convert/download`。
- 2026-07-18: 统一 Go API DTO；增加凭证库存、详情与下载接口；补充凭证不足和 Webhook 死信状态。
- 2026-07-18: 增加站点标题与 Logo 配置接口；兑换页联系方式简化为邮箱记录。
- 2026-07-18: 增加凭证套餐库存接口；上传凭证统一指定套餐；创建凭证分配批次前校验可用库存。
