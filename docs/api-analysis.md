# API Contract Analysis

## Scope

This document records the API inventory used by the OpenAPI-first migration.
The control plane consists of the PostgreSQL-backed `console-api`, the
internal `gateway`, and the Vue `console-web`. The member-cluster webhook and
operator remain Kubernetes API Pattern components and are described by CRD
schemas rather than REST endpoints.

## Console API inventory

The public management API is defined in `api/console/console-api.yaml` and is
served below `/api/v1` by the generated Ogen router. The current operation
groups are:

- Cluster lifecycle, install records, token reissue, and uninstall tasks
- Policy, PolicyGroup, PolicyException, and policy version operations
- Policy distribution management
- Platform overview, trends, leadership events, and settings
- Admission events, trends, settings, and link incidents
- Summary and Kubernetes field-schema discovery

The operation IDs in the OpenAPI document are the coverage key. Generated Go
interfaces are in `internal/console/dto`; service and repository code is under
`internal/console/service` and `internal/console/repository`.

## Gateway internal inventory

The gateway's data path is the mTLS-protected gRPC `Hub` service in
`api/proto/gateway/v1/gateway.proto`: `Register`, `Sync`, `Connect`,
`NotifyChange`, and `Dispatch`. The OpenAPI-compatible index is
`api/internal/gateway.yaml`; it documents the HTTP `/healthz` and `/readyz`
probes and links the streaming RPC contract with `x-grpc-service` because
OpenAPI has no native gRPC-stream representation.

## Console-web dependencies

The SPA calls the `/api/v1` contract through the shared Axios transport in
`web/src/api/http.ts`. API models and operation wrappers are generated or
derived from the OpenAPI contract under `web/src/api/generated`; page-specific
modules re-export those types and add validation for legacy response details.

## CRD resources

The member plane owns `Policy`, `PolicyGroup`, `PolicyException`, and
`PolicyVersion` CRDs. Their schemas are under `charts/kubesentry/crds/` and
their reconcile/admission implementations remain in `internal/operator` and
`internal/webhook`.

## Migration recommendations

1. Keep the gRPC proto as the wire source of truth for streaming gateway calls.
2. Keep all browser-facing calls behind the generated OpenAPI client.
3. Remove the temporary `legacyPlatformInvoke` adapter after platform,
   cluster, install-record, and distribution services expose typed methods.
4. Run OpenAPI lint and operation coverage checks in CI before generating Go or
   TypeScript clients.
