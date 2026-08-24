# API Migration Report

## Current status

The console HTTP surface has been migrated to an Ogen-generated server and is
defined by `api/console/console-api.yaml`. Gateway member traffic remains
gRPC and is indexed by `api/internal/gateway.yaml`.

## Migrated API groups

- Policy, PolicyGroup, PolicyException
- Summary and schema discovery
- Admission events and admission-link incidents
- Platform observability
- Cluster management, install records, and uninstall tasks
- Policy distribution

## Contract and implementation checks

- OpenAPI 3.1 documents are stored below `api/`.
- Generated Go contract code is stored below `internal/console/dto`.
- Browser traffic uses the shared `/api/v1` transport.
- Kubernetes member components remain CRD/controller based.

## Service boundaries

- Keep generated TypeScript output synchronized with the console contract.
- Keep the API Explorer and served OpenAPI document synchronized with the
  checked-in specification.
- Platform observability uses a narrow in-process service façade with no HTTP
  route registration or `ResponseWriter` dependency. Cluster, install-record,
  and distribution operations use typed services directly. The generated Ogen
  server is the only HTTP router for all `/api/v1` operations.

## Verification record

The latest verification run completed successfully:

- `go test -p 1 ./... -count=1`
- `make lint` (`go vet ./...`)
- `make api-lint` (50 console operations and 2 gateway operations)
- `npm test -- --run` (102 tests)
- `npm run build` (Vue type-check and production bundle)
- API Explorer endpoint smoke tests, including `/api/docs/openapi.json` and
  `/system/api-docs`
