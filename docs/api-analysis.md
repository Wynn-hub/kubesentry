# API Contract Analysis

## Boundaries

`api/openapi.yaml` indexes three sources without turning them into one REST API:

| Family | Source of truth | Inventory | Browser HTTP |
| --- | --- | ---: | --- |
| Console | Go handler/DTO Swag annotations | 50 operations | `/api/v1` only |
| Gateway | `api/proto/gateway/v1/gateway.proto` | 5 RPCs: `Register`, `Sync`, `Connect`, `NotifyChange`, `Dispatch` | No |
| Member cluster | `charts/kubesentry/crds/{policy,policygroup,exception,policyversion}.yaml` | 4 CRDs | No |

Swag generates `api/swagger/swagger.yaml`; the normalized OAS3 document is a
reproducible derivative. The legacy `api/console/console-api.yaml` remains only
as a reviewed compatibility baseline and temporary frontend input. The obsolete
`api/internal/gateway.yaml` was removed because its probes were not a browser
contract and duplicated the protobuf streaming boundary.

## Console operation inventory

| Tag | Count | Operations |
| --- | ---: | --- |
| Cluster | 11 | `listClusters`, `createCluster`, `getCluster`, `updateCluster`, `deleteCluster`, `reissueClusterToken`, `getLatestInstallParams`, `createUninstallTask`, `listUninstallTasks`, `retryUninstallTask`, `listInstallRecords` |
| Policy | 9 | `listPolicies`, `createPolicy`, `getResourceSuggestions`, `validatePolicyRego`, `getPolicy`, `updatePolicy`, `deletePolicy`, `listPolicyVersions`, `rollbackPolicy` |
| PolicyGroup | 6 | `listPolicyGroups`, `createPolicyGroup`, `getPolicyGroup`, `updatePolicyGroup`, `deletePolicyGroup`, `setPolicyGroupEnabled` |
| Distribution | 5 | `listDistributions`, `createDistribution`, `getDistribution`, `updateDistribution`, `deleteDistribution` |
| Platform | 5 | `getPlatformOverview`, `getPlatformInstanceTrend`, `listPlatformLeadershipEvents`, `getPlatformSettings`, `updatePlatformSettings` |
| AdmissionEvent | 5 | `listAdmissionEvents`, `getAdmissionEvent`, `getAdmissionEventTrend`, `getAdmissionEventSettings`, `updateAdmissionEventSettings` |
| Exception | 5 | `listExceptions`, `createException`, `getException`, `updateException`, `deleteException` |
| LinkIncident | 2 | `listAdmissionLinkIncidents`, `getAdmissionLinkIncident` |
| Schema | 1 | `getFieldSchema` |
| Summary | 1 | `getSummary` |

The gates require exactly these 50 Swag `@ID` annotations, concrete
`internal/console/handler` methods, and matching runtime HTTP method/path
registrations.

## Console-web dependency map

`web/src/api/http.ts` owns the shared `openapi-fetch` transport, while
`web/src/api/generated/console-api.ts` owns all generated types.

| Modules | Dependency |
| --- | --- |
| `cluster`, `distribution`, `policy`, `group`, `exception` | Same-named Console tags and schemas |
| `admissionEvent`, `observability`, `platform` | AdmissionEvent, LinkIncident, Platform operations; only legacy normalization/CSV adaptation is handwritten |
| `schema`, `summary` | `getFieldSchema` and `getSummary` schemas |
| `pagination` | Shared `PageMeta`; no endpoint ownership |

## Exclusions, risks, and evidence

- `bearerAuth` is reserved documentation only; Console has no auth middleware.
- Gateway probes are operational, not public Console routes; gRPC/mTLS/shared-token semantics stay with protobuf and deployment docs.
- CRDs are not copied into OpenAPI: Kubernetes API/RBAC owns their trust boundary and Gateway owns zero CRDs.
- `make api-lint` validates the compatibility baseline, 50-route coverage, bundle and TypeScript drift, protobuf parsing, and Helm CRD rendering without rewriting committed artifacts. `make swagger-check` validates the Swag-derived chain.
- Full verification additionally runs Go format/vet/tests/build, frontend test/build, Helm lint/template, and final Git diff review.
