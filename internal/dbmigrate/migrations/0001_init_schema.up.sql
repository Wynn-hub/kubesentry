CREATE SCHEMA IF NOT EXISTS kubesentry;
SET search_path TO kubesentry;

CREATE TABLE clusters (
    id                 BIGINT PRIMARY KEY,
    name               TEXT NOT NULL,
    cluster_uid        TEXT NOT NULL DEFAULT '',
    labels             JSONB NOT NULL DEFAULT '{}',
    phase              TEXT NOT NULL DEFAULT '',
    operator_version   TEXT NOT NULL DEFAULT '',
    -- Member-reported inventory, refreshed by StatusReport. Best-effort: a
    -- member that cannot read a value leaves it empty/zero.
    kubernetes_version TEXT NOT NULL DEFAULT '',
    api_server_address TEXT NOT NULL DEFAULT '',
    node_count         INT NOT NULL DEFAULT 0,
    ready_node_count   INT NOT NULL DEFAULT 0,
    last_heartbeat_at  TIMESTAMPTZ,
    observability_onboarded_at TIMESTAMPTZ,
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
    labels            JSONB NOT NULL DEFAULT '{}',
    annotations       JSONB NOT NULL DEFAULT '{}',
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
    labels      JSONB NOT NULL DEFAULT '{}',
    annotations JSONB NOT NULL DEFAULT '{}',
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
    labels      JSONB NOT NULL DEFAULT '{}',
    annotations JSONB NOT NULL DEFAULT '{}',
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

CREATE TABLE cluster_install_records (
    id                       BIGINT PRIMARY KEY,
    cluster_name             TEXT NOT NULL,
    namespace                TEXT NOT NULL DEFAULT '',
    webhook_repository       TEXT NOT NULL DEFAULT '',
    webhook_tag              TEXT NOT NULL DEFAULT '',
    webhook_pull_policy      TEXT NOT NULL DEFAULT '',
    operator_repository      TEXT NOT NULL DEFAULT '',
    operator_tag             TEXT NOT NULL DEFAULT '',
    operator_pull_policy     TEXT NOT NULL DEFAULT '',
    high_availability        BOOLEAN NOT NULL DEFAULT FALSE,
    gateway_address          TEXT NOT NULL DEFAULT '',
    gateway_tls_server_name  TEXT NOT NULL DEFAULT '',
    gateway_ca_bundle        TEXT NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at               TIMESTAMPTZ
);
CREATE INDEX cluster_install_records_cluster_name_idx ON cluster_install_records (cluster_name);

CREATE TABLE admission_event_settings (
    id                     BIGINT PRIMARY KEY,
    default_retention_days INT NOT NULL CHECK (default_retention_days BETWEEN 1 AND 365),
    retry_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    max_retry_attempts     INT NOT NULL DEFAULT 3 CHECK (max_retry_attempts BETWEEN 1 AND 100),
    queue_capacity         INT NOT NULL DEFAULT 1024 CHECK (queue_capacity BETWEEN 1 AND 10000),
    storage_policy_version BIGINT NOT NULL DEFAULT 1,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO admission_event_settings (id, default_retention_days, storage_policy_version)
VALUES (1, 14, 1);

-- Storage rules carry conditions only: matching any rule stores the event, an
-- empty rule set stores everything, and the retention period is the single
-- global admission_event_settings.default_retention_days. priority therefore no
-- longer affects the decision and only gives the console a stable row order.
CREATE TABLE admission_storage_policies (
    id             BIGINT PRIMARY KEY,
    priority       INT NOT NULL,
    match_mode     TEXT NOT NULL DEFAULT 'include' CHECK (match_mode IN ('include', 'exclude')),
    verdicts       TEXT[] NOT NULL DEFAULT '{}',
    clusters       TEXT[] NOT NULL DEFAULT '{}',
    gvks           TEXT[] NOT NULL DEFAULT '{}',
    namespaces     TEXT[] NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ
);
CREATE UNIQUE INDEX admission_storage_policies_priority_key ON admission_storage_policies (priority) WHERE deleted_at IS NULL;

CREATE TABLE admission_events (
    id                     BIGINT PRIMARY KEY,
    event_id               TEXT NOT NULL,
    request_uid            TEXT NOT NULL DEFAULT '',
    cluster_id             BIGINT NOT NULL REFERENCES clusters (id),
    cluster_name           TEXT NOT NULL,
    occurred_at            TIMESTAMPTZ NOT NULL,
    verdict                TEXT NOT NULL CHECK (verdict IN ('audit', 'deny')),
    api_group              TEXT NOT NULL DEFAULT '',
    api_version            TEXT NOT NULL DEFAULT '',
    resource               TEXT NOT NULL DEFAULT '',
    operation              TEXT NOT NULL DEFAULT '',
    namespace              TEXT NOT NULL DEFAULT '',
    resource_name          TEXT NOT NULL DEFAULT '',
    username               TEXT NOT NULL DEFAULT '',
    user_groups            TEXT[] NOT NULL DEFAULT '{}',
    policy_names           TEXT[] NOT NULL DEFAULT '{}',
    policy_group_names     TEXT[] NOT NULL DEFAULT '{}',
    violations             JSONB NOT NULL DEFAULT '[]',
    metadata_snapshot      JSONB NOT NULL DEFAULT '{}',
    spec_snapshot          JSONB NOT NULL DEFAULT '{}',
    storage_policy_version TEXT NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at             TIMESTAMPTZ
);
CREATE UNIQUE INDEX admission_events_event_id_key ON admission_events (event_id);
CREATE INDEX admission_events_created_at_idx ON admission_events (created_at);
CREATE INDEX admission_events_cluster_occurred_at_idx ON admission_events (cluster_id, occurred_at DESC);
CREATE INDEX admission_events_occurred_at_idx ON admission_events (occurred_at DESC);

CREATE TABLE platform_observability_settings (
    id                         BIGINT PRIMARY KEY,
    admission_pipeline_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    last_cleanup_at            TIMESTAMPTZ,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO platform_observability_settings (id, admission_pipeline_enabled)
VALUES (1, FALSE);

CREATE TABLE platform_component_samples (
    id                     BIGINT PRIMARY KEY,
    bucket_started_at      TIMESTAMPTZ NOT NULL,
    component              TEXT NOT NULL CHECK (component IN (
                               'webhook', 'operator', 'gateway',
                               'console-api', 'console-web', 'postgres')),
    instance               TEXT NOT NULL,
    cluster_id             BIGINT NOT NULL DEFAULT 0,
    cluster_name           TEXT NOT NULL DEFAULT '',
    role                   TEXT NOT NULL CHECK (role IN ('leader', 'standby', 'none')),
    process_started_at     TIMESTAMPTZ NOT NULL,
    restart_count          BIGINT NOT NULL DEFAULT 0,
    memory_bytes           BIGINT NOT NULL DEFAULT -1,
    memory_limit_bytes     BIGINT NOT NULL DEFAULT -1,
    cpu_usage_micros       BIGINT NOT NULL DEFAULT -1,
    cpu_quota_micros       BIGINT NOT NULL DEFAULT -1,
    goroutines             BIGINT NOT NULL DEFAULT -1,
    heap_alloc_bytes       BIGINT NOT NULL DEFAULT -1,
    heap_sys_bytes         BIGINT NOT NULL DEFAULT -1,
    gc_pause_total_micros  BIGINT NOT NULL DEFAULT -1,
    gc_count               BIGINT NOT NULL DEFAULT -1,
    health                 TEXT NOT NULL CHECK (health IN ('ok', 'degraded')),
    health_reason          TEXT NOT NULL DEFAULT '',
    db_stats               JSONB NOT NULL DEFAULT '{}',
    last_observed_at       TIMESTAMPTZ NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX platform_component_samples_key
    ON platform_component_samples (bucket_started_at, component, instance, cluster_id);
CREATE INDEX platform_component_samples_time_idx
    ON platform_component_samples (bucket_started_at DESC);
CREATE INDEX platform_component_samples_freshness_idx
    ON platform_component_samples (cluster_id, component, last_observed_at DESC);

CREATE TABLE platform_leadership_events (
    id            BIGINT PRIMARY KEY,
    component     TEXT NOT NULL CHECK (component IN ('operator', 'gateway')),
    cluster_id    BIGINT NOT NULL DEFAULT 0,
    cluster_name  TEXT NOT NULL DEFAULT '',
    instance      TEXT NOT NULL,
    role_from     TEXT NOT NULL CHECK (role_from IN ('leader', 'standby', 'none')),
    role_to       TEXT NOT NULL CHECK (role_to IN ('leader', 'standby', 'none')),
    changed_at    TIMESTAMPTZ NOT NULL,
    detected_by   TEXT NOT NULL CHECK (detected_by IN ('sample', 'session_lost')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX platform_leadership_events_component_idx
    ON platform_leadership_events (component, cluster_id, changed_at DESC);
CREATE INDEX platform_leadership_events_time_idx
    ON platform_leadership_events (changed_at);

CREATE FUNCTION jsonb_counter_add(left_counts JSONB, right_counts JSONB)
RETURNS JSONB
LANGUAGE SQL
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT COALESCE(jsonb_object_agg(counter_key, counter_value), '{}'::jsonb)
    FROM (
        SELECT counter_key, SUM(counter_value)::BIGINT AS counter_value
        FROM (
            SELECT key AS counter_key, value::TEXT::BIGINT AS counter_value
            FROM jsonb_each(COALESCE(left_counts, '{}'::jsonb))
            UNION ALL
            SELECT key AS counter_key, value::TEXT::BIGINT AS counter_value
            FROM jsonb_each(COALESCE(right_counts, '{}'::jsonb))
        ) values_to_add
        GROUP BY counter_key
    ) merged
$$;

CREATE TABLE admission_observation_buckets (
    id                         BIGINT PRIMARY KEY,
    bucket_started_at          TIMESTAMPTZ NOT NULL,
    cluster_id                 BIGINT NOT NULL DEFAULT 0,
    cluster_name               TEXT NOT NULL DEFAULT '',
    component                  TEXT NOT NULL CHECK (component IN ('webhook', 'operator', 'gateway', 'cleanup')),
    generated_count            BIGINT NOT NULL DEFAULT 0,
    received_count             BIGINT NOT NULL DEFAULT 0,
    forwarded_count            BIGINT NOT NULL DEFAULT 0,
    written_count              BIGINT NOT NULL DEFAULT 0,
    duplicate_count            BIGINT NOT NULL DEFAULT 0,
    policy_filtered_count      BIGINT NOT NULL DEFAULT 0,
    retention_elapsed_count    BIGINT NOT NULL DEFAULT 0,
    queue_rejected_count       BIGINT NOT NULL DEFAULT 0,
    retry_count                BIGINT NOT NULL DEFAULT 0,
    dropped_count              BIGINT NOT NULL DEFAULT 0,
    retry_exhausted_count      BIGINT NOT NULL DEFAULT 0,
    batch_failure_count        BIGINT NOT NULL DEFAULT 0,
    auth_failure_count         BIGINT NOT NULL DEFAULT 0,
    ack_timeout_count          BIGINT NOT NULL DEFAULT 0,
    report_dropped_count       BIGINT NOT NULL DEFAULT 0,
    observation_write_failed   BIGINT NOT NULL DEFAULT 0,
    sequence_gap_count         BIGINT NOT NULL DEFAULT 0,
    baseline_reset_count       BIGINT NOT NULL DEFAULT 0,
    series_started_count       BIGINT NOT NULL DEFAULT 0,
    cleanup_run_count          BIGINT NOT NULL DEFAULT 0,
    cleanup_deleted_count      BIGINT NOT NULL DEFAULT 0,
    cleanup_failure_count      BIGINT NOT NULL DEFAULT 0,
    queue_depth_max            BIGINT NOT NULL DEFAULT 0,
    queue_capacity             BIGINT NOT NULL DEFAULT 0,
    fault_reason_counts        JSONB NOT NULL DEFAULT '{}',
    pressure_reason_counts     JSONB NOT NULL DEFAULT '{}',
    filter_reason_counts       JSONB NOT NULL DEFAULT '{}',
    last_observed_at           TIMESTAMPTZ NOT NULL,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX admission_observation_buckets_key
    ON admission_observation_buckets (bucket_started_at, cluster_id, component);
CREATE INDEX admission_observation_buckets_time_idx
    ON admission_observation_buckets (bucket_started_at DESC);
CREATE INDEX admission_observation_buckets_cluster_time_idx
    ON admission_observation_buckets (cluster_id, bucket_started_at DESC);
CREATE INDEX admission_observation_buckets_last_observed_idx
    ON admission_observation_buckets (cluster_id, component, last_observed_at DESC);

CREATE TABLE observation_report_cursors (
    id               BIGINT PRIMARY KEY,
    cluster_id       BIGINT NOT NULL DEFAULT 0,
    series_id        TEXT NOT NULL,
    component        TEXT NOT NULL CHECK (component IN ('webhook', 'operator')),
    last_sequence    BIGINT NOT NULL,
    counter_snapshot JSONB NOT NULL DEFAULT '{}',
    last_active_at   TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX observation_report_cursors_key
    ON observation_report_cursors (cluster_id, series_id);
CREATE INDEX observation_report_cursors_idle_idx
    ON observation_report_cursors (last_active_at);

-- Seed data below: 37 built-in policies, the "default" PolicyGroup and
-- PolicyDistribution that reference them, their initial v1 PolicyVersion
-- snapshots, and the permanent system-namespace exception. All builtin rows
-- carry labels={"kubesentry.io/builtin":"true"} so the console can tell them
-- apart from user-created rows (delete protection, "builtin" badge) without a
-- dedicated source column.

INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083584, 'automount-service-account-token', 'Fails when automountServiceAccountToken is not explicitly set to false.', 'package kubesentry

deny[msg] {
  input.request.resource.resource == "pods"
  not input.request.object.spec.automountServiceAccountToken == false
  msg := "Pod should set spec.automountServiceAccountToken=false"
}
deny[msg] {
  input.request.resource.resource == "serviceaccounts"
  not input.request.object.automountServiceAccountToken == false
  msg := sprintf("ServiceAccount %q should set automountServiceAccountToken=false", [input.request.object.metadata.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods","serviceaccounts"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083585, 'clusterrole-pod-exec-attach', 'Fails when the ClusterRole allows pods/exec or pods/attach.', 'package kubesentry

_exec_attach_verbs := {"create"}
_exec_attach_resources := {"pods/exec", "pods/attach"}

deny[msg] {
  rule := input.request.object.rules[_]
  resource := rule.resources[_]
  _exec_attach_resources[resource]
  verb := rule.verbs[_]
  _exec_attach_verbs[verb]
  msg := sprintf("ClusterRole grants %q on %q", [verb, resource])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["clusterroles"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083586, 'clusterrolebinding-cluster-admin', 'Fails when the ClusterRoleBinding references the cluster-admin ClusterRole.', 'package kubesentry

deny[msg] {
  input.request.object.roleRef.name == "cluster-admin"
  msg := "ClusterRoleBinding references the cluster-admin ClusterRole"
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["clusterrolebindings"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083587, 'clusterrolebinding-pod-exec-attach', 'Fails when the ClusterRoleBinding references a ClusterRole known to allow pods/exec or pods/attach.', 'package kubesentry

_known_exec_roles := {"pod-exec", "pod-attach", "exec-pod", "attach-pod"}

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "ClusterRole"
  role := _known_exec_roles[_]
  contains(lower(ref.name), role)
  msg := sprintf("ClusterRoleBinding references potentially dangerous ClusterRole %q", [ref.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["clusterrolebindings"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083588, 'cpu-limits-missing', 'Fails when resources.limits.cpu is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.limits.cpu
  msg := sprintf("container %q must set resources.limits.cpu", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083589, 'cpu-requests-missing', 'Fails when resources.requests.cpu is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.requests.cpu
  msg := sprintf("container %q must set resources.requests.cpu", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083590, 'dangerous-capabilities', 'Fails when securityContext.capabilities includes dangerous capabilities.', 'package kubesentry

_dangerous := {
  "NET_ADMIN", "SYS_ADMIN", "SYS_PTRACE", "SYS_MODULE",
  "SYS_RAWIO", "DAC_READ_SEARCH", "NET_RAW", "SYS_RESOURCE",
  "SYS_PACCT", "SYS_NICE", "WAKE_ALARM",
}

deny[msg] {
  c := input.request.object.spec.containers[_]
  cap := c.securityContext.capabilities.add[_]
  _dangerous[cap]
  msg := sprintf("container %q adds dangerous capability %q", [c.name, cap])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083591, 'deployment-missing-replicas', 'Fails when a Deployment has only one replica.', 'package kubesentry

deny[msg] {
  replicas := input.request.object.spec.replicas
  replicas < 2
  msg := sprintf("Deployment has only %d replica(s); set replicas >= 2 for availability", [replicas])
}
deny[msg] {
  not input.request.object.spec.replicas
  msg := "Deployment does not specify replicas; default of 1 is insufficient for availability"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["apps"],"apiVersions":["v1"],"resources":["deployments"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083592, 'host-ipc-set', 'Fails when hostIPC attribute is configured.', 'package kubesentry

deny[msg] {
  input.request.object.spec.hostIPC == true
  msg := "hostIPC must not be set"
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083593, 'host-network-set', 'Fails when hostNetwork attribute is configured.', 'package kubesentry

deny[msg] {
  input.request.object.spec.hostNetwork == true
  msg := "hostNetwork must not be set"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083594, 'host-pid-set', 'Fails when hostPID attribute is configured.', 'package kubesentry

deny[msg] {
  input.request.object.spec.hostPID == true
  msg := "hostPID must not be set"
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083595, 'host-port-set', 'Fails when hostPort attribute is configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  port := c.ports[_]
  port.hostPort > 0
  msg := sprintf("container %q must not set hostPort %d", [c.name, port.hostPort])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083596, 'hpa-max-availability', 'Fails when maxReplicas is less than or equal to minReplicas.', 'package kubesentry

deny[msg] {
  min := input.request.object.spec.minReplicas
  max := input.request.object.spec.maxReplicas
  max <= min
  msg := sprintf("HPA maxReplicas (%d) must be greater than minReplicas (%d)", [max, min])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["autoscaling"],"apiVersions":["v2"],"resources":["horizontalpodautoscalers"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083597, 'hpa-min-availability', 'Fails when minReplicas is less than or equal to 1.', 'package kubesentry

deny[msg] {
  min := input.request.object.spec.minReplicas
  min <= 1
  msg := sprintf("HPA minReplicas (%d) should be greater than 1", [min])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["autoscaling"],"apiVersions":["v2"],"resources":["horizontalpodautoscalers"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083598, 'insecure-capabilities', 'Fails when securityContext.capabilities includes insecure capabilities.', 'package kubesentry

_insecure := {
  "NET_BIND_SERVICE", "CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID",
  "KILL", "SETGID", "SETUID", "SETPCAP", "SETFCAP",
  "NET_BROADCAST", "IPC_LOCK", "IPC_OWNER",
  "SYS_BOOT", "SYS_NICE", "SYS_TIME", "SYS_TTY_CONFIG",
  "MKNOD", "AUDIT_WRITE", "AUDIT_CONTROL", "SYSLOG", "LINUX_IMMUTABLE",
}

deny[msg] {
  c := input.request.object.spec.containers[_]
  cap := c.securityContext.capabilities.add[_]
  _insecure[cap]
  msg := sprintf("container %q adds insecure capability %q", [c.name, cap])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083599, 'linux-hardening', 'Fails when neither Seccomp, SELinux, nor capabilities.drop is in use.', 'package kubesentry

_has_seccomp(c) {
  c.securityContext.seccompProfile.type
}
_has_selinux(c) {
  c.securityContext.seLinuxOptions.type
}
_has_drop_caps(c) {
  c.securityContext.capabilities.drop[_]
}

deny[msg] {
  c := input.request.object.spec.containers[_]
  not _has_seccomp(c)
  not _has_selinux(c)
  not _has_drop_caps(c)
  msg := sprintf("container %q has no Linux hardening (set seccompProfile, seLinuxOptions, or capabilities.drop)", [c.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083600, 'liveness-probe-missing', 'Fails when a liveness probe is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.livenessProbe
  msg := sprintf("container %q must define a livenessProbe", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083601, 'memory-limits-missing', 'Fails when resources.limits.memory is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.limits.memory
  msg := sprintf("container %q must set resources.limits.memory", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083602, 'memory-requests-missing', 'Fails when resources.requests.memory is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.requests.memory
  msg := sprintf("container %q must set resources.requests.memory", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083603, 'metadata-and-instance-mismatched', 'Fails when label app.kubernetes.io/instance and metadata.name mismatch.', 'package kubesentry

deny[msg] {
  name := input.request.object.metadata.name
  instance := input.request.object.metadata.labels["app.kubernetes.io/instance"]
  name != instance
  msg := sprintf("metadata.name %q does not match app.kubernetes.io/instance label %q", [name, instance])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["apps"],"apiVersions":["v1"],"resources":["deployments","statefulsets","daemonsets"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083604, 'not-read-only-root-filesystem', 'Fails when securityContext.readOnlyRootFilesystem is not true.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.securityContext.readOnlyRootFilesystem == true
  msg := sprintf("container %q should set securityContext.readOnlyRootFilesystem=true", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083605, 'priority-class-not-set', 'Fails when priorityClassName is not set.', 'package kubesentry

deny[msg] {
  not input.request.object.spec.priorityClassName
  msg := "priorityClassName should be set"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083606, 'privilege-escalation-allowed', 'Fails when securityContext.allowPrivilegeEscalation is true.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  c.securityContext.allowPrivilegeEscalation == true
  msg := sprintf("container %q must not allow privilege escalation", [c.name])
}
deny[msg] {
  c := input.request.object.spec.initContainers[_]
  c.securityContext.allowPrivilegeEscalation == true
  msg := sprintf("initContainer %q must not allow privilege escalation", [c.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083607, 'pull-policy-not-always', 'Fails when imagePullPolicy is not Always.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  c.imagePullPolicy != "Always"
  msg := sprintf("container %q imagePullPolicy should be Always, got %q", [c.name, c.imagePullPolicy])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083608, 'readiness-probe-missing', 'Fails when a readiness probe is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.readinessProbe
  msg := sprintf("container %q must define a readinessProbe", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083609, 'role-pod-exec-attach', 'Fails when the Role allows pods/exec or pods/attach.', 'package kubesentry

_exec_attach_verbs := {"create"}
_exec_attach_resources := {"pods/exec", "pods/attach"}

deny[msg] {
  rule := input.request.object.rules[_]
  resource := rule.resources[_]
  _exec_attach_resources[resource]
  verb := rule.verbs[_]
  _exec_attach_verbs[verb]
  msg := sprintf("Role grants %q on %q", [verb, resource])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["roles"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083610, 'rolebinding-cluster-admin-cluster-role', 'Fails when the RoleBinding references the cluster-admin ClusterRole.', 'package kubesentry

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "ClusterRole"
  ref.name == "cluster-admin"
  msg := "RoleBinding references the cluster-admin ClusterRole"
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["rolebindings"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083611, 'rolebinding-cluster-admin-role', 'Fails when the RoleBinding references a Role with wildcard permissions.', 'package kubesentry

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "Role"
  lower(ref.name) == "cluster-admin"
  msg := sprintf("RoleBinding references a Role named %q which may grant cluster-admin permissions", [ref.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["rolebindings"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083612, 'rolebinding-cluster-role-pod-exec-attach', 'Fails when the RoleBinding references a ClusterRole known to allow pods/exec or pods/attach.', 'package kubesentry

_known_exec_roles := {"pod-exec", "pod-attach", "exec-pod", "attach-pod"}

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "ClusterRole"
  role := _known_exec_roles[_]
  contains(lower(ref.name), role)
  msg := sprintf("RoleBinding references potentially dangerous ClusterRole %q", [ref.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["rolebindings"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083613, 'rolebinding-role-pod-exec-attach', 'Fails when the RoleBinding references a Role known to allow pods/exec or pods/attach.', 'package kubesentry

_known_exec_roles := {"pod-exec", "pod-attach", "exec-pod", "attach-pod"}

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "Role"
  role := _known_exec_roles[_]
  contains(lower(ref.name), role)
  msg := sprintf("RoleBinding references potentially dangerous Role %q", [ref.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["rolebindings"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083614, 'run-as-privileged', 'Fails when securityContext.privileged is true.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  c.securityContext.privileged == true
  msg := sprintf("container %q must not run as privileged", [c.name])
}
deny[msg] {
  c := input.request.object.spec.initContainers[_]
  c.securityContext.privileged == true
  msg := sprintf("initContainer %q must not run as privileged", [c.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083615, 'run-as-root-allowed', 'Fails when securityContext.runAsNonRoot is not true.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.securityContext.runAsNonRoot == true
  msg := sprintf("container %q should set securityContext.runAsNonRoot=true", [c.name])
}
deny[msg] {
  c := input.request.object.spec.initContainers[_]
  not c.securityContext.runAsNonRoot == true
  msg := sprintf("initContainer %q should set securityContext.runAsNonRoot=true", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083616, 'sensitive-configmap-content', 'Fails when potentially sensitive content is detected in ConfigMap keys or values.', 'package kubesentry

_sensitive_keys := {
  "password", "passwd", "secret", "token", "api_key", "apikey",
  "private_key", "access_key", "credentials", "credential",
}

deny[msg] {
  key := input.request.object.data[k]
  k_lower := lower(k)
  pattern := _sensitive_keys[_]
  contains(k_lower, pattern)
  msg := sprintf("ConfigMap key %q may contain sensitive data", [k])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["configmaps"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083617, 'sensitive-container-env-var', 'Fails when the container sets potentially sensitive environment variables.', 'package kubesentry

_sensitive_patterns := {
  "password", "passwd", "secret", "token", "key",
  "api_key", "apikey", "auth", "credential", "credentials",
  "private_key", "access_key", "private_token",
}

deny[msg] {
  c := input.request.object.spec.containers[_]
  env := c.env[_]
  name_lower := lower(env.name)
  pattern := _sensitive_patterns[_]
  contains(name_lower, pattern)
  msg := sprintf("container %q has potentially sensitive env var %q", [c.name, env.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083618, 'tag-not-specified', 'Fails when an image tag is not specified or is latest.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  image := c.image
  not contains(image, ":")
  msg := sprintf("container %q image %q has no tag", [c.name, image])
}
deny[msg] {
  c := input.request.object.spec.containers[_]
  image := c.image
  endswith(image, ":latest")
  msg := sprintf("container %q image %q uses the latest tag", [c.name, image])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083619, 'tls-settings-missing', 'Fails when an Ingress lacks TLS settings.', 'package kubesentry

deny[msg] {
  count(input.request.object.spec.tls) == 0
  msg := "Ingress should configure TLS settings"
}

deny[msg] {
  not input.request.object.spec.tls
  msg := "Ingress should configure TLS settings"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"resources":["ingresses"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok, labels) VALUES (2077920678262083620, 'topology-spread-constraint', 'Fails when no topology spread constraint is defined.', 'package kubesentry

deny[msg] {
  not input.request.object.spec.topologySpreadConstraints
  msg := "no topologySpreadConstraints defined"
}
deny[msg] {
  count(input.request.object.spec.topologySpreadConstraints) == 0
  msg := "topologySpreadConstraints is empty"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true, '{"kubesentry.io/builtin": "true"}');

INSERT INTO policy_groups (id, name, spec, labels) VALUES (2077920678262083621, 'default', '{"displayName":"Default","enabled":true,"policies":{"byName":[{"name":"automount-service-account-token","enforcementMode":"audit"},{"name":"clusterrole-pod-exec-attach","enforcementMode":"enforce"},{"name":"clusterrolebinding-cluster-admin","enforcementMode":"enforce"},{"name":"clusterrolebinding-pod-exec-attach","enforcementMode":"enforce"},{"name":"cpu-limits-missing","enforcementMode":"audit"},{"name":"cpu-requests-missing","enforcementMode":"audit"},{"name":"dangerous-capabilities","enforcementMode":"enforce"},{"name":"deployment-missing-replicas","enforcementMode":"audit"},{"name":"host-ipc-set","enforcementMode":"enforce"},{"name":"host-network-set","enforcementMode":"audit"},{"name":"host-pid-set","enforcementMode":"enforce"},{"name":"host-port-set","enforcementMode":"audit"},{"name":"hpa-max-availability","enforcementMode":"audit"},{"name":"hpa-min-availability","enforcementMode":"audit"},{"name":"insecure-capabilities","enforcementMode":"audit"},{"name":"linux-hardening","enforcementMode":"enforce"},{"name":"liveness-probe-missing","enforcementMode":"audit"},{"name":"memory-limits-missing","enforcementMode":"audit"},{"name":"memory-requests-missing","enforcementMode":"audit"},{"name":"metadata-and-instance-mismatched","enforcementMode":"audit"},{"name":"not-read-only-root-filesystem","enforcementMode":"audit"},{"name":"priority-class-not-set","enforcementMode":"audit"},{"name":"privilege-escalation-allowed","enforcementMode":"enforce"},{"name":"pull-policy-not-always","enforcementMode":"audit"},{"name":"readiness-probe-missing","enforcementMode":"audit"},{"name":"role-pod-exec-attach","enforcementMode":"enforce"},{"name":"rolebinding-cluster-admin-cluster-role","enforcementMode":"enforce"},{"name":"rolebinding-cluster-admin-role","enforcementMode":"enforce"},{"name":"rolebinding-cluster-role-pod-exec-attach","enforcementMode":"enforce"},{"name":"rolebinding-role-pod-exec-attach","enforcementMode":"enforce"},{"name":"run-as-privileged","enforcementMode":"enforce"},{"name":"run-as-root-allowed","enforcementMode":"audit"},{"name":"sensitive-configmap-content","enforcementMode":"enforce"},{"name":"sensitive-container-env-var","enforcementMode":"enforce"},{"name":"tag-not-specified","enforcementMode":"enforce"},{"name":"tls-settings-missing","enforcementMode":"audit"},{"name":"topology-spread-constraint","enforcementMode":"audit"}]}}'::jsonb, '{"kubesentry.io/builtin": "true"}');
INSERT INTO policy_distributions (id, name, spec) VALUES (2077920678262083622, 'default', '{"clusters":{"clusterSelector":{}},"policyGroups":["default"]}'::jsonb);
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

-- Permanent system-namespace exemption, formerly rendered by the
-- kubesentry-gateway chart's templates/default-exception.yaml PolicyException
-- CR (spec D9/D17). now() + interval is used in place of the old Go
-- "876000h" duration literal to express the same ~100-year effectively-forever
-- window.
INSERT INTO policy_exceptions (id, name, spec, expires_at, labels) VALUES (2078506982033063936, 'default-system-namespaces', '{"allPolicies":true,"match":{"namespaces":["kube-system","kube-public","kube-node-lease","kubesentry-system"]},"reason":"System namespaces are excluded from policy enforcement by default"}'::jsonb, now() + interval '100 years', '{"kubesentry.io/builtin": "true"}');

ALTER TABLE policy_distributions
    ADD COLUMN managed BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN owner_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN owner_name TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX policy_distributions_managed_owner_key
    ON policy_distributions (owner_type, owner_name)
    WHERE managed AND deleted_at IS NULL;
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
