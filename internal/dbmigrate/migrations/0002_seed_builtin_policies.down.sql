SET search_path TO kubesentry;
DELETE FROM policy_distributions WHERE name = 'default';
DELETE FROM policy_groups WHERE name = 'default';
DELETE FROM policies WHERE name IN (
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
