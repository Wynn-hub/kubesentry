# API Migration Report

## Result

The Console exposes 50 `/api/v1` operations through explicit `net/http` routes. Go handler/DTO Swag annotations are the sole contract source; `api/swagger/swagger.yaml` is the generated Swagger 2 artifact and `internal/console/openapi_bundle.json` is its normalized OAS 3.0 derivative. `openapi-typescript`, Scalar, and the Console documentation endpoint consume that derived OAS3 artifact.

## Explicit exclusions

| Boundary | Kept as | Evidence |
| --- | --- | --- |
| Gateway member/console transport | protobuf/gRPC | `api/proto/gateway/v1/gateway.proto`, 5 `Hub` RPCs |
| Member policy resources | Kubernetes CRDs | four files under `charts/kubesentry/crds/` |
| Gateway HTTP probes | operational endpoints | no public OpenAPI index remains |

The member CRD Go API is public at `api/kubesentry/v1alpha1`. It owns the versioned CRD schema types and Scheme registration, while Gateway-only transport/persistence models remain in `internal/gatewaymodel`. Gateway continues to expose only protobuf/gRPC services.

`api/internal/gateway.yaml` was removed because it had no tracked runtime consumer and duplicated a protobuf-owned boundary.

## Coverage and drift controls

- The contract checker requires 50 unique Console operation IDs, validates metadata, schemas and references, and compares them with concrete handler annotations and runtime HTTP method/path registrations.
- It validates the five Gateway RPCs and four CRD names against the unified index. `protoc` parses the wire contract and Helm renders CRDs in `api-lint`.
- `make api-lint` checks the temporary TypeScript input and embedded OpenAPI bundle without rewriting the checkout. `make swagger-check` checks the reproducible Swag → Swagger 2 → OAS3 chain.
- `make lint` also runs `scripts/check_architecture_boundaries.py`, which rejects retired package paths, public API imports of repository-private implementations, and production hublink imports of Gateway/database implementations.

## Frontend outcome and risk

All browser resource modules use generated `components` and `operations` through the shared typed transport. Handwritten code is limited to established runtime normalization and CSV adaptation. Console routes now distinguish operational, public, and protected groups through an internal middleware seam, but `NewServer` supplies no authentication middleware, so current anonymous behavior is unchanged. Implementing authentication, adding a REST facade for gRPC streams, or adding new CRDs each requires a separate product/security decision.

## Required evidence

Before integration run `make api-lint`, Go format/vet/tests/build, frontend test/build, Helm lint/template, `git diff --check`, and `git status --short`.
