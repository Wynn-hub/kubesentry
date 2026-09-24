SET search_path TO kubesentry;

DROP INDEX IF EXISTS policy_distributions_owner_name_scope_key;
ALTER TABLE policy_distributions DROP COLUMN tenant_id, DROP COLUMN resource_owner_type;
CREATE UNIQUE INDEX policy_distributions_name_key ON policy_distributions (name) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS policy_exceptions_owner_name_key;
ALTER TABLE policy_exceptions DROP COLUMN tenant_id, DROP COLUMN owner_type;
CREATE UNIQUE INDEX policy_exceptions_name_key ON policy_exceptions (name) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS policy_groups_owner_name_key;
ALTER TABLE policy_groups DROP COLUMN tenant_id, DROP COLUMN owner_type;
CREATE UNIQUE INDEX policy_groups_name_key ON policy_groups (name) WHERE deleted_at IS NULL;

ALTER TABLE policies DROP COLUMN tenant_id, DROP COLUMN owner_type;
ALTER TABLE clusters DROP COLUMN tenant_id;
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE tenants;
