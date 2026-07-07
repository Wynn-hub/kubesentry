# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Go Toolchain Warning

This machine has a Go toolchain version mismatch. The `go` binary is Homebrew-managed but `~/.zshrc` sets `GOROOT` to a different path. **Always prefix `go` commands with the correct GOROOT:**

```bash
GOROOT=/opt/homebrew/opt/go/libexec go <command>
```

The `Makefile` already exports this GOROOT — `make test`, `make build` etc. work without the prefix.

## Common Commands

```bash
# Run all tests
GOROOT=/opt/homebrew/opt/go/libexec go test ./...

# Run a single test
GOROOT=/opt/homebrew/opt/go/libexec go test ./internal/webhook/... -run TestHandlerDeniesPrivilegedPod -v

# Build both binaries
make build   # outputs bin/webhook, bin/operator and bin/console

# Lint
make lint    # runs go vet

# Helm lint
helm lint charts/kubesentry

# Frontend dev server (vite proxies /api to :8080; run cmd/console separately)
cd web && npm run dev
```

## Architecture

Three independent Go binaries communicate only through Kubernetes CRDs:

**`cmd/webhook`** — Data plane. Serves `/validate` (admission), `/healthz`, `/readyz`.
- At startup, builds a controller-runtime informer cache watching `Policy`, `PolicyGroup`, `Namespace`, and `PolicyException` CRDs.
- On each admission request, `Handler` calls `PolicyCache.MatchingForRequest()` which filters enabled `PolicyGroup`s by their `namespaceSelector`, then gathers the policies from matching groups and computes the strictest effective mode across all groups.
- `enforce` mode blocks on denial; `audit` mode allows but returns violations as `AdmissionResponse.Warnings`.
- Violation messages are formatted as `[<policyName> via <group1,group2>] message\n  描述：description` using `formatViolation` in `handler.go`.

**`cmd/operator`** — Management plane. Also handles a `tls-setup` subcommand (run as a Helm pre-install Job).
- `PolicyGroupReconciler`: watches `PolicyGroup` CRDs and ALL `Policy` CRDs (reverse-enqueue via MapFunc on Policy create/delete/label-change). Resolves group members from `spec.policies.byName` + `spec.policies.bySelector` (byName precedence). Computes effective enforcementMode per member (per-entry override on byName, group-level `selectorEnforcementMode` on bySelector). Writes `status.resolvedPolicies` + `status.phase` (Ready / Degraded / Disabled). Maintains `Policy.status.referencedBy` by diffing previous-resolved ∪ current-resolved.
- `PolicyReconciler`: validates Rego on create/update, creates an immutable `PolicyVersion` snapshot, prunes old versions (configurable limit), updates `Policy.Status`. Handles rollback via `spec.rollbackTo`. **No longer aware of "builtin" — every Policy is a first-class user-or-Helm-managed object.**
- `WebhookConfigReconciler`: lists all `Ready` policies (regardless of group membership), aggregates their `match.resources`, patches `ValidatingWebhookConfiguration.Webhooks[*].Rules`.
- `ExceptionReconciler`: validates `PolicyException` lifecycle. `policyGroupRefs` is dynamic — exemption follows `PolicyGroup.status.resolvedPolicies` (no schema change).
- `tls-setup` subcommand: generates an ECDSA P-256 self-signed CA + server cert, writes a TLS Secret, patches the VWC `caBundle`. Skips if the Secret already exists.

**`cmd/console`** — Management/UI plane. REST API over the same CRDs (Policy, PolicyGroup, PolicyException, PolicyVersion) plus an embedded Vue SPA, served on one port, accessed via port-forward with no auth.
- Reuses `webhook.CompileRego` for client-side pre-validation before a Policy is created or updated.
- Rollback is driven the same way the operator expects it: setting `spec.rollbackTo` plus a logical-cursor annotation (`kubesentry.io/logical-cursor`) that lets the UI track prev/next position across an in-flight rollback without re-deriving it from version history each time.

## Key Invariants

**OPA Rego policy contract** — all Rego modules must use package `kubesentry` and expose a `deny` set:
```rego
package kubesentry
deny[msg] { ... msg := "reason" }
```
`CompileRego` in `internal/webhook/evaluator.go` queries `data.kubesentry.deny`. Policies that don't follow this convention compile but always allow.

**Built-in Rego uses v0 syntax** — OPA version bundled with this project does not support `import rego.v1` or the `some x in set` iteration form. Use classic set comprehension: `set[elem]` and `_` wildcards. See `charts/kubesentry/builtin-policies/policies/` for examples.

**`zz_generated.deepcopy.go` is hand-written** — `controller-gen` is incompatible with Go 1.26.2 at the versions that support our k8s v0.31 dependencies. Do not attempt to regenerate it with `make generate`; edit it manually when adding new fields to `internal/api/v1alpha1/types.go`. Key subtlety: `metav1.Time.DeepCopy()` returns `*metav1.Time`, so `out.LastSyncTime = t` (not `&t`).

**PolicyVersion names** follow the pattern `{policy-name}-v{version}` and carry labels `kubesentry/policy` and `kubesentry/version` used for label-selector queries during rollback and pruning.

**Policy is a first-class top-level object.** PolicyGroup does NOT own or create Policy CRs. The Helm chart creates 37 built-in Policy CRs + 3 PolicyGroup CRs side by side; the operator only computes membership and effective mode. Deleting a PolicyGroup leaves its referenced Policies intact.

**Per-request strictest mode.** When a single admission request is covered by multiple enabled PolicyGroups whose namespaceSelectors match, and they reference the same Policy with different effective modes, the webhook takes the strictest (enforce > audit). Groups whose namespaceSelector does not match the request namespace do not participate in the mode calculation.

**Built-in catalogue lives in the chart.** `charts/kubesentry/builtin-policies/policies/*.yaml` (37 files) + `charts/kubesentry/builtin-policies/groups/*.yaml` (3 files) are the single source of truth for built-in policies. `internal/builtins` Go package no longer exists. Adding a new built-in: write one policy yaml + add the kebab-name to the relevant group's `spec.members` list.

**PolicyCache is the single source of truth for the webhook** — the operator does not communicate with the webhook directly. The webhook's informer cache reacts to `Policy.Status.Phase` changes: `PhaseInvalid` policies are removed from the cache; `PhaseReady` policies are compiled and cached. `PolicyGroup.status.resolvedPolicies` drives `CompiledGroup.Members` which determines which policies are evaluated per request.

## Package Layout

```
internal/api/v1alpha1/   CRD types + scheme registration + hand-written DeepCopy
internal/webhook/        cache.go, evaluator.go, handler.go, server.go (PolicyCache holds CompiledPolicy + CompiledGroup)
internal/operator/       policy_reconciler.go, policygroup_reconciler.go, webhookconfig_reconciler.go, exception_reconciler.go
internal/tlssetup/       ECDSA cert generation (no k8s dependency)
internal/console/        REST handlers (policy/group/exception/summary), cursor.go, response.go, server.go
cmd/webhook/             main: informer setup (Policy + PolicyGroup + Namespace + PolicyException), cache sync, server start
cmd/operator/            main: manager setup, reconciler registration, tls-setup subcommand
cmd/console/             main: kubeconfig + cluster.Cluster setup, embeds web/dist, server start
web/                     Vue 3 + TypeScript SPA (Element Plus, vue-i18n, vue-router), embedded into cmd/console via embed.go
charts/kubesentry/       Helm chart
  ├── crds/              CRD manifests
  ├── builtin-policies/  source-of-truth for built-in Policies + PolicyGroups
  │   ├── policies/      37 standalone Policy CRs (one yaml per policy)
  │   └── groups/        3 PolicyGroup member manifests (intermediate data, not full k8s objects)
  └── templates/         Helm renderers (builtin-policies.yaml + builtin-groups.yaml glob those folders)
```

## Testing Patterns

- Webhook tests use `stubStore` (in `handler_test.go`) implementing `PolicyStore` — no fake k8s client needed.
- `denyPrivilegedRego` is declared in `evaluator_test.go` and shared across all files in the `webhook_test` package.
- Operator tests use `sigs.k8s.io/controller-runtime/pkg/client/fake` with `WithStatusSubresource` to enable status updates.
- `buildScheme()` in `policy_reconciler_test.go` is shared across all files in the `operator_test` package — do not redeclare it.
- `admissionregv1.AddToScheme` must be added to the scheme in `webhookconfig_reconciler_test.go`; the base `buildScheme()` only registers CRD types.
- `runtime.RawExtension` lives in `k8s.io/apimachinery/pkg/runtime`, not in the admission package.
- Builtin compile test (`test/builtins/compile_test.go`) runs `helm template` and calls `webhook.CompileRego` on every rendered Policy CR; asserts ≥37 policies. When adding a new built-in policy, ensure its Rego uses v0 syntax and add it to the group's `spec.members`.
- Console handler tests share `newTestServer`/`doRequest` helpers in `internal/console/handlers_test.go` — fake controller-runtime client with `WithStatusSubresource`, white-box `package console` (not `console_test`).
- e2e `test/e2e/t9_console_test.go` exercises the console REST API end-to-end through a `kubectl port-forward` to the deployed `kubesentry-console` Service.

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
