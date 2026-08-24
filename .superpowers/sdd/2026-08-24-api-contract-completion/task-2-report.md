# Task 2: browser API contract completion

## Delivered

- Replaced the browser API wrapper's arbitrary method/path requests and handwritten transport DTOs with `openapi-fetch` calls whose request and response types derive from generated `paths`, `operations`, and `components` types.
- Kept view-facing exports as generated-type aliases, retaining the existing resource function names and wire behavior.
- Preserved hostile-response validation in `summary.ts` and its tests; generated TypeScript types do not replace runtime normalization.
- Kept legacy admission-filter call sites compatible by splitting comma-separated values at the API-resource boundary before submitting generated array query parameters.

## TDD evidence

Before removing the old generic `request(method, url)` wrapper, the frontend type build failed because an expected error for `request('GET', '/not-in-the-contract')` was unused. That demonstrated arbitrary paths were accepted. The replacement test now verifies that the typed generated client rejects the same path at compile time.

## Minimal contract corrections

- `PolicyGroup.policies.bySelector` now uses `KubernetesLabelSelector`.
- `PolicyVersionEntry` restores `phase`; `PolicyVersionList` restores `cursor`, `head`, `inFlight`, `prevEnabled`, and `nextEnabled`, matching the current service response.
- `ClusterPhase` includes `Uninstalled`.
- Admission event CSV filters `cluster`, `namespace`, and `gvk` are explicit form-style, non-exploded arrays; finite `verdict` values are an array of the `audit`/`deny` enum.

## Verification

- `make api-gen` — passed.
- `make api-lint` — passed: 50 console and 2 gateway operations checked; TypeScript generated.
- `cd web && npm run build` — passed.
- `cd web && npm test` — passed: 16 files, 101 tests.
- `go test ./internal/console/...` remains an ordered Task 3 dependency. Before the schema correction, the package already failed because handlers still refer to removed explicit DTO symbols such as `dto.CreateClusterReq`, `dto.UpdateClusterReq`, and `dto.UpdatePlatformSettingsReq`. The admission schema correction additionally requires `internal/console/handler/admission.go` to replace `params.Cluster.Or("")`, `params.Namespace.Or("")`, and `params.Gvk.Or("")` with direct generated array handling. Task 3 owns that handler migration.

## Scope notes

The pre-existing untracked `docs/api-prompt.txt` and `scripts/__pycache__/` were preserved and are not part of this task.
