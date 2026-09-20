# API Migration Report

## Result

The Console exposes 50 `/api/v1` operations through explicit `net/http` routes. Go handler/DTO Swag annotations are the contract source; `api/swagger/swagger.yaml` and normalized OAS3 are generated derivatives. `openapi-typescript` temporarily still consumes the reviewed legacy OAS3 baseline until the frontend switch is complete.

## Explicit exclusions

| Boundary | Kept as | Evidence |
| --- | --- | --- |
| Gateway member/console transport | protobuf/gRPC | `api/proto/gateway/v1/gateway.proto`, 5 `Hub` RPCs |
| Member policy resources | Kubernetes CRDs | four files under `charts/kubesentry/crds/` |
| Gateway HTTP probes | operational endpoints | no public OpenAPI index remains |

`api/internal/gateway.yaml` was removed because it had no tracked runtime consumer and duplicated a protobuf-owned boundary.

## Coverage and drift controls

- The contract checker requires 50 unique Console operation IDs, validates metadata, schemas and references, and compares them with concrete handler annotations and runtime HTTP method/path registrations.
- It validates the five Gateway RPCs and four CRD names against the unified index. `protoc` parses the wire contract and Helm renders CRDs in `api-lint`.
- `make api-lint` checks the temporary TypeScript input and embedded OpenAPI bundle without rewriting the checkout. `make swagger-check` checks the reproducible Swag → Swagger 2 → OAS3 chain.

## Frontend outcome and risk

All browser resource modules use generated `components` and `operations` through the shared typed transport. Handwritten code is limited to established runtime normalization and CSV adaptation. The migration does not add Console authentication, a REST facade for gRPC streams, or new CRDs; each needs a separate product/security decision.

## Required evidence

Before integration run `make api-lint`, Go format/vet/tests/build, frontend test/build, Helm lint/template, `git diff --check`, and `git status --short`.
