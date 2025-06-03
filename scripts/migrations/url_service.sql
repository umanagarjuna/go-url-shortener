CREATE TABLE IF NOT EXISTS urls (
                                    id BIGSERIAL PRIMARY KEY,
                                    short_code VARCHAR(10) UNIQUE NOT NULL,
    original_url TEXT NOT NULL,
    user_id BIGINT NOT NULL,  -- Fixed: moved NOT NULL before data type
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    click_count BIGINT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
    );

-- Create indexes separately (PostgreSQL uses CREATE INDEX, not INDEX in table definition)
CREATE INDEX IF NOT EXISTS idx_urls_short_code ON urls (short_code);
CREATE INDEX IF NOT EXISTS idx_urls_user_id ON urls (user_id);
CREATE INDEX IF NOT EXISTS idx_urls_created_at ON urls (created_at);
CREATE INDEX IF NOT EXISTS idx_urls_updated_at ON urls (updated_at);

-- Additional useful indexes for your use case
CREATE INDEX IF NOT EXISTS idx_urls_user_url ON urls (user_id, original_url);
CREATE INDEX IF NOT EXISTS idx_urls_active ON urls (is_active);
CREATE INDEX IF NOT EXISTS idx_urls_expires_at ON urls (expires_at) WHERE expires_at IS NOT NULL;
