# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Go Toolchain Warning

This machine has a Go toolchain version mismatch. The `go` binary is Go 1.26.2 (homebrew) but `~/.zshrc` sets `GOROOT` to a different path. **Always prefix `go` commands with the correct GOROOT:**

```bash
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go <command>
```

The `Makefile` already exports this GOROOT — `make test`, `make build` etc. work without the prefix.

## Common Commands

```bash
# Run all tests
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go test ./...

# Run a single test
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec go test ./internal/webhook/... -run TestHandlerDeniesPrivilegedPod -v

# Build both binaries
make build   # outputs bin/webhook and bin/operator

# Lint
make lint    # runs go vet

# Helm lint
helm lint charts/kubesentry
```

## Architecture

Two independent Go binaries communicate only through Kubernetes CRDs:

**`cmd/webhook`** — Data plane. Serves `/validate` (admission), `/healthz`, `/readyz`.
- At startup, builds a controller-runtime informer cache watching `Policy` CRDs and populates an in-memory `PolicyCache`.
- On each admission request, `Handler` calls `PolicyCache.MatchingPolicies()` to get relevant compiled policies, then evaluates them concurrently with a 5s timeout.
- `enforce` mode blocks on denial; `audit` mode allows but returns violations as `AdmissionResponse.Warnings`.
- Violation messages are formatted as `[group/key] message\n  描述：description` using `formatViolation` in `handler.go`.

**`cmd/operator`** — Management plane. Also handles a `tls-setup` subcommand (run as a Helm pre-install Job).
- `PolicyGroupReconciler`: watches `PolicyGroup` CRDs, creates/updates/deletes child `Policy` objects. Reads built-in Rego from `internal/builtins.Library`. Skips creation if a same-key `Policy` with no `OwnerReference` to this group exists (custom policy wins). Updates `PolicyGroup.Status` with active/skipped counts and conditions.
- `PolicyReconciler`: validates Rego on create/update, creates an immutable `PolicyVersion` snapshot, prunes old versions (configurable limit), updates `Policy.Status`. Handles rollback by patching `Policy.Spec` from a `PolicyVersion` and requeuing.
- `WebhookConfigReconciler`: lists all `Ready` policies, aggregates their match rules (deduplicated), and patches `ValidatingWebhookConfiguration.Webhooks[*].Rules`. The VWC starts with `rules: []`; the Operator fills it dynamically.
- `tls-setup` subcommand: generates an ECDSA P-256 self-signed CA + server cert, writes a TLS Secret, patches the VWC `caBundle`. Skips if the Secret already exists.

## Key Invariants

**OPA Rego policy contract** — all Rego modules must use package `kubesentry` and expose a `deny` set:
```rego
package kubesentry
deny[msg] { ... msg := "reason" }
```
`CompileRego` in `internal/webhook/evaluator.go` queries `data.kubesentry.deny`. Policies that don't follow this convention compile but always allow.

**Built-in Rego uses v0 syntax** — OPA version bundled with this project does not support `import rego.v1` or the `some x in set` iteration form. Use classic set comprehension: `set[elem]` and `_` wildcards. See `internal/builtins/rego/` for examples.

**`zz_generated.deepcopy.go` is hand-written** — `controller-gen` is incompatible with Go 1.26.2 at the versions that support our k8s v0.31 dependencies. Do not attempt to regenerate it with `make generate`; edit it manually when adding new fields to `internal/api/v1alpha1/types.go`. Key subtlety: `metav1.Time.DeepCopy()` returns `*metav1.Time`, so `out.LastSyncTime = t` (not `&t`).

**PolicyVersion names** follow the pattern `{policy-name}-v{version}` and carry labels `kubesentry/policy` and `kubesentry/version` used for label-selector queries during rollback and pruning.

**Policy labels set by PolicyGroupReconciler** — child `Policy` objects carry three labels:
- `kubesentry.io/key` — the policy key (camelCase, e.g. `runAsPrivileged`)
- `kubesentry.io/group` — the PolicyGroup name
- `kubesentry.io/source` — `builtin` or `custom`

These are read by `syncPolicy` in `cmd/webhook/main.go` to populate `CompiledPolicy.Key`, `.GroupName`, and `.Description`.

**Conflict detection uses `OwnerReference`** — `PolicyGroupReconciler.reconcilePolicy` lists existing Policies by `kubesentry.io/key` label. If the found Policy has no `OwnerReference` pointing to the current `PolicyGroup`, it's treated as a custom override and the reconciler skips creation (returns `skipped=true`).

**CamelToKebab naming** — Policy names are derived from their key via `operator.CamelToKebab`: `runAsPrivileged` → `run-as-privileged`. This is the Kubernetes object name for the child Policy.

**PolicyCache is the single source of truth for the webhook** — the operator does not communicate with the webhook directly. The webhook's informer cache reacts to `Policy.Status.Phase` changes: `PhaseInvalid` policies are removed from the cache; `PhaseReady` policies are compiled and cached.

## Package Layout

```
internal/api/v1alpha1/   CRD types + scheme registration + hand-written DeepCopy
internal/builtins/       embedded Rego library (library.go + rego/*.rego, 37 built-in policies)
internal/webhook/        cache.go, evaluator.go, handler.go, server.go
internal/operator/       policy_reconciler.go, policygroup_reconciler.go, webhookconfig_reconciler.go
internal/tlssetup/       ECDSA cert generation (no k8s dependency)
cmd/webhook/             main: informer setup, cache sync, server start
cmd/operator/            main: manager setup, reconciler registration, tls-setup subcommand
charts/kubesentry/       Helm chart (CRDs in crds/, templates split by component)
```

## Testing Patterns

- Webhook tests use `stubStore` (in `handler_test.go`) implementing `PolicyStore` — no fake k8s client needed.
- `denyPrivilegedRego` is declared in `evaluator_test.go` and shared across all files in the `webhook_test` package.
- Operator tests use `sigs.k8s.io/controller-runtime/pkg/client/fake` with `WithStatusSubresource` to enable status updates.
- `buildScheme()` in `policy_reconciler_test.go` is shared across all files in the `operator_test` package — do not redeclare it.
- `admissionregv1.AddToScheme` must be added to the scheme in `webhookconfig_reconciler_test.go`; the base `buildScheme()` only registers CRD types.
- `runtime.RawExtension` lives in `k8s.io/apimachinery/pkg/runtime`, not in the admission package.
- Builtins tests (`internal/builtins/library_test.go`) call `webhook.CompileRego` to verify all 37 Rego files compile; if adding a new built-in policy, ensure its Rego uses v0 syntax.

## IDE Diagnostics Note

The gopls LSP may report false-positive "undefined" errors for symbols defined in the same compilation (e.g. after a `go build` succeeds). Always verify with `go build ./...` or `go test ./...` — if the compiler is happy, the IDE error is a stale cache and can be ignored.

## Branching & Release Rules

These rules apply to every git operation in this repository.

**Branches:**
- `main` — source of truth; every fix and feature must land here
- `release-x.y.z` — cut from `main` at release time; accepts hotfix cherry-picks only
- `feat/<name>` — feature work; rebase onto `main`, delete after landing
- `fix/<name>` — bug fixes; rebase onto `main`, delete after landing
- `hotfix/<name>` — patches for old releases; land in `main` first, then cherry-pick

**Rebase only — no merge commits:**
- Always `git rebase origin/main` before landing a branch
- Use `--force-with-lease` when pushing rebased branches; never bare `--force`
- Never force-push `main`

**Hotfix rule:** A hotfix commit must be in `main` before it is cherry-picked to any release branch. Never patch a release branch without backporting.

**Image tag policy:**
- `main` `values.yaml` always has `tag: "latest"` — never change to a specific version
- On release branch only: pin both image tags to `"vx.y.z"`, commit as `chore(helm): pin image tags to vx.y.z for release` — this commit must NOT be cherry-picked to `main`

**Release checklist (run on `main`):**
1. `make test-all` passes (unit tests + e2e — `make release` enforces this automatically)
2. Bump `charts/kubesentry/Chart.yaml` `version` + `appVersion`, commit
3. `git tag vx.y.z && git push origin main && git push origin vx.y.z`
4. `make release VERSION=vx.y.z`
5. `git checkout -b release-x.y.z vx.y.z && git push origin release-x.y.z`
6. On release branch: set both `values.yaml` image tags to `"vx.y.z"`, commit `chore(helm): pin image tags to vx.y.z for release`, push
7. `gh release create vx.y.z --title "vx.y.z" --target main --notes-file <notes> --latest`

**Version bumps:** PATCH = bugfix only · MINOR = new feature · MAJOR = breaking change or milestone
