CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    token      VARCHAR(128) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used_at    TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_token
ON password_reset_tokens(token);
