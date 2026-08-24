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

## Remaining cleanup tracked by this report

- Keep generated TypeScript output synchronized with the console contract.
- Keep the API Explorer and served OpenAPI document synchronized with the
  checked-in specification.
- Remove the compatibility bridge once the remaining typed services are
  complete.

## Verification record

The final implementation must record fresh results for Go tests, `go vet`,
frontend type-check/build, OpenAPI lint, operation coverage, and API Explorer
smoke checks in the completion summary.
