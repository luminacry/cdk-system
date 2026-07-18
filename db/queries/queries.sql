-- name: GetAdminByUsername :one
SELECT id, username, password_hash, session_version, created_at, updated_at
FROM admins
WHERE username = $1;

-- name: CountAdmins :one
SELECT COUNT(*) FROM admins;

-- name: CreateAdmin :one
INSERT INTO admins (username, password_hash)
VALUES ($1, $2)
RETURNING id, username, password_hash, session_version, created_at, updated_at;

-- name: IncrementAdminSessionVersion :exec
UPDATE admins
SET session_version = session_version + 1, updated_at = NOW()
WHERE id = $1;

-- name: CreateBatch :one
INSERT INTO batches (
    name, description, payload_json, prefix, code_length,
    expires_at, max_uses_per_code, max_redeems_per_user,
    webhook_url, webhook_secret, assign_credential, credential_plan_type, status
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12, $13
)
RETURNING *;

-- name: ListBatches :many
SELECT
    b.*,
    COUNT(c.id) AS code_count,
    COUNT(c.id) FILTER (WHERE c.use_count > 0) AS used_count
FROM batches b
LEFT JOIN codes c ON c.batch_id = b.id
GROUP BY b.id
ORDER BY b.created_at DESC;

-- name: GetBatchByID :one
SELECT * FROM batches WHERE id = $1;

-- name: ToggleBatchStatus :one
UPDATE batches
SET status = CASE
    WHEN status = 'active' THEN 'disabled'::batch_status
    ELSE 'active'::batch_status
END
WHERE id = $1
RETURNING *;

-- name: GetBatchCounts :one
SELECT
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE c.status = 'disabled') AS disabled,
    COUNT(*) FILTER (WHERE c.use_count >= b.max_uses_per_code AND c.status != 'disabled') AS usedup,
    COALESCE(SUM(c.use_count), 0)::bigint AS total_uses
FROM codes c
JOIN batches b ON b.id = c.batch_id
WHERE c.batch_id = $1;

-- name: CreateCode :exec
INSERT INTO codes (batch_id, code)
VALUES ($1, $2);

-- name: CreateCodes :copyfrom
INSERT INTO codes (batch_id, code)
VALUES ($1, $2);

-- name: GetCodeByCode :one
SELECT c.*, b.name AS batch_name, b.status AS batch_status, b.prefix,
       b.payload_json, b.expires_at, b.max_uses_per_code, b.max_redeems_per_user,
       b.webhook_url, b.assign_credential, b.credential_plan_type
FROM codes c
JOIN batches b ON b.id = c.batch_id
WHERE c.code = $1
FOR UPDATE OF c;

-- name: ListCodesByBatch :many
SELECT c.*,
    CASE
        WHEN c.status = 'disabled' THEN 'disabled'
        WHEN c.use_count >= b.max_uses_per_code THEN 'used'
        WHEN c.use_count > 0 THEN 'partused'
        ELSE 'active'
    END AS state
FROM codes c
JOIN batches b ON b.id = c.batch_id
WHERE c.batch_id = $1
ORDER BY c.id DESC
LIMIT $2 OFFSET $3;

-- name: ListCodesByBatchAndState :many
SELECT c.*,
    CASE
        WHEN c.status = 'disabled' THEN 'disabled'
        WHEN c.use_count >= b.max_uses_per_code THEN 'used'
        WHEN c.use_count > 0 THEN 'partused'
        ELSE 'active'
    END AS state
FROM codes c
JOIN batches b ON b.id = c.batch_id
WHERE c.batch_id = $1
  AND (
      CASE
          WHEN c.status = 'disabled' THEN 'disabled'
          WHEN c.use_count >= b.max_uses_per_code THEN 'used'
          WHEN c.use_count > 0 THEN 'partused'
          ELSE 'active'
      END
  ) = sqlc.arg(state)::text
ORDER BY c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCodesByBatch :one
SELECT COUNT(*) FROM codes WHERE batch_id = $1;

-- name: CountCodesByBatchAndState :one
SELECT COUNT(*) FROM codes c
JOIN batches b ON b.id = c.batch_id
WHERE c.batch_id = $1
  AND (
      CASE
          WHEN c.status = 'disabled' THEN 'disabled'
          WHEN c.use_count >= b.max_uses_per_code THEN 'used'
          WHEN c.use_count > 0 THEN 'partused'
          ELSE 'active'
      END
  ) = sqlc.arg(state)::text;

-- name: ToggleCodeStatus :one
UPDATE codes
SET status = CASE
    WHEN status = 'unused' THEN 'disabled'::code_status
    ELSE 'unused'::code_status
END
WHERE id = $1
RETURNING *;

-- name: IncrementCodeUseCount :one
UPDATE codes
SET use_count = use_count + 1
WHERE id = $1
  AND status = 'unused'
  AND use_count < $2
RETURNING id, use_count;

-- name: CreateIdempotencyKey :one
INSERT INTO idempotency_keys (key, request_hash)
VALUES ($1, $2)
ON CONFLICT (key) DO NOTHING
RETURNING *;

-- name: GetIdempotencyKeyByKey :one
SELECT * FROM idempotency_keys WHERE key = $1;

-- name: LockIdempotencyKeyByKey :one
SELECT * FROM idempotency_keys WHERE key = $1 FOR UPDATE;

-- name: SetIdempotencyResponse :exec
UPDATE idempotency_keys
SET response_body = $2
WHERE key = $1;

-- name: CreateRedemption :one
INSERT INTO redemptions (
    code_id, batch_id, code_text, user_id, user_key,
    payload_snapshot, result, message, webhook_status,
    webhook_attempt, ip, idempotency_key, event_id
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13
)
RETURNING *;

-- name: GetRedemptionByID :one
SELECT * FROM redemptions WHERE id = $1;

-- name: ListRedemptions :many
SELECT
    r.id,
    r.created_at,
    b.name AS batch_name,
    r.code_text AS code,
    r.user_id,
    r.result,
    r.message,
    r.webhook_status,
    r.webhook_response,
    host(r.ip) AS ip
FROM redemptions r
LEFT JOIN batches b ON b.id = r.batch_id
WHERE (sqlc.arg(batch_id)::bigint = 0 OR r.batch_id = sqlc.arg(batch_id)::bigint)
  AND (sqlc.arg(result_filter)::text = '' OR r.result::text = sqlc.arg(result_filter)::text)
  AND (sqlc.arg(webhook_filter)::text = '' OR r.webhook_status::text = sqlc.arg(webhook_filter)::text)
  AND (sqlc.arg(user_filter)::text = '' OR r.user_key ILIKE '%' || sqlc.arg(user_filter)::text || '%')
ORDER BY r.created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRedemptions :one
SELECT COUNT(*) FROM redemptions
WHERE (sqlc.arg(batch_id)::bigint = 0 OR batch_id = sqlc.arg(batch_id)::bigint)
  AND (sqlc.arg(result_filter)::text = '' OR result::text = sqlc.arg(result_filter)::text)
  AND (sqlc.arg(webhook_filter)::text = '' OR webhook_status::text = sqlc.arg(webhook_filter)::text)
  AND (sqlc.arg(user_filter)::text = '' OR user_key ILIKE '%' || sqlc.arg(user_filter)::text || '%');

-- name: UpdateRedemptionWebhookStatus :execrows
UPDATE redemptions
SET webhook_status = sqlc.arg(webhook_status)::webhook_status,
    webhook_response = sqlc.arg(webhook_response),
    webhook_attempt = webhook_attempt + 1
WHERE id = sqlc.arg(id)
  AND event_id = sqlc.arg(event_id);

-- name: PrepareRedemptionWebhookResend :execrows
UPDATE redemptions
SET webhook_status = 'pending',
    webhook_response = '',
    event_id = sqlc.arg(event_id)
WHERE id = sqlc.arg(id)
  AND result = 'success'
  AND webhook_status IN ('success', 'failed', 'dead_letter');

-- name: GetStats :one
SELECT
    (SELECT COUNT(*) FROM codes) AS codes_total,
    (SELECT COUNT(*) FROM codes WHERE use_count > 0) AS codes_used,
    (SELECT COUNT(*) FROM batches) AS batches,
    (SELECT COUNT(*) FROM redemptions WHERE result = 'success') AS redeem_total,
    (SELECT COUNT(*) FROM redemptions WHERE result = 'success' AND created_at >= DATE_TRUNC('day', NOW())) AS today,
    (SELECT COUNT(*) FROM redemptions WHERE webhook_status IN ('failed', 'dead_letter')) AS webhook_failed;

-- name: GetDailyRedemptions :many
WITH days AS (
    SELECT generate_series(
        DATE_TRUNC('day', NOW()) - INTERVAL '13 days',
        DATE_TRUNC('day', NOW()),
        INTERVAL '1 day'
    )::date AS date
)
SELECT
    d.date,
    COUNT(r.id) FILTER (WHERE r.result = 'success') AS count
FROM days d
LEFT JOIN redemptions r
  ON r.created_at >= d.date::timestamptz
 AND r.created_at < d.date::timestamptz + INTERVAL '1 day'
 AND r.created_at >= DATE_TRUNC('day', NOW()) - INTERVAL '13 days'
GROUP BY d.date
ORDER BY d.date ASC;

-- name: GetRecentRedemptions :many
SELECT
    r.id,
    r.created_at,
    b.name AS batch_name,
    r.code_text AS code,
    r.user_id,
    r.result,
    r.message,
    r.webhook_status
FROM redemptions r
LEFT JOIN batches b ON b.id = r.batch_id
ORDER BY r.created_at DESC
LIMIT 8;

-- name: CreateWebhookOutboxEvent :one
INSERT INTO webhook_outbox (event_id, redemption_id, status, scheduled_at)
VALUES ($1, $2, 'pending', NOW())
RETURNING *;

-- name: LeaseWebhookOutboxEvent :one
UPDATE webhook_outbox
SET
    status = 'pending',
    lease_token = $1,
    lease_expires_at = NOW() + INTERVAL '2 minutes',
    attempts = attempts + 1
WHERE id = (
    SELECT id FROM webhook_outbox
    WHERE status IN ('pending', 'failed')
      AND scheduled_at <= NOW()
      AND (lease_expires_at IS NULL OR lease_expires_at <= NOW())
    ORDER BY scheduled_at ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING *;

-- name: UpdateWebhookOutboxSuccess :execrows
UPDATE webhook_outbox
SET status = 'success',
    last_response = sqlc.arg(last_response),
    lease_token = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg(id) AND lease_token = sqlc.arg(lease_token);

-- name: UpdateWebhookOutboxFailed :execrows
UPDATE webhook_outbox
SET status = sqlc.arg(status)::webhook_status,
    last_response = sqlc.arg(last_response),
    scheduled_at = sqlc.arg(scheduled_at),
    lease_token = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg(id) AND lease_token = sqlc.arg(lease_token);

-- name: ListFAQ :many
SELECT * FROM faq_items ORDER BY sort_order ASC, id ASC;

-- name: CreateFAQ :one
INSERT INTO faq_items (sort_order, title, content)
VALUES (
    COALESCE((SELECT MAX(sort_order) FROM faq_items), 0) + 1,
    $1, $2
)
RETURNING *;

-- name: GetFAQByID :one
SELECT * FROM faq_items WHERE id = $1;

-- name: UpdateFAQ :one
UPDATE faq_items
SET title = $2, content = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteFAQ :exec
DELETE FROM faq_items WHERE id = $1;

-- name: ReorderFAQ :exec
UPDATE faq_items
SET sort_order = s.ord
FROM unnest($1::bigint[]) WITH ORDINALITY AS s(id, ord)
WHERE faq_items.id = s.id;

-- name: CreateCredential :one
INSERT INTO credentials (email, plan_type, expired, data)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListCredentials :many
SELECT id, email, plan_type, expired, used, redemption_id, created_at, updated_at
FROM credentials
WHERE (
    sqlc.arg(state_filter)::text = 'all'
    OR (sqlc.arg(state_filter)::text = 'used' AND used = true)
    OR (sqlc.arg(state_filter)::text = 'unused' AND used = false)
)
ORDER BY id DESC
LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: CountCredentials :one
SELECT COUNT(*) FROM credentials
WHERE (
    sqlc.arg(state_filter)::text = 'all'
    OR (sqlc.arg(state_filter)::text = 'used' AND used = true)
    OR (sqlc.arg(state_filter)::text = 'unused' AND used = false)
);

-- name: GetCredentialByID :one
SELECT * FROM credentials WHERE id = $1;

-- name: DeleteCredential :exec
DELETE FROM credentials WHERE id = $1;

-- name: DeleteCredentialsByIDs :execrows
DELETE FROM credentials WHERE id = ANY($1::bigint[]);

-- name: GetUnusedCredential :one
SELECT * FROM credentials
WHERE used = false
  AND (expired IS NULL OR expired > NOW())
  AND (sqlc.arg(plan_type)::text = '' OR plan_type = sqlc.arg(plan_type)::text)
ORDER BY id ASC
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: MarkCredentialUsed :one
UPDATE credentials
SET used = true, redemption_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: CountCredentialsByStatus :one
SELECT
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE used = true) AS used,
    COUNT(*) FILTER (WHERE used = false) AS unused
FROM credentials;

-- name: ListCredentialPlans :many
SELECT
    plan_type,
    COUNT(*) AS total,
    COUNT(*) FILTER (
        WHERE used = false AND (expired IS NULL OR expired > NOW())
    ) AS available,
    COUNT(*) FILTER (WHERE used = true) AS used,
    COUNT(*) FILTER (
        WHERE used = false AND expired IS NOT NULL AND expired <= NOW()
    ) AS expired
FROM credentials
GROUP BY plan_type
ORDER BY plan_type COLLATE "C";

-- name: CountAvailableCredentialsByPlan :one
SELECT COUNT(*)
FROM credentials
WHERE plan_type = $1
  AND used = false
  AND (expired IS NULL OR expired > NOW());

-- name: GetSiteSettings :one
SELECT
    site_title,
    logo_content_type,
    updated_at
FROM site_settings
WHERE singleton = true;

-- name: GetSiteLogo :one
SELECT
    logo_data,
    COALESCE(logo_content_type, '') AS logo_content_type,
    updated_at
FROM site_settings
WHERE singleton = true
  AND logo_data IS NOT NULL;

-- name: UpdateSiteSettings :one
UPDATE site_settings
SET
    site_title = sqlc.arg(site_title),
    logo_data = CASE
        WHEN sqlc.arg(update_logo)::boolean
            THEN NULLIF(sqlc.arg(logo_data)::bytea, ''::bytea)
        ELSE logo_data
    END,
    logo_content_type = CASE
        WHEN sqlc.arg(update_logo)::boolean
            THEN NULLIF(sqlc.arg(logo_content_type)::text, '')
        ELSE logo_content_type
    END
WHERE singleton = true
RETURNING
    site_title,
    logo_content_type,
    updated_at;
