# API Development Rules

1. Start Console browser HTTP work in `api/console/console-api.yaml`; it alone generates Ogen DTOs and browser TypeScript for the 50 `/api/v1` operations.
2. Every Console operation needs explicit metadata and reusable request/success schemas, a generated-server operation, and a concrete `internal/console/handler` method; do not rely on `dto.UnimplementedHandler`.
3. Keep database models, API DTOs, request types, and response types separate. Handlers adapt protocols; services own business logic and repository interfaces.
4. Browser modules use generated `components` and `operations` from `web/src/api/generated/console-api.ts`. Runtime normalization may preserve legacy behavior but must not recreate resource DTOs or arbitrary paths.
5. Gateway transport stays protobuf-first at `api/proto/gateway/v1/gateway.proto`; do not add an OpenAPI facade for streaming RPCs. Gateway probes are operational, not public Console operations.
6. `Policy`, `PolicyGroup`, `PolicyException`, and `PolicyVersion` stay CRD-first under `charts/kubesentry/crds/`; Kubernetes credentials/RBAC own their access and Gateway owns zero CRDs.
7. Keep `api/openapi.yaml` as the boundary index. Update it only for an approved boundary change; do not restore `api/internal/gateway.yaml` as a competing source.
8. Run `make api-lint` before commit. It checks (never rewrites) bundle, Ogen, and TypeScript output and validates protobuf/CRD families. Use `make api-gen` or `cd web && npm run generate:api` only as deliberate regeneration steps, then review the diff.
9. For every changed operation, add contract, handler/service, and frontend coverage as applicable; run Go format/vet/tests/build, frontend test/build, Helm lint/template, and final diff checks.
