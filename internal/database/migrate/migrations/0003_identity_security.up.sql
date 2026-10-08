SET search_path TO kubesentry;

ALTER TABLE users
    ADD COLUMN last_login_at TIMESTAMPTZ,
    ADD COLUMN last_login_ip TEXT,
    ADD COLUMN password_changed_at TIMESTAMPTZ,
    ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN failed_login_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
    ADD COLUMN locked_until TIMESTAMPTZ;

UPDATE users
SET password_changed_at = updated_at
WHERE password_hash <> '' AND password_changed_at IS NULL;

CREATE TABLE user_activation_tokens (
    id         BIGINT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX user_activation_tokens_active_user_key
    ON user_activation_tokens (user_id) WHERE used_at IS NULL AND deleted_at IS NULL;

CREATE TABLE password_history (
    id            BIGINT PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX password_history_user_created_idx
    ON password_history (user_id, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE security_settings (
    id                           BIGINT PRIMARY KEY CHECK (id = 1),
    activation_lifetime_hours    INTEGER NOT NULL DEFAULT 24 CHECK (activation_lifetime_hours BETWEEN 1 AND 720),
    password_lifetime_days       INTEGER NOT NULL DEFAULT 0 CHECK (password_lifetime_days BETWEEN 0 AND 3650),
    password_min_length          INTEGER NOT NULL DEFAULT 12 CHECK (password_min_length BETWEEN 8 AND 128),
    password_history_count       INTEGER NOT NULL DEFAULT 5 CHECK (password_history_count BETWEEN 0 AND 24),
    max_failed_login_attempts    INTEGER NOT NULL DEFAULT 5 CHECK (max_failed_login_attempts BETWEEN 1 AND 100),
    lockout_duration_minutes     INTEGER NOT NULL DEFAULT 15 CHECK (lockout_duration_minutes BETWEEN 1 AND 10080),
    session_idle_minutes         INTEGER NOT NULL DEFAULT 30 CHECK (session_idle_minutes BETWEEN 1 AND 10080),
    session_absolute_minutes     INTEGER NOT NULL DEFAULT 480 CHECK (session_absolute_minutes BETWEEN 1 AND 43200),
    allow_concurrent_sessions    BOOLEAN NOT NULL DEFAULT false,
    updated_by                   BIGINT REFERENCES users (id),
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO security_settings (id) VALUES (1);
