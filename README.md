# KubeSentry

English | [中文](README_zh.md)

[![Go Version](https://img.shields.io/badge/go-1.26+-blue)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-green)](LICENSE)
[![Docker Hub](https://img.shields.io/docker/pulls/wynnhub/kubesentry-webhook?label=webhook%20pulls)](https://hub.docker.com/r/wynnhub/kubesentry-webhook)
[![Docker Hub](https://img.shields.io/docker/pulls/wynnhub/kubesentry-operator?label=operator%20pulls)](https://hub.docker.com/r/wynnhub/kubesentry-operator)

A Kubernetes Validating Admission Webhook that enforces OPA/Rego policies defined as CRDs, with an Operator for lifecycle management, version control, and built-in policy groups.

## Quick Start

```bash
helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --namespace kubesentry-system \
  --create-namespace
```

No login required — images and chart are publicly available on Docker Hub. The Helm pre-install Job auto-generates TLS certificates and patches the `ValidatingWebhookConfiguration`.

## Architecture

Two independent Go binaries share the same CRD API types:

```
┌────────────────────────────────────────────────────────────────┐
│                       Kubernetes Cluster                        │
│                                                                 │
│  ┌──────────────────┐      ┌────────────────────────────────┐  │
│  │  kubesentry-     │      │     kubesentry-operator        │  │
│  │  webhook         │      │                                │  │
│  │                  │      │  PolicyGroupReconciler         │  │
│  │  - OPA evaluator │      │  - creates child Policy CRDs  │  │
│  │  - Policy cache  │      │  - built-in library (37 rules) │  │
│  │  - /validate     │      │  - conflict detection          │  │
│  │  - /healthz      │      │                                │  │
│  │  - /readyz       │      │  PolicyReconciler              │  │
│  └────────┬─────────┘      │  - validates Rego              │  │
│           │                │  - creates PolicyVersion        │  │
│           │ watches        │  - handles rollback             │  │
│           ▼                │                                │  │
│  ┌──────────────────┐      │  WebhookConfigReconciler       │  │
│  │   Policy CRD     │      │  - aggregates rules            │  │
│  │   (cluster-scope)│      │  - patches VWC                 │  │
│  └──────────────────┘      └────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────┐      ┌────────────────────────────────┐  │
│  │  PolicyGroup CRD │      │  PolicyVersion CRD             │  │
│  │  (cluster-scope) │      │  (immutable snapshots)         │  │
│  └──────────────────┘      └────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

## Features

- **OPA/Rego policy engine** — embeds OPA directly, no sidecar needed
- **CRD-based policies** — define policies as `Policy` Kubernetes resources
- **Built-in policy groups** — 37 curated rules across Security, Efficiency, and Reliability groups, deployed automatically at install time
- **Policy groups** — group policies together with per-group and per-policy enable/disable switches
- **Custom policy groups** — create your own `PolicyGroup` CRDs; when a custom policy shares the same key as a built-in, the custom one wins
- **Structured violation messages** — denials include `[group/key] message` and a description field; audit violations appear as `AdmissionResponse.Warnings`
- **Enforcement modes** — `enforce` (blocks) or `audit` (logs + warnings only)
- **Version control** — every policy change creates an immutable `PolicyVersion` snapshot
- **Rollback** — set `spec.rollbackTo.version` to restore any previous version
- **Dynamic webhook rules** — Operator automatically updates `ValidatingWebhookConfiguration` based on Ready policies
- **Self-signed TLS** — auto-generated at Helm install time via a pre-install Job
- **Parallel evaluation** — multiple policies evaluated concurrently with a 5s timeout
- **Leader election** — Operator runs with leader election for HA
- **Multi-platform** — images support `linux/amd64` and `linux/arm64`

## Built-in Policy Groups

KubeSentry ships 37 built-in policies across three groups, enabled by default:

### Security (23 policies)

| Key | Default Mode | Description |
|---|---|---|
| `runAsPrivileged` | enforce | Blocks containers running as privileged |
| `privilegeEscalationAllowed` | enforce | Blocks `allowPrivilegeEscalation: true` |
| `runAsRootAllowed` | audit | Warns when `runAsNonRoot` is not set |
| `notReadOnlyRootFilesystem` | audit | Warns when `readOnlyRootFilesystem` is not set |
| `linuxHardening` | enforce | Requires at least one of: seccompProfile, seLinuxOptions, capabilities.drop |
| `insecureCapabilities` | audit | Warns on insecure capability additions |
| `dangerousCapabilities` | enforce | Blocks dangerous capabilities (SYS_ADMIN, NET_ADMIN, …) |
| `hostPIDSet` | enforce | Blocks `hostPID: true` |
| `hostIPCSet` | enforce | Blocks `hostIPC: true` |
| `hostNetworkSet` | audit | Warns on `hostNetwork: true` |
| `hostPortSet` | audit | Warns when `hostPort` is set |
| `sensitiveContainerEnvVar` | enforce | Blocks env vars with names matching sensitive patterns |
| `automountServiceAccountToken` | audit | Warns when service account token is auto-mounted |
| `sensitiveConfigmapContent` | enforce | Blocks ConfigMaps with sensitive-looking keys |
| `tlsSettingsMissing` | audit | Warns on Ingresses without TLS |
| `clusterrolePodExecAttach` | enforce | Blocks ClusterRoles granting pods/exec or pods/attach |
| `rolePodExecAttach` | enforce | Blocks Roles granting pods/exec or pods/attach |
| `clusterrolebindingPodExecAttach` | enforce | Blocks ClusterRoleBindings referencing exec/attach roles by name |
| `rolebindingRolePodExecAttach` | enforce | Blocks RoleBindings referencing exec/attach roles by name |
| `rolebindingClusterRolePodExecAttach` | enforce | Blocks RoleBindings referencing exec/attach ClusterRoles by name |
| `clusterrolebindingClusterAdmin` | enforce | Blocks ClusterRoleBindings to cluster-admin |
| `rolebindingClusterAdminClusterRole` | enforce | Blocks RoleBindings to cluster-admin ClusterRole |
| `rolebindingClusterAdminRole` | enforce | Blocks RoleBindings to Roles named cluster-admin |

### Efficiency (4 policies)

| Key | Default Mode | Description |
|---|---|---|
| `cpuRequestsMissing` | audit | Warns when CPU requests are not set |
| `memoryRequestsMissing` | audit | Warns when memory requests are not set |
| `cpuLimitsMissing` | audit | Warns when CPU limits are not set |
| `memoryLimitsMissing` | audit | Warns when memory limits are not set |

### Reliability (10 policies)

| Key | Default Mode | Description |
|---|---|---|
| `readinessProbeMissing` | audit | Warns when readiness probe is absent |
| `livenessProbeMissing` | audit | Warns when liveness probe is absent |
| `tagNotSpecified` | enforce | Blocks images without a tag or using `:latest` |
| `pullPolicyNotAlways` | audit | Warns when imagePullPolicy is not Always |
| `priorityClassNotSet` | audit | Warns when priorityClassName is not set |
| `deploymentMissingReplicas` | audit | Warns when a Deployment has fewer than 2 replicas |
| `metadataAndInstanceMismatched` | audit | Warns when `metadata.name` and `app.kubernetes.io/instance` differ |
| `topologySpreadConstraint` | audit | Warns when no topology spread constraints are defined |
| `hpaMaxAvailability` | audit | Warns when HPA maxReplicas ≤ minReplicas |
| `hpaMinAvailability` | audit | Warns when HPA minReplicas ≤ 1 |

### Customizing built-in groups

Disable a whole group or individual policies via Helm values:

```yaml
builtinGroups:
  security:
    enabled: true
    policies:
      hostNetworkSet:
        enabled: false       # disable this policy
      runAsRootAllowed:
        mode: enforce        # override mode to enforce
  efficiency:
    enabled: false           # disable the entire group
```

## PolicyGroup CRD

You can also create your own groups with a mix of built-in and custom policies:

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: PolicyGroup
metadata:
  name: my-policies
spec:
  enabled: true
  displayName: "My Custom Policies"
  policies:
    # use a built-in policy with mode override
    - key: runAsPrivileged
      mode: enforce
    # custom policy not in the built-in library
    - key: noDebugContainers
      mode: enforce
      rego: |
        package kubesentry
        deny[msg] {
          c := input.request.object.spec.containers[_]
          c.name == "debug"
          msg := "debug containers are not allowed"
        }
      match:
        operations: [CREATE, UPDATE]
        resources:
          - apiGroups: [""]
            apiVersions: ["v1"]
            resources: ["pods"]
```

When a `PolicyGroup` entry shares a `key` with an existing standalone `Policy` (one with no `OwnerReference` to any `PolicyGroup`), the standalone policy takes priority and the group entry is skipped.

### Violation messages

When a policy triggers, the response includes the group, key, and description:

```
[security/runAsPrivileged] container "app" must not run as privileged
  描述：Fails when securityContext.privileged is true.
```

`audit`-mode violations appear as `AdmissionResponse.Warnings` (request is allowed).

## PolicyException — time-bound, audited exemptions

`PolicyException` lets you bypass specific Policies (or whole PolicyGroups, or
even every policy) for a scoped set of resources, for a bounded amount of time.

### Quick example

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: PolicyException
metadata:
  name: hr-system-legacy-migration
spec:
  policyRefs:
    - run-as-privileged
  match:
    namespaces: [hr-system]
  duration: 24h
  reason: "Legacy billing migration; ticket OPS-1245"
```

### Fields

- `policyRefs` / `policyGroupRefs` / `allPolicies` — exactly one. Choose specific
  Policies, a whole PolicyGroup, or whitelist everything.
- `match.namespaces` — exact namespace names (no glob).
- `match.namespaceSelector` — labels on the `Namespace` object.
- `match.resourceSelector` — labels on the admitted object itself.
- `duration` — required, Go `time.Duration` (e.g. `24h`, `30m`).
- `retainAfterExpiry` — optional; default `0` (delete immediately at expiry).
- `reason` — required, non-empty audit string.

### Rules

- Time origin is `metadata.creationTimestamp`. Editing `duration` recomputes
  `status.expiresAt`; `status.effectiveAt` never moves.
- `Expired` is terminal — a once-expired exception cannot be revived by
  editing `duration`. Renew by creating a new object.
- Only `duration`, `retainAfterExpiry`, and `reason` are mutable. Everything
  else (targets, match) is locked at creation.
- Immutability is enforced via a `ValidatingAdmissionPolicy` whose CEL
  comparison treats list fields (`policyRefs`, `policyGroupRefs`,
  `match.namespaces`) as **ordered**. Patches that reorder elements without
  changing the set are rejected as immutability violations. Tools that
  re-sort JSON arrays during apply should be configured to preserve the
  original order, or you should re-create the object instead of patching.

## Standalone Policy Example

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: deny-privileged
spec:
  enforcementMode: enforce
  match:
    operations: [CREATE, UPDATE]
    resources:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
  rego: |
    package kubesentry

    deny[msg] {
      input.request.object.spec.containers[_].securityContext.privileged == true
      msg := "privileged containers are not allowed"
    }
```

See [`examples/`](examples/) for more ready-to-use policies.

### Pod interception vs. Deployment interception

By default policies match `pods`, which covers all workload types. When a
`Deployment` is applied, `kubectl apply` succeeds immediately — the webhook
only fires when the Deployment controller later tries to create the Pod, so
rejection appears in `kubectl describe deployment` events rather than on the
command line.

If you want `kubectl apply` on a Deployment to fail immediately, add
`deployments` to the match rules **and** write separate Rego rules for each
resource type, because the container path differs:

| Resource | Container path in Rego |
|---|---|
| `pods` | `input.request.object.spec.containers[_]` |
| `deployments` | `input.request.object.spec.template.spec.containers[_]` |

Example policy that intercepts both — see
[`examples/policy-no-privileged-with-deployments.yaml`](examples/policy-no-privileged-with-deployments.yaml).

## Rollback

Set `spec.rollbackTo.version` — the Operator restores `spec.rego`, `spec.match`, and `spec.enforcementMode` from the target `PolicyVersion` and clears the field automatically:

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: deny-privileged
spec:
  rollbackTo:
    version: 2
```

## Installation

### Prerequisites

- Kubernetes 1.28+
- Helm 3.8+
- `kubectl` configured

### Install (latest)

```bash
helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --namespace kubesentry-system \
  --create-namespace
```

### Install a specific version

```bash
helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --version 1.0.1 \
  --namespace kubesentry-system \
  --create-namespace
```

### Install from source

```bash
helm install kubesentry charts/kubesentry \
  --namespace kubesentry-system \
  --create-namespace
```

### Docker Hub images

| Image | Tags |
|---|---|
| [`wynnhub/kubesentry-webhook`](https://hub.docker.com/r/wynnhub/kubesentry-webhook) | `latest`, `v1.0.1`, `v1.0.0` |
| [`wynnhub/kubesentry-operator`](https://hub.docker.com/r/wynnhub/kubesentry-operator) | `latest`, `v1.0.1`, `v1.0.0` |

Both images are public and support `linux/amd64` and `linux/arm64`.

## Configuration

| Value | Default | Description |
|---|---|---|
| `webhook.replicas` | `2` | Webhook server replicas |
| `webhook.image.tag` | *(chart appVersion)* | Override image tag |
| `operator.replicas` | `1` | Operator replicas |
| `operator.image.tag` | *(chart appVersion)* | Override image tag |
| `tls.secretName` | `kubesentry-tls` | TLS Secret name |
| `failurePolicy` | `Fail` | Webhook failure policy |
| `policy.versionHistoryLimit` | `20` | Max `PolicyVersion` objects per Policy |
| `webhookNamespaceSelector` | excludes `kube-system`, `kubesentry-system` | Namespace selector |
| `builtinGroups.security.enabled` | `true` | Enable the Security policy group |
| `builtinGroups.efficiency.enabled` | `true` | Enable the Efficiency policy group |
| `builtinGroups.reliability.enabled` | `true` | Enable the Reliability policy group |
| `builtinGroups.<group>.policies.<key>.enabled` | — | Enable/disable a single built-in policy |
| `builtinGroups.<group>.policies.<key>.mode` | — | Override mode (`enforce`\|`audit`) for a single policy |

## Development

### Requirements

- Go 1.26+
- Docker (for cross-platform builds)
- Helm 3.8+

### Common commands

```bash
make test          # run all tests
make build         # compile for local arch → bin/webhook, bin/operator
make lint          # go vet
make helm-package  # lint + package chart → dist/kubesentry-<version>.tgz
```

### Release pipeline

```bash
# First time: login to Docker Hub
docker login -u wynnhub
helm registry login registry-1.docker.io -u wynnhub

# Tag and release
git tag v1.0.1
make release VERSION=v1.0.1
```

`make release` runs in order:

| Step | Command | Output |
|---|---|---|
| 1. Test | `go test ./...` | — |
| 2. Cross-compile | `docker run golang:1.26-alpine go build` | `bin/linux-amd64/`, `bin/linux-arm64/` |
| 3. Push images | `docker buildx ... --push` | `wynnhub/kubesentry-webhook:v1.0.1` + `:latest` |
| 4. Package chart | `helm package` | `dist/kubesentry-1.0.1.tgz` |
| 5. Push chart | `helm push ... oci://` | `oci://registry-1.docker.io/wynnhub/kubesentry:1.0.1` |

### Multi-platform images

Images are published as OCI Manifest Lists. Kubernetes selects the correct platform automatically at pull time — no chart configuration needed.

```bash
# Inspect published platforms
docker buildx imagetools inspect wynnhub/kubesentry-webhook:latest
```

### Project structure

```
kubesentry/
├── cmd/
│   ├── webhook/main.go       # webhook server entrypoint
│   └── operator/main.go      # operator + tls-setup subcommand
├── internal/
│   ├── api/v1alpha1/         # CRD type definitions
│   ├── builtins/             # embedded Rego library (37 built-in policies)
│   │   └── rego/             # .rego files (one per policy)
│   ├── webhook/              # OPA evaluator, cache, HTTP handler
│   ├── operator/             # Policy, PolicyGroup, and WebhookConfig reconcilers
│   └── tlssetup/             # ECDSA cert generation
├── charts/kubesentry/        # Helm chart
│   ├── crds/                 # CRD manifests (Policy, PolicyVersion, PolicyGroup)
│   └── templates/            # K8s resource templates
├── Dockerfile.webhook        # runtime-only, no build step
└── Dockerfile.operator
```

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
