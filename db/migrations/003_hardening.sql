-- +goose Up
-- +goose StatementBegin

ALTER TYPE redemption_result ADD VALUE IF NOT EXISTS 'credential_unavailable';

ALTER TABLE batches
    ADD COLUMN assign_credential BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN credential_plan_type TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT chk_batch_credential_plan_length
        CHECK (LENGTH(credential_plan_type) <= 100),
    ADD CONSTRAINT chk_batch_credential_plan_required
        CHECK (NOT assign_credential OR LENGTH(TRIM(credential_plan_type)) > 0);

ALTER TABLE redemptions DROP CONSTRAINT IF EXISTS chk_redemption_code_text_length;
ALTER TABLE redemptions DROP CONSTRAINT IF EXISTS chk_redemption_user_id_length;
ALTER TABLE redemptions DROP CONSTRAINT IF EXISTS chk_redemption_user_key_length;
ALTER TABLE redemptions
    ADD CONSTRAINT chk_redemption_code_text_length CHECK (LENGTH(code_text) <= 128),
    ADD CONSTRAINT chk_redemption_user_id_length CHECK (LENGTH(user_id) <= 256),
    ADD CONSTRAINT chk_redemption_user_key_length CHECK (LENGTH(user_key) <= 256);

ALTER TABLE idempotency_keys
    ADD CONSTRAINT chk_idempotency_key_length CHECK (LENGTH(key) BETWEEN 1 AND 128);

ALTER TABLE credentials
    ADD CONSTRAINT chk_credential_email_length CHECK (LENGTH(email) BETWEEN 1 AND 320),
    ADD CONSTRAINT chk_credential_plan_length CHECK (LENGTH(plan_type) BETWEEN 1 AND 100);

CREATE INDEX idx_credentials_available
    ON credentials(plan_type, id)
    WHERE used = false;

SELECT setval(
    pg_get_serial_sequence('batches', 'id'),
    COALESCE((SELECT MAX(id) FROM batches), 1),
    EXISTS (SELECT 1 FROM batches)
);
SELECT setval(
    pg_get_serial_sequence('codes', 'id'),
    COALESCE((SELECT MAX(id) FROM codes), 1),
    EXISTS (SELECT 1 FROM codes)
);
SELECT setval(
    pg_get_serial_sequence('redemptions', 'id'),
    COALESCE((SELECT MAX(id) FROM redemptions), 1),
    EXISTS (SELECT 1 FROM redemptions)
);
SELECT setval(
    pg_get_serial_sequence('faq_items', 'id'),
    COALESCE((SELECT MAX(id) FROM faq_items), 1),
    EXISTS (SELECT 1 FROM faq_items)
);
SELECT setval(
    pg_get_serial_sequence('credentials', 'id'),
    COALESCE((SELECT MAX(id) FROM credentials), 1),
    EXISTS (SELECT 1 FROM credentials)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_credentials_available;
ALTER TABLE credentials
    DROP CONSTRAINT IF EXISTS chk_credential_email_length,
    DROP CONSTRAINT IF EXISTS chk_credential_plan_length;
ALTER TABLE idempotency_keys
    DROP CONSTRAINT IF EXISTS chk_idempotency_key_length;
ALTER TABLE batches
    DROP CONSTRAINT IF EXISTS chk_batch_credential_plan_required,
    DROP CONSTRAINT IF EXISTS chk_batch_credential_plan_length,
    DROP COLUMN IF EXISTS credential_plan_type,
    DROP COLUMN IF EXISTS assign_credential;

-- PostgreSQL enum values cannot be safely removed in a down migration.

-- +goose StatementEnd
