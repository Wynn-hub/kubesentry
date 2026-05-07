# KubeSentry

English | [中文](README_zh.md)

A Kubernetes Validating Admission Webhook that enforces OPA/Rego policies defined as CRDs, with an Operator for lifecycle management and version control.

## Architecture

Two independent Go binaries share the same CRD API types:

```
┌─────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                    │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────────────┐ │
│  │  kubesentry-     │      │   kubesentry-operator    │ │
│  │  webhook         │      │                          │ │
│  │                  │      │  PolicyReconciler        │ │
│  │  - OPA evaluator │      │  - validates Rego        │ │
│  │  - Policy cache  │      │  - creates PolicyVersion │ │
│  │  - /validate     │      │  - handles rollback      │ │
│  │  - /healthz      │      │                          │ │
│  │  - /readyz       │      │  WebhookConfigReconciler │ │
│  └────────┬─────────┘      │  - aggregates rules      │ │
│           │                │  - patches VWC           │ │
│           │ watches        └──────────────────────────┘ │
│           ▼                                             │
│  ┌──────────────────┐      ┌──────────────────────────┐ │
│  │   Policy CRD     │      │  PolicyVersion CRD       │ │
│  │   (cluster-scope)│      │  (immutable snapshots)   │ │
│  └──────────────────┘      └──────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

## Features

- **OPA/Rego policy engine** — embeds OPA directly, no sidecar needed
- **CRD-based policies** — define policies as `Policy` Kubernetes resources
- **Enforcement modes** — `enforce` (blocks) or `audit` (logs only)
- **Version control** — every policy change creates an immutable `PolicyVersion` snapshot
- **Rollback** — set `spec.rollbackTo.version` to restore any previous version
- **Dynamic webhook rules** — Operator automatically updates `ValidatingWebhookConfiguration` based on Ready policies
- **Self-signed TLS** — auto-generated at Helm install time via a pre-install Job
- **Parallel evaluation** — multiple policies evaluated concurrently with a 5s timeout
- **Leader election** — Operator runs with leader election for HA
- **Multi-platform** — images support `linux/amd64` and `linux/arm64`

## Policy Example

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
- Helm 3.8+ (OCI support)
- `kubectl` configured

### Install from Docker Hub

```bash
# Login once (only needed for private repositories)
helm registry login registry-1.docker.io -u wynnhub

helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --version 0.1.0 \
  --namespace kubesentry-system \
  --create-namespace
```

The Helm pre-install Job generates a self-signed CA and server certificate, stores them in a Secret, and patches the `ValidatingWebhookConfiguration` `caBundle` automatically.

### Install from source

```bash
helm install kubesentry charts/kubesentry \
  --namespace kubesentry-system \
  --create-namespace
```

## Configuration

| Value | Default | Description |
|---|---|---|
| `webhook.replicas` | `2` | Webhook server replicas |
| `operator.replicas` | `1` | Operator replicas |
| `tls.secretName` | `kubesentry-tls` | TLS Secret name |
| `failurePolicy` | `Fail` | Webhook failure policy |
| `policy.versionHistoryLimit` | `20` | Max `PolicyVersion` objects per Policy |
| `webhookNamespaceSelector` | excludes `kube-system`, `kubesentry-system` | Namespace selector |

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
git tag v0.1.0
make release
```

`make release` runs in order:

| Step | Command | Output |
|---|---|---|
| 1. Test | `go test ./...` | — |
| 2. Cross-compile | `docker run golang:1.26-alpine go build` | `bin/linux-amd64/`, `bin/linux-arm64/` |
| 3. Push images | `docker buildx ... --push` | `wynnhub/kubesentry-webhook:v0.1.0` (amd64 + arm64 manifest) |
| 4. Package chart | `helm package` | `dist/kubesentry-0.1.0.tgz` |
| 5. Push chart | `helm push ... oci://` | `oci://registry-1.docker.io/wynnhub/kubesentry:0.1.0` |

### Multi-platform images

Images are published as OCI Manifest Lists. Kubernetes selects the correct platform automatically at pull time based on the node architecture — no chart configuration needed.

```bash
# Inspect published platforms
docker buildx imagetools inspect wynnhub/kubesentry-webhook:v0.1.0
```

### Project structure

```
kubesentry/
├── cmd/
│   ├── webhook/main.go       # webhook server entrypoint
│   └── operator/main.go      # operator + tls-setup subcommand
├── internal/
│   ├── api/v1alpha1/         # CRD type definitions
│   ├── webhook/              # OPA evaluator, cache, HTTP handler
│   ├── operator/             # Policy and WebhookConfig reconcilers
│   └── tlssetup/             # ECDSA cert generation
├── charts/kubesentry/        # Helm chart
│   ├── crds/                 # CRD manifests
│   └── templates/            # K8s resource templates
├── Dockerfile.webhook        # runtime-only, no build step
└── Dockerfile.operator
```

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
