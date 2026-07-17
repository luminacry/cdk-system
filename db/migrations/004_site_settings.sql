-- +goose Up
-- +goose StatementBegin

CREATE TABLE site_settings (
    singleton         BOOLEAN PRIMARY KEY DEFAULT true,
    site_title        TEXT NOT NULL DEFAULT 'CDK 兑换中心',
    logo_data         BYTEA,
    logo_content_type TEXT,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_site_settings_singleton CHECK (singleton),
    CONSTRAINT chk_site_title_length CHECK (
        site_title = BTRIM(site_title)
        AND CHAR_LENGTH(site_title) BETWEEN 1 AND 100
    ),
    CONSTRAINT chk_site_logo_pair CHECK (
        (logo_data IS NULL AND logo_content_type IS NULL)
        OR (logo_data IS NOT NULL AND logo_content_type IS NOT NULL)
    ),
    CONSTRAINT chk_site_logo_size CHECK (
        logo_data IS NULL OR OCTET_LENGTH(logo_data) <= 2097152
    ),
    CONSTRAINT chk_site_logo_content_type CHECK (
        logo_content_type IS NULL OR logo_content_type IN ('image/png', 'image/jpeg')
    )
);

INSERT INTO site_settings (singleton, site_title)
VALUES (true, 'CDK 兑换中心');

CREATE TRIGGER site_settings_updated_at BEFORE UPDATE ON site_settings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS site_settings_updated_at ON site_settings;
DROP TABLE IF EXISTS site_settings;

-- +goose StatementEnd
