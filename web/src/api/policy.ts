import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type MatchResource = components['schemas']['dto.MatchResourceInput']
export type PolicyMatch = components['schemas']['dto.PolicyMatchInput']
export type PolicyListItem = components['schemas']['dto.SwagPolicyListItem']
export type PolicyDetail = components['schemas']['dto.SwagPolicyDetail']
export type PolicyRequest = components['schemas']['dto.SwagPolicyInput']
export type VersionEntry = components['schemas']['dto.SwagPolicyVersionEntry']
export type VersionTimeline = components['schemas']['dto.SwagPolicyVersionList']
export type ResourceSuggestions = components['schemas']['dto.SwagResourceSuggestions']
export type ListPoliciesQuery = NonNullable<operations['listPolicies']['parameters']['query']>
export const POLICY_PHASES = ['Ready', 'Invalid'] as const

export const listPolicies = (params: ListPoliciesQuery = {}) =>
  extract<'listPolicies'>(apiClient.GET('/policies', { params: { query: params } }))
export const getPolicy = (name: string) =>
  extract<'getPolicy'>(apiClient.GET('/policies/{name}', { params: { path: { name } } }))
export const createPolicy = (body: PolicyRequest) =>
  extract<'createPolicy'>(apiClient.POST('/policies', { body }))
export const updatePolicy = (name: string, body: PolicyRequest) =>
  extract<'updatePolicy'>(apiClient.PUT('/policies/{name}', { params: { path: { name } }, body }))
export const deletePolicy = (name: string, force = false) =>
  extract<'deletePolicy'>(apiClient.DELETE('/policies/{name}', { params: { path: { name }, query: force ? { force } : {} } }))
export const validateRego = (rego: components['schemas']['dto.SwagValidateRegoInput']['rego']) =>
  extract<'validatePolicyRego'>(apiClient.POST('/policies/validate', { body: { rego } }))
export const getTimeline = async (name: string) => {
  const timeline = await extract<'listPolicyVersions'>(apiClient.GET('/policies/{name}/versions', { params: { path: { name } } }))
  return { ...timeline, versions: [...timeline.versions].sort((a, b) => b.version - a.version) }
}
export const rollback = (name: string, direction: components['schemas']['dto.SwagRollbackInput']['direction']) =>
  extract<'rollbackPolicy'>(apiClient.POST('/policies/{name}/rollback', { params: { path: { name } }, body: { direction } }))
export const getResourceSuggestions = () =>
  extract<'getResourceSuggestions'>(apiClient.GET('/policies/resource-suggestions'))
