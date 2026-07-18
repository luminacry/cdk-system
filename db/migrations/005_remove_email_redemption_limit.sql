-- +goose Up
-- +goose StatementBegin

DROP TABLE IF EXISTS user_batch_usage;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

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

CREATE TRIGGER user_batch_usage_updated_at BEFORE UPDATE ON user_batch_usage
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose StatementEnd
