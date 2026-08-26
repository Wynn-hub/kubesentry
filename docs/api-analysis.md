# API Contract Analysis

## Boundaries

`api/openapi.yaml` indexes three sources without turning them into one REST API:

| Family | Source of truth | Inventory | Browser HTTP |
| --- | --- | ---: | --- |
| Console | `api/console/console-api.yaml` | 50 operations | `/api/v1` only |
| Gateway | `api/proto/gateway/v1/gateway.proto` | 5 RPCs: `Register`, `Sync`, `Connect`, `NotifyChange`, `Dispatch` | No |
| Member cluster | `charts/kubesentry/crds/{policy,policygroup,exception,policyversion}.yaml` | 4 CRDs | No |

Console OpenAPI alone generates Ogen and browser types. The obsolete
`api/internal/gateway.yaml` was removed: its probes were not a browser contract
and duplicated the protobuf streaming boundary.

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

The gate requires exactly these 50 generated operations and concrete
`internal/console/handler` methods; an inherited `dto.UnimplementedHandler`
method cannot satisfy coverage.

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
- `make api-lint` validates contracts, 50-route coverage, bundle drift, scratch Ogen/TypeScript drift, protobuf parsing, and Helm CRD rendering without rewriting committed artifacts.
- Full verification additionally runs Go format/vet/tests/build, frontend test/build, Helm lint/template, and final Git diff review.
