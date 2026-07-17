CREATE SCHEMA IF NOT EXISTS kubesentry;
SET search_path TO kubesentry;

CREATE TABLE clusters (
    id                 BIGINT PRIMARY KEY,
    name               TEXT NOT NULL,
    cluster_uid        TEXT NOT NULL DEFAULT '',
    labels             JSONB NOT NULL DEFAULT '{}',
    phase              TEXT NOT NULL DEFAULT '',
    operator_version   TEXT NOT NULL DEFAULT '',
    last_heartbeat_at  TIMESTAMPTZ,
    reg_token_hash     TEXT NOT NULL DEFAULT '',
    auth_token_hash    TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at         TIMESTAMPTZ
);
CREATE UNIQUE INDEX clusters_name_key ON clusters (name) WHERE deleted_at IS NULL;

CREATE TABLE policies (
    id                BIGINT PRIMARY KEY,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    rego              TEXT NOT NULL DEFAULT '',
    enforcement_mode  TEXT NOT NULL DEFAULT '',
    match             JSONB NOT NULL DEFAULT '{}',
    compile_ok        BOOLEAN NOT NULL DEFAULT false,
    compile_error     TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX policies_name_key ON policies (name) WHERE deleted_at IS NULL;

CREATE TABLE policy_versions (
    id             BIGINT PRIMARY KEY,
    policy_id      BIGINT NOT NULL REFERENCES policies (id),
    version        INT NOT NULL,
    rego           TEXT NOT NULL DEFAULT '',
    spec_snapshot  JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ
);
CREATE INDEX policy_versions_policy_id_idx ON policy_versions (policy_id);
CREATE UNIQUE INDEX policy_versions_policy_version_key ON policy_versions (policy_id, version) WHERE deleted_at IS NULL;

CREATE TABLE policy_groups (
    id          BIGINT PRIMARY KEY,
    name        TEXT NOT NULL,
    spec        JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE UNIQUE INDEX policy_groups_name_key ON policy_groups (name) WHERE deleted_at IS NULL;

CREATE TABLE policy_exceptions (
    id          BIGINT PRIMARY KEY,
    name        TEXT NOT NULL,
    spec        JSONB NOT NULL DEFAULT '{}',
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE UNIQUE INDEX policy_exceptions_name_key ON policy_exceptions (name) WHERE deleted_at IS NULL;

CREATE TABLE policy_distributions (
    id          BIGINT PRIMARY KEY,
    name        TEXT NOT NULL,
    spec        JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE UNIQUE INDEX policy_distributions_name_key ON policy_distributions (name) WHERE deleted_at IS NULL;

CREATE TABLE distribution_cluster_status (
    id                    BIGINT PRIMARY KEY,
    distribution_id       BIGINT NOT NULL REFERENCES policy_distributions (id),
    cluster_id            BIGINT NOT NULL REFERENCES clusters (id),
    status                TEXT NOT NULL DEFAULT '',
    applied_version_hash  TEXT NOT NULL DEFAULT '',
    last_error            TEXT NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ
);
CREATE UNIQUE INDEX distribution_cluster_status_pair_key ON distribution_cluster_status (distribution_id, cluster_id) WHERE deleted_at IS NULL;
