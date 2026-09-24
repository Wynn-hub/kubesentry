SET search_path TO kubesentry;

CREATE TABLE tenants (
    id         BIGINT PRIMARY KEY,
    name       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX tenants_name_key ON tenants (name) WHERE deleted_at IS NULL;

CREATE TABLE users (
    id            BIGINT PRIMARY KEY,
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL CHECK (role IN ('system_admin', 'tenant_admin', 'viewer')),
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    tenant_id     BIGINT REFERENCES tenants (id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    CHECK (
        (role = 'system_admin' AND tenant_id IS NULL) OR
        (role IN ('tenant_admin', 'viewer') AND tenant_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX users_username_key ON users (username) WHERE deleted_at IS NULL;
INSERT INTO users (id, username, role) VALUES (2081000000000000001, 'admin', 'system_admin');

CREATE TABLE sessions (
    id           BIGINT PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users (id),
    token_hash   TEXT NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX sessions_user_active_idx ON sessions (user_id) WHERE revoked_at IS NULL AND deleted_at IS NULL;

ALTER TABLE clusters ADD COLUMN tenant_id BIGINT REFERENCES tenants (id);

ALTER TABLE policies
    ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'system' CHECK (owner_type IN ('system', 'tenant')),
    ADD COLUMN tenant_id BIGINT REFERENCES tenants (id),
    ADD CHECK ((owner_type = 'system' AND tenant_id IS NULL) OR (owner_type = 'tenant' AND tenant_id IS NOT NULL));

ALTER TABLE policy_groups
    ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'system' CHECK (owner_type IN ('system', 'tenant')),
    ADD COLUMN tenant_id BIGINT REFERENCES tenants (id),
    ADD CHECK ((owner_type = 'system' AND tenant_id IS NULL) OR (owner_type = 'tenant' AND tenant_id IS NOT NULL));
DROP INDEX policy_groups_name_key;
CREATE UNIQUE INDEX policy_groups_owner_name_key
    ON policy_groups (COALESCE(tenant_id, 0), name) WHERE deleted_at IS NULL;

ALTER TABLE policy_exceptions
    ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'system' CHECK (owner_type IN ('system', 'tenant')),
    ADD COLUMN tenant_id BIGINT REFERENCES tenants (id),
    ADD CHECK ((owner_type = 'system' AND tenant_id IS NULL) OR (owner_type = 'tenant' AND tenant_id IS NOT NULL));
DROP INDEX policy_exceptions_name_key;
CREATE UNIQUE INDEX policy_exceptions_owner_name_key
    ON policy_exceptions (COALESCE(tenant_id, 0), name) WHERE deleted_at IS NULL;

ALTER TABLE policy_distributions
    ADD COLUMN resource_owner_type TEXT NOT NULL DEFAULT 'system' CHECK (resource_owner_type IN ('system', 'tenant')),
    ADD COLUMN tenant_id BIGINT REFERENCES tenants (id),
    ADD CHECK ((resource_owner_type = 'system' AND tenant_id IS NULL) OR (resource_owner_type = 'tenant' AND tenant_id IS NOT NULL));
DROP INDEX policy_distributions_name_key;
CREATE UNIQUE INDEX policy_distributions_owner_name_scope_key
    ON policy_distributions (COALESCE(tenant_id, 0), name) WHERE deleted_at IS NULL;
