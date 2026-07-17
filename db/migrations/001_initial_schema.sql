-- +goose Up
-- +goose StatementBegin

CREATE TYPE batch_status AS ENUM ('active', 'disabled');
CREATE TYPE code_status AS ENUM ('unused', 'disabled');
CREATE TYPE redemption_result AS ENUM (
    'success',
    'invalid_input',
    'invalid_code',
    'code_disabled',
    'batch_disabled',
    'expired',
    'code_used_up',
    'user_limit',
    'rate_limited'
);
CREATE TYPE webhook_status AS ENUM ('none', 'pending', 'success', 'failed', 'dead_letter');

CREATE TABLE admins (
    id              BIGSERIAL PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    session_version BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE batches (
    id                   BIGSERIAL PRIMARY KEY,
    name                 TEXT NOT NULL,
    description          TEXT NOT NULL DEFAULT '',
    payload_json         JSONB NOT NULL DEFAULT '{}'::jsonb,
    prefix               TEXT NOT NULL DEFAULT '',
    code_length          INTEGER NOT NULL DEFAULT 12,
    expires_at           TIMESTAMPTZ,
    max_uses_per_code    INTEGER NOT NULL DEFAULT 1,
    max_redeems_per_user INTEGER NOT NULL DEFAULT 1,
    webhook_url          TEXT NOT NULL DEFAULT '',
    webhook_secret       TEXT NOT NULL DEFAULT '',
    status               batch_status NOT NULL DEFAULT 'active',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_batch_name_length CHECK (LENGTH(name) BETWEEN 1 AND 200),
    CONSTRAINT chk_batch_description_length CHECK (LENGTH(description) <= 1000),
    CONSTRAINT chk_batch_prefix_format CHECK (prefix ~ '^[A-Z0-9]*$' AND LENGTH(prefix) <= 8),
    CONSTRAINT chk_batch_code_length CHECK (code_length BETWEEN 8 AND 20),
    CONSTRAINT chk_batch_max_uses_positive CHECK (max_uses_per_code BETWEEN 1 AND 100000),
    CONSTRAINT chk_batch_max_redeems_positive CHECK (max_redeems_per_user BETWEEN 1 AND 100000),
    CONSTRAINT chk_batch_webhook_url_length CHECK (LENGTH(webhook_url) <= 2000)
);

CREATE TABLE codes (
    id         BIGSERIAL PRIMARY KEY,
    batch_id   BIGINT NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    code       TEXT NOT NULL UNIQUE,
    status     code_status NOT NULL DEFAULT 'unused',
    use_count  INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_code_format CHECK (code ~ '^[A-Z0-9]+$' AND LENGTH(code) BETWEEN 1 AND 64),
    CONSTRAINT chk_code_use_count_nonnegative CHECK (use_count >= 0)
);

CREATE INDEX idx_codes_batch ON codes(batch_id);
CREATE INDEX idx_codes_status ON codes(status);

CREATE TABLE user_batch_usage (
    batch_id    BIGINT NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    user_key    TEXT NOT NULL,
    used_count  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (batch_id, user_key),
    CONSTRAINT chk_user_usage_nonnegative CHECK (used_count >= 0),
    CONSTRAINT chk_user_key_length CHECK (LENGTH(user_key) BETWEEN 1 AND 256)
);

CREATE TABLE idempotency_keys (
    id            BIGSERIAL PRIMARY KEY,
    key           TEXT NOT NULL UNIQUE,
    request_hash  BYTEA NOT NULL,
    response_body JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_idempotency_keys_created_at ON idempotency_keys(created_at);

CREATE TABLE redemptions (
    id                BIGSERIAL PRIMARY KEY,
    code_id           BIGINT REFERENCES codes(id) ON DELETE SET NULL,
    batch_id          BIGINT REFERENCES batches(id) ON DELETE SET NULL,
    code_text         TEXT NOT NULL,
    user_id           TEXT NOT NULL,
    user_key          TEXT NOT NULL,
    payload_snapshot  JSONB NOT NULL DEFAULT '{}'::jsonb,
    result            redemption_result NOT NULL,
    message           TEXT NOT NULL DEFAULT '',
    webhook_status    webhook_status NOT NULL DEFAULT 'none',
    webhook_response  TEXT NOT NULL DEFAULT '',
    webhook_attempt   INTEGER NOT NULL DEFAULT 0,
    ip                INET NOT NULL DEFAULT '0.0.0.0',
    idempotency_key   TEXT REFERENCES idempotency_keys(key) ON DELETE SET NULL,
    event_id          TEXT UNIQUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_redemption_code_text_length CHECK (LENGTH(code_text) BETWEEN 1 AND 128),
    CONSTRAINT chk_redemption_user_id_length CHECK (LENGTH(user_id) BETWEEN 1 AND 256),
    CONSTRAINT chk_redemption_user_key_length CHECK (LENGTH(user_key) BETWEEN 1 AND 256),
    CONSTRAINT chk_redemption_message_length CHECK (LENGTH(message) <= 500),
    CONSTRAINT chk_redemption_webhook_response_length CHECK (LENGTH(webhook_response) <= 500),
    CONSTRAINT chk_redemption_webhook_attempt_nonnegative CHECK (webhook_attempt >= 0),
    CONSTRAINT chk_redemption_event_id_length CHECK (event_id IS NULL OR LENGTH(event_id) <= 64)
);

CREATE INDEX idx_redemptions_batch ON redemptions(batch_id);
CREATE INDEX idx_redemptions_user_key ON redemptions(user_key);
CREATE INDEX idx_redemptions_result ON redemptions(result);
CREATE INDEX idx_redemptions_created_at ON redemptions(created_at DESC);
CREATE INDEX idx_redemptions_batch_result ON redemptions(batch_id, result);
CREATE INDEX idx_redemptions_webhook_status ON redemptions(webhook_status);

CREATE TABLE webhook_outbox (
    id              BIGSERIAL PRIMARY KEY,
    event_id        TEXT NOT NULL UNIQUE,
    redemption_id   BIGINT NOT NULL REFERENCES redemptions(id) ON DELETE CASCADE,
    status          webhook_status NOT NULL DEFAULT 'pending',
    scheduled_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_token     TEXT,
    lease_expires_at TIMESTAMPTZ,
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_response   TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_outbox_event_id_length CHECK (LENGTH(event_id) <= 64),
    CONSTRAINT chk_outbox_attempts_nonnegative CHECK (attempts >= 0),
    CONSTRAINT chk_outbox_last_response_length CHECK (LENGTH(last_response) <= 500)
);

CREATE INDEX idx_webhook_outbox_pending ON webhook_outbox(status, scheduled_at)
    WHERE status IN ('pending', 'failed');
CREATE INDEX idx_webhook_outbox_lease ON webhook_outbox(status, lease_expires_at)
    WHERE status IN ('pending', 'failed');

CREATE TABLE faq_items (
    id         BIGSERIAL PRIMARY KEY,
    sort_order INTEGER NOT NULL,
    title      TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_faq_title_length CHECK (LENGTH(title) BETWEEN 1 AND 200),
    CONSTRAINT chk_faq_content_length CHECK (LENGTH(content) BETWEEN 1 AND 4000)
);

CREATE UNIQUE INDEX idx_faq_sort_order ON faq_items(sort_order);

-- Default FAQ entries
INSERT INTO faq_items (id, sort_order, title, content) VALUES
(1, 1, '如何获取兑换码？', '兑换码（CDK）通过官方活动、社群福利、合作渠道发放，请认准官方渠道，谨防诈骗。'),
(2, 2, '兑换码的使用规则是什么？', '每个兑换码默认只能兑换一次，同一用户在同一活动中可能有限兑次数。兑换码可能设有有效期，过期将无法使用。'),
(3, 3, '提示「已被使用 / 已过期 / 无效」怎么办？', '请核对兑换码是否输入正确（注意区分大小写、不要有空格）；确认无误仍失败，说明该码已被使用或过期，请联系发放方。'),
(4, 4, '兑换成功后奖励如何到账？', '兑换成功后系统会自动发放奖励并展示兑换内容，请妥善保存页面展示的信息。如长时间未到账，请携带兑换记录联系客服。')
ON CONFLICT (id) DO NOTHING;

-- Function to update updated_at automatically
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER batches_updated_at BEFORE UPDATE ON batches
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER user_batch_usage_updated_at BEFORE UPDATE ON user_batch_usage
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER webhook_outbox_updated_at BEFORE UPDATE ON webhook_outbox
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER faq_items_updated_at BEFORE UPDATE ON faq_items
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS faq_items_updated_at ON faq_items;
DROP TRIGGER IF EXISTS webhook_outbox_updated_at ON webhook_outbox;
DROP TRIGGER IF EXISTS user_batch_usage_updated_at ON user_batch_usage;
DROP TRIGGER IF EXISTS batches_updated_at ON batches;
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS webhook_outbox;
DROP TABLE IF EXISTS redemptions;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS user_batch_usage;
DROP TABLE IF EXISTS codes;
DROP TABLE IF EXISTS batches;
DROP TABLE IF EXISTS admins;
DROP TABLE IF EXISTS faq_items;

DROP TYPE IF EXISTS webhook_status;
DROP TYPE IF EXISTS redemption_result;
DROP TYPE IF EXISTS code_status;
DROP TYPE IF EXISTS batch_status;

-- +goose StatementEnd
