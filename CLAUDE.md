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

# Build all five binaries
make build   # outputs bin/webhook, bin/operator, bin/console, bin/gateway and bin/db

# Lint
make lint    # runs go vet

# Helm lint
helm lint charts/kubesentry-gateway charts/kubesentry

# Frontend dev server (vite proxies /api to :8080; run cmd/console separately)
cd web && npm run dev
```

## Architecture

Five independent Go binaries split across two planes that never talk to each other directly:

- **Member-cluster plane** (`cmd/webhook`, `cmd/operator`) — unchanged by the gateway/database migration described below. Components communicate only through Kubernetes CRDs (`Policy`, `PolicyGroup`, `PolicyException`, `PolicyVersion`) installed by `charts/kubesentry`.
- **Gateway plane** (`cmd/gateway`, `cmd/console`, `cmd/db`) — the "中台"/management layer. Holds **no CRDs of its own**. All business data (clusters, policies, groups, exceptions, distributions) lives in a shared PostgreSQL database, accessed via GORM repositories in `internal/database/repository`. The member-cluster plane and the gateway plane are bridged only by `internal/operator/hublink` (member side) talking gRPC to `cmd/gateway` (gateway side) — see the `gatewaypb.Hub` service below.

**`cmd/webhook`** — Data plane, runs on every member cluster. Serves `/validate` (admission), `/healthz`, `/readyz`.
- At startup, builds a controller-runtime informer cache watching `Policy`, `PolicyGroup`, `Namespace`, and `PolicyException` CRDs.
- On each admission request, `Handler` calls `PolicyCache.MatchingForRequest()` which filters enabled `PolicyGroup`s by their `namespaceSelector`, then gathers the policies from matching groups and computes the strictest effective mode across all groups.
- `enforce` mode blocks on denial; `audit` mode allows but returns violations as `AdmissionResponse.Warnings`.
- Violation messages are formatted as `[<policyName> via <group1,group2>] message\n  描述：description` using `formatViolation` in `handler.go`.

**`cmd/operator`** — Management plane, runs on every member cluster. Also handles a `tls-setup` subcommand (run as a Helm pre-install Job).
- `PolicyGroupReconciler`, `PolicyReconciler`, `WebhookConfigReconciler`, `ExceptionReconciler` — all CRD-based, all unchanged by the gateway/database migration (see the Key Invariants below, which still apply verbatim to this plane).
- `tls-setup` subcommand: generates an ECDSA P-256 self-signed CA + server cert, writes a TLS Secret, patches the VWC `caBundle`. Skips if the Secret already exists.
- When `GATEWAY_ADDRESS` is set, the operator additionally starts `internal/operator/hublink` as a manager `Runnable`: this is the member-cluster side of the gateway connection — one-time `Register`, then a long-lived `Sync` stream (heartbeats + status reports up, `DesiredState` snapshots down) with exponential-backoff reconnect. `hublink/apply.go` applies the received `Policy`/`PolicyGroup` `ManagedObject`s as CRs on the member cluster, pruning ones no longer in the snapshot. Single-cluster installs simply never set `GATEWAY_ADDRESS`, so `hublink` never starts — that omission is the entire single-cluster compatibility story.

**`cmd/gateway`** — Gateway plane, PostgreSQL-backed via GORM (`internal/database/model`, `internal/database/repository`). Implements the `gatewaypb.Hub` gRPC service (`api/proto/gateway/v1/gateway.proto`, generated into `internal/gatewaypb`):
- `Register`/`Sync` — member-cluster registration and the long-lived bidirectional stream described above. `Sync`'s `pushLoop` (`internal/gateway/server.go`) renders each connected cluster's `DesiredState` snapshot from the DB (`internal/gateway/render.go`) and resends it whenever `Notifier` fires (a real content change) or a 60s resync ticker elapses (drift correction), whichever comes first.
- `NotifyChange` — fire-and-forget: wakes every connected cluster's `pushLoop` to re-render. Carries an optional `trace_id` (spec D18) that rides the next `DesiredState` push and ends up as the `kubesentry.io/last-trace-id` annotation on the member CR `hublink/apply.go` applies.
- `Dispatch` — a dumb point-to-point relay: forwards an `Envelope` payload to the named cluster's live `Sync` session and waits up to 10s for a correlated reply. No destination resolution, no offline queueing, and nothing calls it yet — see "Known Follow-ups" below.
- `Connect` — a separate registration stream for non-cluster components (today: `console-api`), authenticated with a shared static token (`GATEWAY_SHARED_TOKEN`) rather than the cluster `Register` flow. See "Known Follow-ups" — this token is not wired into any Helm values/Secret yet, so `Connect` always fails in every environment.
- Runs its own `StatusFlusher` (`internal/gateway/heartbeats.go`) which periodically flushes in-memory heartbeat/status state into the `clusters` table and flips clusters to Offline after a missed-heartbeat threshold.

**`cmd/db`** — schema migration runner (`internal/database/migrate`, golang-migrate under the hood), invoked as a Helm `post-install,pre-upgrade` hook Job (`db migrate`). Also retains a `db register-cluster --name=<name>` one-off subcommand for CLI/dev seeding. Normal cluster registration is now available through console-api and the web UI, which return a complete one-time Helm install command and persist its non-secret install parameters in `cluster_install_records`.

**`cmd/console`** — split into two containers by the Helm chart (`charts/kubesentry-gateway/templates/console/deployment.yaml`), both from the same Go binary/build:
- **`api`** — the REST API only (no embedded frontend). Handlers for Policy/PolicyGroup/PolicyException/Cluster/PolicyDistribution/summary (`internal/console/handlers_*.go`) read/write the same Postgres tables as `cmd/gateway`, via the same `gateway.Repos` GORM bundle. Cluster create/token-reissue routes generate complete Helm install commands; install-record routes expose the saved non-secret parameters. The only remaining live-cluster dependency is a `discovery.DiscoveryInterface` used solely by `handlers_schema.go` for the visual policy builder's OpenAPI field lookups (`ServerResourcesForGroupVersion` + `/openapi/v3`) — every other handler is DB-backed, there is no controller-runtime manager or informer cache in this binary at all.
  - `RevalidateAllPolicies` runs once at startup: a defense against someone editing the database directly and bypassing the API's synchronous Rego validation.
  - When `GATEWAY_ADDRESS` is set, `console.RunGatewayLink` maintains a `Connect` stream to the gateway (soft dependency — retries forever, never blocks HTTP readiness) and write handlers call `NotifyChange` best-effort so the gateway's `pushLoop` doesn't have to wait for its 60s resync.
  - Reuses `webhook.CompileRego` for client-side pre-validation before a Policy is created or updated.
  - Rollback is driven the same way the operator expects it: setting `spec.rollbackTo` plus a logical-cursor annotation (`kubesentry.io/logical-cursor`) that lets the UI track prev/next position across an in-flight rollback without re-deriving it from version history each time.
- **`web`** — nginx serving the built Vue SPA (`web/dist`), proxying `/api/*` and `/healthz` to the `api` container over localhost within the same pod. `Dockerfile.console-web` contains only the SPA + nginx runtime; the gateway chart supplies nginx's runtime configuration through `templates/console/nginx-configmap.yaml`. Console uses database-backed local users and HttpOnly SameSite=Strict sessions; access is available through NodePort or port-forward.

Console bootstraps a passwordless `admin` system administrator in the database. The first caller to `POST /api/v1/setup/admin` atomically sets its password; deployments must restrict network access until setup is complete. System administrators are not tenant-bound, while tenant administrators and viewers belong to exactly one tenant. Authorization is enforced by HTTP middleware and resource services, not by the SPA. Identity state lives in `kubesentry.tenants`, `kubesentry.users`, and `kubesentry.sessions`; only session token hashes are stored. Session absolute/idle durations and Secure Cookie behavior are configured through `console.session` in the gateway chart.

**Chart deploy ordering** (`charts/kubesentry-gateway`) is initContainer-enforced, not a Helm hook chain: the Postgres `StatefulSet` comes up first, the `kubesentry-db-migrate` Job runs as a `post-install,pre-upgrade` hook, the gateway Deployment's `wait-for-migration` initContainer polls `kubectl wait --for=condition=complete job/kubesentry-db-migrate`, and the console Deployment's `wait-for-gateway` initContainer polls the gateway's gRPC port with `nc -z` before either container starts.

## Key Invariants

All invariants in this section describe the **member-cluster plane** (`internal/webhook`, `internal/operator`, `charts/kubesentry`) and are untouched by the gateway/database migration — every Policy/PolicyGroup/PolicyException/PolicyVersion CRD, reconciler, and webhook behavior described below still works exactly as written. See "Gateway Plane Invariants" further down for the Postgres-backed side.

**OPA Rego policy contract** — all Rego modules must use package `kubesentry` and expose a `deny` set:
```rego
package kubesentry
deny[msg] { ... msg := "reason" }
```
`CompileRego` in `internal/webhook/evaluator.go` queries `data.kubesentry.deny`. Policies that don't follow this convention compile but always allow.

**Built-in Rego uses v0 syntax** — OPA version bundled with this project does not support `import rego.v1` or the `some x in set` iteration form. Use classic set comprehension: `set[elem]` and `_` wildcards. See `charts/kubesentry/builtin-policies/policies/` for examples.

**`zz_generated.deepcopy.go` is hand-written** — `controller-gen` is incompatible with Go 1.26.2 at the versions that support our k8s v0.31 dependencies. Do not attempt to regenerate it with `make generate`; edit it manually when adding new fields to `api/kubesentry/v1alpha1/types.go`. Key subtlety: `metav1.Time.DeepCopy()` returns `*metav1.Time`, so `out.LastSyncTime = t` (not `&t`).

**PolicyVersion names** follow the pattern `{policy-name}-v{version}` and carry labels `kubesentry/policy` and `kubesentry/version` used for label-selector queries during rollback and pruning.

**Policy is a first-class top-level object.** PolicyGroup does NOT own or create Policy CRs. The Helm chart creates 37 built-in Policy CRs + 3 PolicyGroup CRs side by side; the operator only computes membership and effective mode. Deleting a PolicyGroup leaves its referenced Policies intact.

**Per-request strictest mode.** When a single admission request is covered by multiple enabled PolicyGroups whose namespaceSelectors match, and they reference the same Policy with different effective modes, the webhook takes the strictest (enforce > audit). Groups whose namespaceSelector does not match the request namespace do not participate in the mode calculation.

**Built-in catalogue lives in the chart.** `charts/kubesentry/builtin-policies/policies/*.yaml` (37 files) + `charts/kubesentry/builtin-policies/groups/*.yaml` (3 files) are the single source of truth for built-in policies. `internal/builtins` Go package no longer exists. Adding a new built-in: write one policy yaml + add the kebab-name to the relevant group's `spec.members` list.

**PolicyCache is the single source of truth for the webhook** — the operator does not communicate with the webhook directly. The webhook's informer cache reacts to `Policy.Status.Phase` changes: `PhaseInvalid` policies are removed from the cache; `PhaseReady` policies are compiled and cached. `PolicyGroup.status.resolvedPolicies` drives `CompiledGroup.Members` which determines which policies are evaluated per request.

### Gateway Plane Invariants

**Gateway/console hold zero CRD lifecycle responsibilities.** Their `Policy`, `PolicyGroup`, `PolicyException`, `Cluster`, and `PolicyDistribution` records are plain Postgres tables (`internal/database/migrate/migrations/0001_init_schema.up.sql`) accessed through `internal/database/repository`. Gateway/console may reuse versioned schema/spec types from the public `api/kubesentry/v1alpha1` package, but Kubernetes clients, Scheme ownership, reconcilers, and CRD lifecycle remain in the member plane. Gateway-only `Cluster` and `PolicyDistribution` payload models live in `internal/gatewaymodel` and are not CRDs.

**Snowflake IDs, not UUIDs or serials.** `internal/database/model.Init(nodeID)` must run once per process before any repo touches the DB (every `cmd/gateway`, `cmd/console`, `cmd/db` `main.go` calls it with `model.NodeIDFromPodName(os.Getenv("POD_NAME"))`). Forgetting this panics or produces zero-value IDs on insert.

**Auth model is two-tier.** Member clusters use a one-time `RegTokenHash` (issued at `Cluster` row creation, consumed by `Register`) then a long-lived `AuthTokenHash` (issued by `Register`, used by every `Sync` call) — gateway only ever stores SHA-256 hashes, never plaintext, and compares with `crypto/subtle.ConstantTimeCompare`. Non-cluster components (`console-api`) instead present a single shared static token (`GATEWAY_SHARED_TOKEN`) on `Connect` — see "Known Follow-ups", this token is currently unconfigured everywhere.

**`trace_id` only rides a genuine change.** `internal/gateway/server.go`'s `pushLoop` only attaches a `trace_id` to an outgoing `DesiredState` when the wakeup came from a real `Notify()` (not the 60s resync ticker, not the initial connect send) *and* that specific cluster's rendered content hash actually changed — a ticker-forced resend or another cluster's unrelated change must never carry a stale trace_id.

## Package Layout

```
api/kubesentry/v1alpha1/ public member CRD Go API + hand-written DeepCopy; must not import internal packages
internal/webhook/        cache.go, evaluator.go, handler.go, server.go (PolicyCache holds CompiledPolicy + CompiledGroup) — member plane, unchanged
internal/operator/       policy_reconciler.go, policygroup_reconciler.go, webhookconfig_reconciler.go, exception_reconciler.go — member plane, unchanged
internal/operator/hublink/  member-side gateway client: Register + Sync loop, apply.go renders received ManagedObjects onto member CRs
internal/tlssetup/       ECDSA cert generation (no k8s dependency), shared by operator tls-setup and cmd/gateway's default TLS bundle
internal/database/model/ Snowflake ID generator + GORM base model (CreatedAt/UpdatedAt/ID conventions shared by every repository model)
internal/database/migrate/      golang-migrate wrapper + migrations/*.sql (schema + seed data)
internal/database/repository/ GORM repositories: cluster_repo.go, policy_repo.go, group_repo.go, exception_repo.go, distribution_repo.go, install_record_repo.go
internal/gatewaypb/      generated from api/proto/gateway/v1/gateway.proto (Register/Sync/Connect/NotifyChange/Dispatch)
internal/gateway/        gateway.Server (gatewaypb.Hub impl), render.go (DB rows -> DesiredState), notifier.go, registry.go (session/connect registries), heartbeats.go (StatusFlusher)
internal/groupresolve/   PolicyGroup membership resolution (byName/bySelector/exclude), shared by the gateway render path
internal/console/        REST handlers (policy/group/exception/summary/schema), gatewaylink.go (Connect client), startup_validate.go, cursor.go, response.go, server.go
cmd/webhook/             main: informer setup (Policy + PolicyGroup + Namespace + PolicyException), cache sync, server start — member plane
cmd/operator/            main: manager setup, reconciler registration, tls-setup subcommand, optional hublink Runnable when GATEWAY_ADDRESS is set
cmd/gateway/             main: GORM DB open, model.Init, gRPC server on GRPC_ADDR, plain-HTTP health server on :8081, StatusFlusher
cmd/console/             main: GORM DB open, discovery client (schema handler only), optional Connect link to gateway, HTTP server (api container only, no embedded frontend)
cmd/db/                  main: `db migrate` (Helm hook Job) and `db register-cluster --name=<name>` (dev/deploy-local seeding)
web/                     Vue 3 + TypeScript SPA (Element Plus, vue-i18n, vue-router), built to web/dist and served by the console chart's nginx `web` container (no longer embedded into the Go binary)
Dockerfile.console-web    Multi-stage SPA build + nginx runtime image; nginx server configuration is injected by the gateway chart
charts/kubesentry/       Member-cluster Helm chart (webhook, operator, CRDs, built-in policies) — unchanged by this migration
  ├── crds/              CRD manifests
  ├── builtin-policies/  source-of-truth for built-in Policies + PolicyGroups
  │   ├── policies/      37 standalone Policy CRs (one yaml per policy)
  │   └── groups/        3 PolicyGroup member manifests (intermediate data, not full k8s objects)
  └── templates/         Helm renderers (builtin-policies.yaml + builtin-groups.yaml glob those folders)
charts/kubesentry-gateway/  Gateway-plane Helm chart (gateway, console web+api, Postgres, migration Job)
  ├── templates/postgres/     StatefulSet + Service + credentials Secret
  ├── templates/db-migrate-job.yaml  post-install,pre-upgrade hook Job running `db migrate`
  ├── templates/deployment.yaml      gateway Deployment (wait-for-migration initContainer)
  └── templates/console/     console nginx ConfigMap + Deployment (web+api containers, wait-for-gateway initContainer) + its own rbac.yaml/service.yaml
```

## Testing Patterns

- Webhook tests use `stubStore` (in `handler_test.go`) implementing `PolicyStore` — no fake k8s client needed.
- `denyPrivilegedRego` is declared in `evaluator_test.go` and shared across all files in the `webhook_test` package.
- Operator tests use `sigs.k8s.io/controller-runtime/pkg/client/fake` with `WithStatusSubresource` to enable status updates.
- `buildScheme()` in `policy_reconciler_test.go` is shared across all files in the `operator_test` package — do not redeclare it.
- `admissionregv1.AddToScheme` must be added to the scheme in `webhookconfig_reconciler_test.go`; the base `buildScheme()` only registers CRD types.
- `runtime.RawExtension` lives in `k8s.io/apimachinery/pkg/runtime`, not in the admission package.
- Builtin compile test (`test/builtins/compile_test.go`) runs `helm template` and calls `webhook.CompileRego` on every rendered Policy CR; asserts ≥37 policies. When adding a new built-in policy, ensure its Rego uses v0 syntax and add it to the group's `spec.members`.
- **Gateway-plane DB tests use testcontainers, not fakes.** `internal/database/repository`, `internal/database/migrate`, `internal/gateway`, and `internal/console`'s GORM-backed handler tests each spin up a throwaway `postgres:16-alpine` container via `github.com/testcontainers/testcontainers-go`, run real migrations (`dbmigrate.Migrate`), and exercise the real GORM repos — see `newTestDB` in `internal/database/repository/testhelper_test.go` (the canonical copy; `internal/gateway/testhelper_test.go` and `internal/console/testhelper_db_test.go` are near-identical unexported copies since Go test helpers can't be shared across package boundaries). This makes these packages slow and Docker-dependent. The repository `make test` and `make test-report` targets deliberately use Go package parallelism `-p 1` so a full run does not create dozens of PostgreSQL containers concurrently; override `TEST_PACKAGE_PARALLELISM` only when the Docker daemon has sufficient capacity.
- Console handler tests are split by backing: `newTestServer`/`doRequest` in `internal/console/handlers_test.go` back the schema handler with a fake `schemaFetcher` (no DB needed); `newDBTestServer`/`newDBTestServerWithHandlers` in `internal/console/testhelper_db_test.go` back the policy/group/exception/summary handlers with a real testcontainers Postgres via `gateway.Repos`. Both are white-box `package console` (not `console_test`).
- e2e (`test/e2e`, build-tagged `e2e`) is driven through console-api: `t9_console_test.go` port-forwards `svc/kubesentry-console` (port 8080, fronted by the `web` nginx container) and exercises the REST API end-to-end. `t11_distribution_test.go` creates the gateway-side Policy/PolicyGroup/PolicyDistribution rows through those routes, then verifies render/push, member-side managed-object materialization and pruning, cluster liveness, and reserved `default` delete protection.

## Known Follow-ups

**`Dispatch` is not yet connected to a product-facing workflow.** The gateway
implements a point-to-point relay for a named online cluster, but no console or
operator feature invokes it. It deliberately has no destination resolution or
offline queue. Policy synchronization and cluster uninstallation use their own
desired-state/task paths and do not depend on `Dispatch`; a future imperative
cluster-command feature should define routing, durability, retries, and UI/API
semantics before using it.

## IDE Diagnostics Note

The gopls LSP may report false-positive "undefined" errors for symbols defined in the same compilation (e.g. after a `go build` succeeds). Always verify with `go build ./...` or `go test ./...` — if the compiler is happy, the IDE error is a stale cache and can be ignored.

## Documentation Maintenance

Documentation is a required deliverable, not a follow-up. Apply the matrix in
`docs/zh-CN/documentation-maintenance.md` (English mirror:
`docs/en-US/documentation-maintenance.md`) whenever changing user-visible
features, permissions, APIs, components, data/control flows, Helm values,
ports, network direction, installation, upgrade, uninstall, or built-in rules.

- Chinese is the primary documentation language; update the English mirror in
  the same change.
- User-facing behavior must update the authenticated Help Center and both
  locales.
- Architecture changes must update the editable `.drawio` source and its SVG.
- Network or Helm changes must update both READMEs, the network guide, Help
  Center, and the relevant chart README.
- Verify links, locale key parity, frontend tests/build, and Helm lint before
  completion. Do not include real credentials or environment-specific secrets.

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
2. **Fresh-install verification (MANDATORY, has bitten us more than once):** on the local cluster run the full uninstall → clean install cycle against the packaged chart, and confirm all pods become Ready and the 37 builtin policies reach `Ready`:
   ```bash
   make undeploy-local        # full teardown incl. CRDs
   make helm-package VERSION=vx.y.z
   helm install kubesentry dist/kubesentry-x.y.z.tgz \
     --namespace kubesentry-system --create-namespace --wait --timeout 5m
   kubectl get policies | grep -c Ready   # expect 37
   make undeploy-local        # clean up again
   ```
   This catches packaging-only failures that e2e helpers can mask (e.g. the v1.1.0 chart shipped with CRDs under `templates/`, which passes e2e but breaks every fresh `helm install`).
3. Bump `charts/kubesentry/Chart.yaml` `version` + `appVersion`, commit
4. `git tag vx.y.z && git push origin main && git push origin vx.y.z`
5. `make release VERSION=vx.y.z`
6. `git checkout -b release-x.y.z vx.y.z && git push origin release-x.y.z`
7. On release branch: set both `values.yaml` image tags to `"vx.y.z"`, commit `chore(helm): pin image tags to vx.y.z for release`, push
8. `gh release create vx.y.z --title "vx.y.z" --target main --notes-file <notes> --latest` — **LAST step, only after every verification above has passed.**

**GitHub Immutable Releases (hard platform constraint):** this repo has immutable releases enabled. Once `gh release create vx.y.z` publishes, that tag name is burned forever — deleting the release/tag does NOT free the name, and it can never be recreated (learned the hard way with v1.1.0, which is permanently unusable). Therefore: never publish the GitHub Release until the tag is final and fully verified; if a defect is found after publishing, the only path is a new version number.

**Version bumps:** PATCH = bugfix only · MINOR = new feature · MAJOR = breaking change or milestone
