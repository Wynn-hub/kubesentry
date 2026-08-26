# API Migration Report

## Result

The Console migration is complete at 50 OpenAPI-defined `/api/v1` operations. Ogen generates `internal/console/dto`; `openapi-typescript` generates `web/src/api/generated/console-api.ts`; generated routing enters typed Console handlers and services.

## Explicit exclusions

| Boundary | Kept as | Evidence |
| --- | --- | --- |
| Gateway member/console transport | protobuf/gRPC | `api/proto/gateway/v1/gateway.proto`, 5 `Hub` RPCs |
| Member policy resources | Kubernetes CRDs | four files under `charts/kubesentry/crds/` |
| Gateway HTTP probes | operational endpoints | no public OpenAPI index remains |

`api/internal/gateway.yaml` was removed because it had no tracked runtime consumer and duplicated a protobuf-owned boundary.

## Coverage and drift controls

- The contract checker requires 50 unique Console operation IDs, validates metadata, schemas and references, and compares them with generated Ogen operations plus concrete handler methods.
- It validates the five Gateway RPCs and four CRD names against the unified index. `protoc` parses the wire contract and Helm renders CRDs in `api-lint`.
- `make api-lint` uses scratch paths for Go and TypeScript generation and fails on differences. It also checks the embedded OpenAPI bundle without rewriting the checkout.

## Frontend outcome and risk

All browser resource modules use generated `components` and `operations` through the shared typed transport. Handwritten code is limited to established runtime normalization and CSV adaptation. The migration does not add Console authentication, a REST facade for gRPC streams, or new CRDs; each needs a separate product/security decision.

## Required evidence

Before integration run `make api-lint`, Go format/vet/tests/build, frontend test/build, Helm lint/template, `git diff --check`, and `git status --short`.
