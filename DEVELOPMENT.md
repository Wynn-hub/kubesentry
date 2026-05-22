# KubeSentry Development Guide

This document provides deep insights into KubeSentry's architecture, design decisions, and development workflows.

## Project Structure

```
kubesentry/
├── cmd/
│   ├── webhook/main.go          # Webhook server entrypoint
│   └── operator/main.go          # Operator + tls-setup subcommand
├── internal/
│   ├── api/v1alpha1/            # CRD type definitions + scheme
│   ├── webhook/                 # OPA evaluator, cache, HTTP handler
│   ├── operator/                # Policy/PolicyGroup/WebhookConfig/Exception reconcilers
│   └── tlssetup/                # ECDSA cert generation
├── charts/kubesentry/           # Helm chart
│   ├── crds/                    # CRD manifests
│   ├── builtin-policies/        # Source-of-truth for the built-in catalogue
│   │   ├── policies/            # 37 standalone Policy CRs (one yaml per policy)
│   │   └── groups/              # 3 PolicyGroup member manifests
│   └── templates/               # Renderers (builtin-policies.yaml + builtin-groups.yaml)
├── test/builtins/               # helm template + CompileRego smoke test
├── examples/                    # Sample policies and test resources
├── CONTRIBUTING.md              # Contribution guidelines
├── DEVELOPMENT.md               # This file
├── CODE_OF_CONDUCT.md           # Community standards
├── CLAUDE.md                    # AI assistant guidance
└── README.md                    # User-facing documentation
```

## Two-Binary Architecture

KubeSentry consists of two independent Go binaries that communicate **only through Kubernetes CRDs**:

### `webhook` (Data Plane)

**Role**: Evaluate admission requests against cached policies

**Endpoints:**
- `POST /validate` — Kubernetes webhook validation handler
- `GET /healthz` — Health check (always succeeds)
- `GET /readyz` — Readiness check (reflects cache sync status)

**Key Components:**
- **PolicyCache** (`internal/webhook/cache.go`) — In-memory map of compiled policies, protected by `sync.RWMutex`
- **Handler** (`internal/webhook/handler.go`) — HTTP handler that calls cache and evaluates policies concurrently
- **Evaluator** (`internal/webhook/evaluator.go`) — OPA integration, Rego compilation and evaluation

**Lifecycle:**
1. Startup: Build controller-runtime informer cache watching `Policy`, `PolicyGroup`, `Namespace`, and `PolicyException` CRDs
2. Cache sync: Compile every `PhaseReady` `Policy` into the in-memory `PolicyCache`; build `CompiledGroup.Members[]` from each `PolicyGroup.status.resolvedPolicies`. `PhaseInvalid` policies are removed (or skipped) — they never reach the cache.
3. Per request:
   - Call `Cache.MatchingForRequest()` — filters enabled `PolicyGroup`s by `namespaceSelector`, gathers candidate policies, picks the strictest effective mode across matching groups
   - Apply any active `PolicyException` (cached via `ExceptionCache` with namespace index + selector matching)
   - Spawn goroutine per surviving policy, evaluate with 5s timeout
   - Collect `enforce`-mode denials → return `Allowed=false`
   - Collect `audit`-mode violations → return `Allowed=true` with warnings

**Design Decision**: Why in-memory cache instead of querying Kubernetes API per request?
- **Performance**: Avoid API roundtrip latency on hot path (every admission request)
- **Decoupling**: Webhook doesn't depend on operator availability; can run read-only
- **Resilience**: Cache survives operator restarts (policies remain active)

### `operator` (Management Plane)

**Role**: Manage policy lifecycle, generate snapshots, sync webhook configuration

**Subcommands:**
- Default: Start the manager with three reconcilers
- `tls-setup`: Pre-install Hook Job that generates TLS certificates (runs once)

**Key Components:**
- **PolicyGroupReconciler** — Watches `PolicyGroup` CRDs **and** all `Policy` CRDs (reverse-enqueue via `MapFunc` on Policy create/delete/label-change). Resolves group members from `spec.policies.byName` + `spec.policies.bySelector` (byName takes precedence on name collision). Computes per-entry effective `enforcementMode` (`byName` entry override, else `selectorEnforcementMode`, else the Policy's own `spec.enforcementMode`). Writes `status.resolvedPolicies` + `status.resolvedCount` + `status.phase` (`Ready` / `Degraded` / `Disabled`). Maintains `Policy.status.referencedBy` by diffing previous-resolved ∪ current-resolved.
- **PolicyReconciler** — Validates Rego, creates immutable `PolicyVersion` snapshots, prunes old versions, handles rollback. **No longer aware of "builtin"** — every `Policy` is a first-class top-level object regardless of whether it was created by the user or rendered by the Helm chart.
- **WebhookConfigReconciler** — Lists every `Ready` policy (regardless of group membership), aggregates `match.resources`, patches `ValidatingWebhookConfiguration.Webhooks[*].Rules`.
- **ExceptionReconciler** — Validates `PolicyException` lifecycle (`effectiveAt`, `expiresAt`, terminal `Expired` phase). `policyGroupRefs` is dynamic — the exempt set follows `PolicyGroup.status.resolvedPolicies` (no schema change).

**Lifecycle:**
1. Startup: Register all four reconcilers with the controller-runtime manager
2. Leader election: Only one operator replica at a time runs reconciliation
3. Per PolicyGroup change (or reverse-enqueue from Policy CRUD/label-change):
   - Resolve members: union `byName` + `bySelector`, dropping `byName` duplicates
   - Compute each member's effective `enforcementMode`
   - Update `status.resolvedPolicies` and recompute `status.phase`
   - Diff previous-resolved vs current-resolved → update affected `Policy.status.referencedBy`
4. Per Policy change:
   - Validate Rego syntax
   - Create immutable `PolicyVersion` snapshot (idempotent on `AlreadyExists`)
   - Prune versions beyond `policy.versionHistoryLimit`
   - Update `Policy.status` (`Ready` / `Invalid`)
5. Per Policy Status change (Phase=`Ready` only):
   - Aggregate `match.resources` across every `Ready` policy in the cluster
   - Patch `ValidatingWebhookConfiguration.Webhooks[*].Rules`

**Design Decision**: Why is `PolicyGroup` a pure reference object (no OwnerReference)?
- **First-class policies**: A built-in `Policy` shipped by the chart is indistinguishable from a user-applied one. The user owns it via Helm/GitOps; the operator just classifies it via group membership.
- **No cascade surprises**: Deleting a `PolicyGroup` leaves its referenced `Policy` CRs intact — only `status.referencedBy` is cleared. Users delete `Policy` CRs directly when they want them gone.
- **Independent reuse**: One `Policy` can be referenced by multiple groups (potentially with different effective modes) without ownership conflict — per-request strictest-mode resolution handles overlap.
- **Helm composability**: The chart can render the policy yaml + group yaml side by side, with the operator simply doing the join at runtime.

### Data Flow

```
charts/kubesentry/builtin-policies/  +  user-applied Policy/PolicyGroup CRs
    ↓ (Helm renders → kube-apiserver)
Policy (cluster-scope CRD)  +  PolicyGroup (cluster-scope CRD)
    ↓ (informer sync, reverse-enqueue)
PolicyGroupReconciler resolves byName ∪ bySelector → status.resolvedPolicies
    ↓ (informer sync into webhook)
PolicyCache (compiled per-policy + CompiledGroup with Members[])
    ↓ (per-request: groups whose namespaceSelector matches → strictest mode)
AdmissionResponse (accept / deny / warnings)
```

Webhook and operator can restart independently — the cache resyncs from `Policy` + `PolicyGroup` CRDs.

## Key Invariants and Constraints

### 1. OPA Rego v0 Syntax Only

The bundled OPA version does **not** support Rego v1 syntax. All `.rego` files must use v0:

✓ **Correct:**
```rego
package kubesentry
deny[msg] {
    c := input.request.object.spec.containers[_]
    c.securityContext.privileged == true
    msg := "cannot run privileged"
}
```

✗ **Wrong (v1 syntax, will fail):**
```rego
import rego.v1
deny contains msg if {
    some c in input.request.object.spec.containers
    c.securityContext.privileged == true
    msg := "cannot run privileged"
}
```

**Key differences:**
- Use `set[_]` instead of `some x in set`
- Use `_` wildcards for unused variables
- No `import rego.v1`

All 37 built-in policies in `charts/kubesentry/builtin-policies/policies/` use v0 syntax. The smoke test in `test/builtins/compile_test.go` runs `helm template` and feeds every rendered Policy CR through `webhook.CompileRego` to keep this guarantee.

### 2. Rego Module Contract

Every policy Rego module must expose a `deny` set in the `kubesentry` package:

```rego
package kubesentry

deny[msg] { ... msg := "reason" }
```

The evaluator queries `data.kubesentry.deny` to extract violation messages. Policies that don't follow this pattern compile but always allow.

### 3. Label Convention (Chart-Authored)

Built-in policies are rendered by the Helm chart with two labels — the operator does **not** add or own them:

| Label | Value | Purpose |
|---|---|---|
| `kubesentry.io/category` | `security` / `efficiency` / `reliability` | Picked up by `bySelector` group members; user-defined Policies can opt in with the same label |
| `app.kubernetes.io/managed-by` | `Helm` | Standard Helm ownership marker |

When `PolicyGroup.spec.policies.bySelector.matchLabels` is set, the PolicyGroupReconciler dynamically includes every `Policy` whose labels match (including user-applied ones that adopt `kubesentry.io/category=<group>`). See the README "bySelector dynamic capture" warning — disabling a built-in policy via `builtinPolicies.<name>.enabled=false` does not stop a future copy of that Policy from being re-captured by the same selector.

### 4. Naming Convention

Built-in `Policy.metadata.name` uses kebab-case (e.g. `run-as-privileged`, `linux-hardening`) — there is no longer a camelCase → kebab-case conversion step inside the operator. The yaml files in `charts/kubesentry/builtin-policies/policies/` are the source of truth; both the file basename and `metadata.name` must match the kebab-case form.

### 5. PolicyVersion Naming and Labels

`PolicyVersion` objects are named `{policy-name}-v{version}` and carry labels for queries:

```yaml
metadata:
  name: deny-privileged-v2
  labels:
    kubesentry.io/policy: deny-privileged
    kubesentry.io/version: "2"
```

These labels are used during rollback (select target version) and pruning (delete oldest versions).

### 6. zz_generated.deepcopy.go is Hand-Written

⚠️ **Important**: Do NOT run `make generate` or `controller-gen` to regenerate this file.

**Why**: `controller-gen` is incompatible with Go 1.26.2 and our k8s.io/api v0.31 dependency pinning.

**How to edit**: When adding new fields to `internal/api/v1alpha1/types.go`:

1. Add the field to the struct
2. Manually add a corresponding line in the `DeepCopyInto()` method:
   ```go
   out.NewField = in.NewField.DeepCopy()  // for pointer types
   // OR
   out.NewField = in.NewField  // for value types (strings, ints, bools)
   ```
3. For slice fields, use:
   ```go
   if in.Slice != nil {
       out.Slice = make([]Type, len(in.Slice))
       copy(out.Slice, in.Slice)
   }
   ```
4. Note: `metav1.Time.DeepCopy()` returns `*metav1.Time`, so assign with `out.Time = in.Time.DeepCopy()` (not `&`).

See `internal/api/v1alpha1/zz_generated.deepcopy.go` for complete examples.

## Testing Patterns

### Test Organization

Tests are split across two shared contexts:

**Operator package** (`internal/operator/*_test.go`):
- All files in the package share `buildScheme()` (defined in `policy_reconciler_test.go:28`)
- Add new scheme requirements to `buildScheme()`, e.g., `admissionregv1.AddToScheme(s)` for VWC tests

**Webhook package** (`internal/webhook/*_test.go`):
- All files share `denyPrivilegedRego` constant (defined in `evaluator_test.go:10`)
- All files share test helpers like `stubStore`, `buildAdmissionRequest()`, `mustJSON()`

### Creating New Tests

```bash
# For a new package, create a _test.go with standard imports
cat > internal/mypackage/example_test.go << 'EOF'
package mypackage_test

import (
    "testing"
    "github.com/Wynn-hub/kubesentry/internal/mypackage"
)

func TestMyFeature(t *testing.T) {
    // Arrange
    ...
    // Act
    result := mypackage.MyFunction()
    // Assert
    if result != expected { t.Errorf(...) }
}
EOF
```

### Coverage Requirements

Minimum 80% coverage for core packages:
- `internal/webhook` — webhook functionality (cache, evaluator, handler, ExceptionCache)
- `internal/operator` — policy/group/webhookconfig/exception reconcilers
- `internal/api/v1alpha1` — CRD types

The built-in policy catalogue is covered by `test/builtins/compile_test.go` — it `helm template`s the chart and asserts every rendered Policy compiles via `webhook.CompileRego`, plus a count assertion (≥37).

Entrypoints (`cmd/webhook`, `cmd/operator`) are integration-tested via Kubernetes.

## Local Development Workflow

### 1. Setting Up a Test Cluster

```bash
# Create a local cluster (if not using existing cluster)
kind create cluster --name dev

# Set kubeconfig
export KUBECONFIG=$HOME/.kube/config

# Verify cluster
kubectl get nodes
```

### 2. Running Tests Locally

```bash
# All tests with coverage
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go test ./... -cover

# Specific package
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go test ./internal/webhook/... -v

# With verbose output and stop on first failure
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go test ./... -v -failfast
```

### 3. Building Binaries

```bash
# Build for local architecture
make build
# Outputs: bin/webhook bin/operator

# Build for Docker (multi-platform)
docker buildx build --platform linux/amd64,linux/arm64 -f Dockerfile.webhook .
docker buildx build --platform linux/amd64,linux/arm64 -f Dockerfile.operator .
```

### 4. Deploying Locally

```bash
# Load images into kind
kind load docker-image wynnhub/kubesentry-webhook:latest --name dev
kind load docker-image wynnhub/kubesentry-operator:latest --name dev

# Install via Helm (using local images)
helm install kubesentry charts/kubesentry \
  --namespace kubesentry-system --create-namespace \
  --set webhook.image.pullPolicy=Never \
  --set operator.image.pullPolicy=Never

# Watch installation
kubectl get pods -n kubesentry-system -w
```

### 5. Testing a Policy

```bash
# Apply a test policy
kubectl apply -f examples/policy-no-privileged.yaml

# Check policy status
kubectl get policies
kubectl describe policy deny-privileged-containers

# Test with allowed pod
kubectl apply -f examples/test-normal-pod.yaml
# Should show: created

# Test with denied pod (watch for event)
kubectl apply -f examples/test-privileged-pod.yaml
# Should show admission webhook rejection in `kubectl describe pod`
```

### 6. Debugging

#### View Webhook Logs
```bash
kubectl logs -n kubesentry-system -l app=webhook -f

# Or specific pod
kubectl logs -n kubesentry-system webhook-0
```

#### View Operator Logs
```bash
kubectl logs -n kubesentry-system -l app=operator -f
```

#### Inspect CRDs
```bash
# List policies
kubectl get policies -A

# Show details
kubectl get policy deny-privileged -o yaml

# Check status
kubectl describe policy deny-privileged
```

#### OPA REPL for Rego Debugging

Test Rego locally before deploying:

```bash
# Install opa cli
# https://www.openpolicyagent.org/docs/latest/introduction/#running-opa

# Run REPL
opa run

# Load your rego file
data = data.json  # paste your input
# Then query
query := data_kubesentry_deny

# Or test directly
opa test charts/kubesentry/builtin-policies/policies/
```

## Rego Policy Development Guide

### Container Paths Reference

```rego
// Pods: access containers and initContainers directly
input.request.object.spec.containers[_]
input.request.object.spec.initContainers[_]

// Deployments, DaemonSets, StatefulSets: nested under template
input.request.object.spec.template.spec.containers[_]
input.request.object.spec.template.spec.initContainers[_]

// CronJobs: further nested under jobTemplate
input.request.object.spec.jobTemplate.spec.template.spec.containers[_]

// Accessing request metadata
input.request.resource.resource      // "pods", "deployments", etc.
input.request.resource.apiGroup      // "", "apps", etc.
input.request.operation              // "CREATE", "UPDATE", etc.
input.request.namespace              // target namespace
input.request.object.metadata.name   // resource name
```

### Common Rego Patterns

**Check container count:**
```rego
count(input.request.object.spec.containers[_]) > 1
```

**Iterate containers with filtering:**
```rego
c := input.request.object.spec.containers[_]
c.name == "app"
c.securityContext.runAsRoot == true
```

**Set intersection (capabilities):**
```rego
c := input.request.object.spec.containers[_]
caps := c.securityContext.capabilities.add[_]
dangerous := {"SYS_ADMIN", "NET_ADMIN"}
caps in dangerous
```

**Default values:**
```rego
mode := object.get(c.securityContext, "runAsNonRoot", false)
```

### Testing Your Rego

Use the OPA test framework:

```bash
# Create a sidecar test file next to your policy, e.g.
# charts/kubesentry/builtin-policies/policies/mypolicy_test.rego
test_deny_privileged {
    count(deny) > 0 with data.kubesentry.deny as [_]
    # where _ is your input
}

# Run tests
opa test charts/kubesentry/builtin-policies/policies/mypolicy_test.rego
```

> Note: the chart only ships `*.yaml` files via `policies/*.yaml` globs, so
> standalone `_test.rego` files are not included in the rendered release.

## Release Process

### Version Scheme

Uses semantic versioning: `vMAJOR.MINOR.PATCH`

- `MAJOR` — Breaking changes (e.g., CRD schema incompatible)
- `MINOR` — New features (e.g., new built-in policies)
- `PATCH` — Bug fixes and improvements

### Release Steps

1. **Create a Git tag:**
   ```bash
   git tag -a v0.2.0 -m "Release 0.2.0"
   git push origin v0.2.0
   ```

2. **Run release build:**
   ```bash
   make release  # Cross-compile, push images, publish chart
   ```

   This performs:
   - Run all tests
   - Cross-compile for `linux/amd64` and `linux/arm64`
   - Build and push multi-platform Docker images
   - Package and push Helm chart to OCI registry

3. **Verify release:**
   ```bash
   # Inspect published images
   docker buildx imagetools inspect wynnhub/kubesentry-webhook:v0.2.0

   # Verify Helm chart
   helm search repo kubesentry --version 0.2.0
   ```

4. **Create GitHub Release:**
   - Go to Releases page
   - Paste changelog (automated from commits)
   - Link to published artifacts

## IDE and Tooling Notes

### gopls LSP False Positives

The Go language server (gopls) frequently reports false-positive "undefined" errors even after successful builds. Example:

```
undefined: v1alpha1.PolicyGroup  ← gopls complains
$ go build ./...  ← compiler is fine
```

**Resolution**: Always verify with the actual Go compiler (`go build`, `go test`). IDE errors are stale cache.

To refresh gopls:
- VS Code: Reload window (Cmd+Shift+P → Reload Window)
- GoLand: Restart IDE or clear cache (File → Invalidate Caches)

### Makefile Commands

```bash
make test          # Run all tests
make build         # Compile for local arch → bin/webhook, bin/operator
make lint          # go vet check
make helm-package  # Lint + package chart → dist/
make release       # Full release pipeline (tests, images, chart)
```

All `make` targets use the correct GOROOT automatically.

## Running the t5 PolicyException suite

`t5_exception_test.go` exercises PolicyException end-to-end. It is included in
`make test-e2e` and `make test-e2e-report` automatically.

To run just t5:

```bash
make build-image-e2e
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec \
  go test -tags=e2e ./test/e2e/... -run TestT5 -v -timeout 5m
```

The suite uses wall-clock waits for expiry (≈60s). To skip during local
iteration, exclude the run pattern:

```bash
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec \
  go test -tags=e2e ./test/e2e/... -run "TestT[1-4]_" -v
```

## Further Reading

- [CONTRIBUTING.md](CONTRIBUTING.md) — Contribution guidelines
- [README.md](README.md) — User documentation
- [CLAUDE.md](CLAUDE.md) — AI assistant context
- Kubernetes Admission Webhooks: https://kubernetes.io/docs/reference/access-authn-authz/webhook/
- OPA Policy Language: https://www.openpolicyagent.org/docs/latest/policy-language/
- controller-runtime docs: https://pkg.go.dev/sigs.k8s.io/controller-runtime
