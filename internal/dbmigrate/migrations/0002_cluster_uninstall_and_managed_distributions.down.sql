SET search_path TO kubesentry;

DROP INDEX IF EXISTS cluster_uninstall_tasks_active_cluster_key;
DROP INDEX IF EXISTS cluster_uninstall_tasks_cluster_created_idx;
DROP TABLE IF EXISTS cluster_uninstall_tasks;

DROP INDEX IF EXISTS policy_distributions_managed_owner_key;
ALTER TABLE policy_distributions
    DROP COLUMN IF EXISTS owner_name,
    DROP COLUMN IF EXISTS owner_type,
    DROP COLUMN IF EXISTS managed;
