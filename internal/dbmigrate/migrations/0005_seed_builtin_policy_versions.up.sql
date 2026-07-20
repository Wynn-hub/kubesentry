SET search_path TO kubesentry;

-- Backfills the v1 PolicyVersion snapshot that migration 0002 forgot to
-- create for the 37 seeded built-in policies. Every console-created/updated
-- policy gets a paired policy_versions row (see createPolicy/updatePolicy in
-- internal/console/handlers_policy.go); without this, a built-in policy's
-- first console edit is mislabeled "version 1" and rollback has nothing
-- below it to restore. The spec_snapshot shape mirrors policySpecSnapshot's
-- JSON output (match/enforcementMode/rego/description) exactly, so it
-- unmarshals into v1alpha1.PolicySpec identically to a snapshot the console
-- would have written itself. Fresh ids are synthesized here (row_number
-- offset) rather than hand-copied from 0002's INSERTs, since policy_id and
-- content are looked up by name instead.
INSERT INTO policy_versions (id, policy_id, version, rego, spec_snapshot)
SELECT
    2079500000000000000 + row_number() OVER (ORDER BY id),
    id,
    1,
    rego,
    jsonb_build_object(
        'match', match,
        'enforcementMode', enforcement_mode,
        'rego', rego,
        'description', description
    )
FROM policies
WHERE name IN (
    'automount-service-account-token', 'clusterrole-pod-exec-attach',
    'clusterrolebinding-cluster-admin', 'clusterrolebinding-pod-exec-attach',
    'cpu-limits-missing', 'cpu-requests-missing', 'dangerous-capabilities',
    'deployment-missing-replicas', 'host-ipc-set', 'host-network-set',
    'host-pid-set', 'host-port-set', 'hpa-max-availability', 'hpa-min-availability',
    'insecure-capabilities', 'linux-hardening', 'liveness-probe-missing',
    'memory-limits-missing', 'memory-requests-missing',
    'metadata-and-instance-mismatched', 'not-read-only-root-filesystem',
    'priority-class-not-set', 'privilege-escalation-allowed', 'pull-policy-not-always',
    'readiness-probe-missing', 'role-pod-exec-attach',
    'rolebinding-cluster-admin-cluster-role', 'rolebinding-cluster-admin-role',
    'rolebinding-cluster-role-pod-exec-attach', 'rolebinding-role-pod-exec-attach',
    'run-as-privileged', 'run-as-root-allowed', 'sensitive-configmap-content',
    'sensitive-container-env-var', 'tag-not-specified', 'tls-settings-missing',
    'topology-spread-constraint'
);
