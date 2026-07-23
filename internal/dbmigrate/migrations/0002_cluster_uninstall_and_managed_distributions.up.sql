SET search_path TO kubesentry;

ALTER TABLE policy_distributions
    ADD COLUMN managed BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN owner_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN owner_name TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX policy_distributions_managed_owner_key
    ON policy_distributions (owner_type, owner_name)
    WHERE managed AND deleted_at IS NULL;

-- The seed baseline is the one existing distribution whose ownership is
-- unambiguous. Its selector remains the all-clusters selector from 0001.
UPDATE policy_distributions
SET managed = true, owner_type = 'PolicyGroup', owner_name = 'default'
WHERE name = 'default' AND deleted_at IS NULL;

CREATE TABLE cluster_uninstall_tasks (
    id           BIGINT PRIMARY KEY,
    cluster_id   BIGINT NOT NULL REFERENCES clusters (id),
    status       TEXT NOT NULL DEFAULT 'Pending',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    received_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT NOT NULL DEFAULT '',
    attempts     INT NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX cluster_uninstall_tasks_cluster_created_idx
    ON cluster_uninstall_tasks (cluster_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX cluster_uninstall_tasks_active_cluster_key
    ON cluster_uninstall_tasks (cluster_id)
    WHERE deleted_at IS NULL AND status IN ('Pending', 'Received', 'Running');
