SET search_path TO kubesentry;

INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083584, 'automount-service-account-token', 'Fails when automountServiceAccountToken is not explicitly set to false.', 'package kubesentry

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
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods","serviceaccounts"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083585, 'clusterrole-pod-exec-attach', 'Fails when the ClusterRole allows pods/exec or pods/attach.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["clusterroles"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083586, 'clusterrolebinding-cluster-admin', 'Fails when the ClusterRoleBinding references the cluster-admin ClusterRole.', 'package kubesentry

deny[msg] {
  input.request.object.roleRef.name == "cluster-admin"
  msg := "ClusterRoleBinding references the cluster-admin ClusterRole"
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["clusterrolebindings"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083587, 'clusterrolebinding-pod-exec-attach', 'Fails when the ClusterRoleBinding references a ClusterRole known to allow pods/exec or pods/attach.', 'package kubesentry

_known_exec_roles := {"pod-exec", "pod-attach", "exec-pod", "attach-pod"}

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "ClusterRole"
  role := _known_exec_roles[_]
  contains(lower(ref.name), role)
  msg := sprintf("ClusterRoleBinding references potentially dangerous ClusterRole %q", [ref.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["clusterrolebindings"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083588, 'cpu-limits-missing', 'Fails when resources.limits.cpu is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.limits.cpu
  msg := sprintf("container %q must set resources.limits.cpu", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083589, 'cpu-requests-missing', 'Fails when resources.requests.cpu is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.requests.cpu
  msg := sprintf("container %q must set resources.requests.cpu", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083590, 'dangerous-capabilities', 'Fails when securityContext.capabilities includes dangerous capabilities.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083591, 'deployment-missing-replicas', 'Fails when a Deployment has only one replica.', 'package kubesentry

deny[msg] {
  replicas := input.request.object.spec.replicas
  replicas < 2
  msg := sprintf("Deployment has only %d replica(s); set replicas >= 2 for availability", [replicas])
}
deny[msg] {
  not input.request.object.spec.replicas
  msg := "Deployment does not specify replicas; default of 1 is insufficient for availability"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["apps"],"apiVersions":["v1"],"resources":["deployments"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083592, 'host-ipc-set', 'Fails when hostIPC attribute is configured.', 'package kubesentry

deny[msg] {
  input.request.object.spec.hostIPC == true
  msg := "hostIPC must not be set"
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083593, 'host-network-set', 'Fails when hostNetwork attribute is configured.', 'package kubesentry

deny[msg] {
  input.request.object.spec.hostNetwork == true
  msg := "hostNetwork must not be set"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083594, 'host-pid-set', 'Fails when hostPID attribute is configured.', 'package kubesentry

deny[msg] {
  input.request.object.spec.hostPID == true
  msg := "hostPID must not be set"
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083595, 'host-port-set', 'Fails when hostPort attribute is configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  port := c.ports[_]
  port.hostPort > 0
  msg := sprintf("container %q must not set hostPort %d", [c.name, port.hostPort])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083596, 'hpa-max-availability', 'Fails when maxReplicas is less than or equal to minReplicas.', 'package kubesentry

deny[msg] {
  min := input.request.object.spec.minReplicas
  max := input.request.object.spec.maxReplicas
  max <= min
  msg := sprintf("HPA maxReplicas (%d) must be greater than minReplicas (%d)", [max, min])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["autoscaling"],"apiVersions":["v2"],"resources":["horizontalpodautoscalers"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083597, 'hpa-min-availability', 'Fails when minReplicas is less than or equal to 1.', 'package kubesentry

deny[msg] {
  min := input.request.object.spec.minReplicas
  min <= 1
  msg := sprintf("HPA minReplicas (%d) should be greater than 1", [min])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["autoscaling"],"apiVersions":["v2"],"resources":["horizontalpodautoscalers"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083598, 'insecure-capabilities', 'Fails when securityContext.capabilities includes insecure capabilities.', 'package kubesentry

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
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083599, 'linux-hardening', 'Fails when neither Seccomp, SELinux, nor capabilities.drop is in use.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083600, 'liveness-probe-missing', 'Fails when a liveness probe is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.livenessProbe
  msg := sprintf("container %q must define a livenessProbe", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083601, 'memory-limits-missing', 'Fails when resources.limits.memory is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.limits.memory
  msg := sprintf("container %q must set resources.limits.memory", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083602, 'memory-requests-missing', 'Fails when resources.requests.memory is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.requests.memory
  msg := sprintf("container %q must set resources.requests.memory", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083603, 'metadata-and-instance-mismatched', 'Fails when label app.kubernetes.io/instance and metadata.name mismatch.', 'package kubesentry

deny[msg] {
  name := input.request.object.metadata.name
  instance := input.request.object.metadata.labels["app.kubernetes.io/instance"]
  name != instance
  msg := sprintf("metadata.name %q does not match app.kubernetes.io/instance label %q", [name, instance])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["apps"],"apiVersions":["v1"],"resources":["deployments","statefulsets","daemonsets"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083604, 'not-read-only-root-filesystem', 'Fails when securityContext.readOnlyRootFilesystem is not true.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.securityContext.readOnlyRootFilesystem == true
  msg := sprintf("container %q should set securityContext.readOnlyRootFilesystem=true", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083605, 'priority-class-not-set', 'Fails when priorityClassName is not set.', 'package kubesentry

deny[msg] {
  not input.request.object.spec.priorityClassName
  msg := "priorityClassName should be set"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083606, 'privilege-escalation-allowed', 'Fails when securityContext.allowPrivilegeEscalation is true.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083607, 'pull-policy-not-always', 'Fails when imagePullPolicy is not Always.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  c.imagePullPolicy != "Always"
  msg := sprintf("container %q imagePullPolicy should be Always, got %q", [c.name, c.imagePullPolicy])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083608, 'readiness-probe-missing', 'Fails when a readiness probe is not configured.', 'package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.readinessProbe
  msg := sprintf("container %q must define a readinessProbe", [c.name])
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083609, 'role-pod-exec-attach', 'Fails when the Role allows pods/exec or pods/attach.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["roles"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083610, 'rolebinding-cluster-admin-cluster-role', 'Fails when the RoleBinding references the cluster-admin ClusterRole.', 'package kubesentry

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "ClusterRole"
  ref.name == "cluster-admin"
  msg := "RoleBinding references the cluster-admin ClusterRole"
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["rolebindings"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083611, 'rolebinding-cluster-admin-role', 'Fails when the RoleBinding references a Role with wildcard permissions.', 'package kubesentry

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "Role"
  lower(ref.name) == "cluster-admin"
  msg := sprintf("RoleBinding references a Role named %q which may grant cluster-admin permissions", [ref.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["rolebindings"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083612, 'rolebinding-cluster-role-pod-exec-attach', 'Fails when the RoleBinding references a ClusterRole known to allow pods/exec or pods/attach.', 'package kubesentry

_known_exec_roles := {"pod-exec", "pod-attach", "exec-pod", "attach-pod"}

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "ClusterRole"
  role := _known_exec_roles[_]
  contains(lower(ref.name), role)
  msg := sprintf("RoleBinding references potentially dangerous ClusterRole %q", [ref.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["rolebindings"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083613, 'rolebinding-role-pod-exec-attach', 'Fails when the RoleBinding references a Role known to allow pods/exec or pods/attach.', 'package kubesentry

_known_exec_roles := {"pod-exec", "pod-attach", "exec-pod", "attach-pod"}

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "Role"
  role := _known_exec_roles[_]
  contains(lower(ref.name), role)
  msg := sprintf("RoleBinding references potentially dangerous Role %q", [ref.name])
}
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["rbac.authorization.k8s.io"],"apiVersions":["v1"],"resources":["rolebindings"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083614, 'run-as-privileged', 'Fails when securityContext.privileged is true.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083615, 'run-as-root-allowed', 'Fails when securityContext.runAsNonRoot is not true.', 'package kubesentry

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
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083616, 'sensitive-configmap-content', 'Fails when potentially sensitive content is detected in ConfigMap keys or values.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["configmaps"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083617, 'sensitive-container-env-var', 'Fails when the container sets potentially sensitive environment variables.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083618, 'tag-not-specified', 'Fails when an image tag is not specified or is latest.', 'package kubesentry

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
', 'enforce', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083619, 'tls-settings-missing', 'Fails when an Ingress lacks TLS settings.', 'package kubesentry

deny[msg] {
  count(input.request.object.spec.tls) == 0
  msg := "Ingress should configure TLS settings"
}

deny[msg] {
  not input.request.object.spec.tls
  msg := "Ingress should configure TLS settings"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"resources":["ingresses"]}]}'::jsonb, true);
INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (2077920678262083620, 'topology-spread-constraint', 'Fails when no topology spread constraint is defined.', 'package kubesentry

deny[msg] {
  not input.request.object.spec.topologySpreadConstraints
  msg := "no topologySpreadConstraints defined"
}
deny[msg] {
  count(input.request.object.spec.topologySpreadConstraints) == 0
  msg := "topologySpreadConstraints is empty"
}
', 'audit', '{"operations":["CREATE","UPDATE"],"resources":[{"apiGroups":[""],"apiVersions":["v1"],"resources":["pods"]}]}'::jsonb, true);

INSERT INTO policy_groups (id, name, spec) VALUES (2077920678262083621, 'default', '{"displayName":"Default","enabled":true,"policies":{"byName":[{"name":"automount-service-account-token","enforcementMode":"audit"},{"name":"clusterrole-pod-exec-attach","enforcementMode":"enforce"},{"name":"clusterrolebinding-cluster-admin","enforcementMode":"enforce"},{"name":"clusterrolebinding-pod-exec-attach","enforcementMode":"enforce"},{"name":"cpu-limits-missing","enforcementMode":"audit"},{"name":"cpu-requests-missing","enforcementMode":"audit"},{"name":"dangerous-capabilities","enforcementMode":"enforce"},{"name":"deployment-missing-replicas","enforcementMode":"audit"},{"name":"host-ipc-set","enforcementMode":"enforce"},{"name":"host-network-set","enforcementMode":"audit"},{"name":"host-pid-set","enforcementMode":"enforce"},{"name":"host-port-set","enforcementMode":"audit"},{"name":"hpa-max-availability","enforcementMode":"audit"},{"name":"hpa-min-availability","enforcementMode":"audit"},{"name":"insecure-capabilities","enforcementMode":"audit"},{"name":"linux-hardening","enforcementMode":"enforce"},{"name":"liveness-probe-missing","enforcementMode":"audit"},{"name":"memory-limits-missing","enforcementMode":"audit"},{"name":"memory-requests-missing","enforcementMode":"audit"},{"name":"metadata-and-instance-mismatched","enforcementMode":"audit"},{"name":"not-read-only-root-filesystem","enforcementMode":"audit"},{"name":"priority-class-not-set","enforcementMode":"audit"},{"name":"privilege-escalation-allowed","enforcementMode":"enforce"},{"name":"pull-policy-not-always","enforcementMode":"audit"},{"name":"readiness-probe-missing","enforcementMode":"audit"},{"name":"role-pod-exec-attach","enforcementMode":"enforce"},{"name":"rolebinding-cluster-admin-cluster-role","enforcementMode":"enforce"},{"name":"rolebinding-cluster-admin-role","enforcementMode":"enforce"},{"name":"rolebinding-cluster-role-pod-exec-attach","enforcementMode":"enforce"},{"name":"rolebinding-role-pod-exec-attach","enforcementMode":"enforce"},{"name":"run-as-privileged","enforcementMode":"enforce"},{"name":"run-as-root-allowed","enforcementMode":"audit"},{"name":"sensitive-configmap-content","enforcementMode":"enforce"},{"name":"sensitive-container-env-var","enforcementMode":"enforce"},{"name":"tag-not-specified","enforcementMode":"enforce"},{"name":"tls-settings-missing","enforcementMode":"audit"},{"name":"topology-spread-constraint","enforcementMode":"audit"}]}}'::jsonb);
INSERT INTO policy_distributions (id, name, spec) VALUES (2077920678262083622, 'default', '{"clusters":{"clusterSelector":{}},"policyGroups":["default"]}'::jsonb);
