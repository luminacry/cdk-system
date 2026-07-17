-- +goose Up
-- +goose StatementBegin
CREATE TABLE credentials (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL,
    plan_type TEXT NOT NULL,
    expired TIMESTAMPTZ,
    data JSONB NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false,
    redemption_id BIGINT REFERENCES redemptions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_credentials_used ON credentials(used);
CREATE INDEX idx_credentials_redemption ON credentials(redemption_id);

SELECT setval('credentials_id_seq', 1, false);

-- Reuse existing trigger function for updated_at
CREATE TRIGGER update_credentials_updated_at
    BEFORE UPDATE ON credentials
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS credentials;
-- +goose StatementEnd
