import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type LabelSelectorRequirement = components['schemas']['KubernetesLabelSelectorRequirement']
export type LabelSelector = components['schemas']['KubernetesLabelSelector']
export type PolicyRef = NonNullable<NonNullable<components['schemas']['PolicyGroupSpec']['policies']>['byName']>[number]
export type GroupPolicies = NonNullable<components['schemas']['PolicyGroupSpec']['policies']>
export type GroupListItem = components['schemas']['PolicyGroupListItem']
export type EffectiveMember = components['schemas']['EffectiveMember']
export type GroupSpec = components['schemas']['PolicyGroupSpec']
export type GroupStatus = components['schemas']['PolicyGroupStatus']
export type GroupDetail = components['schemas']['PolicyGroupDetail']
export type ClusterScope = components['schemas']['PolicyGroupScope']
export type GroupRequest = components['schemas']['PolicyGroupRequest']
export type ListGroupsQuery = NonNullable<operations['listPolicyGroups']['parameters']['query']>

export const listGroups = (params: ListGroupsQuery = {}) =>
  extract<'listPolicyGroups'>(apiClient.GET('/policygroups', { params: { query: params } }))
export const getGroup = (name: string) =>
  extract<'getPolicyGroup'>(apiClient.GET('/policygroups/{name}', { params: { path: { name } } }))
export const createGroup = (body: GroupRequest) =>
  extract<'createPolicyGroup'>(apiClient.POST('/policygroups', { body }))
export const updateGroup = (name: string, body: GroupRequest) =>
  extract<'updatePolicyGroup'>(apiClient.PUT('/policygroups/{name}', { params: { path: { name } }, body }))
export const deleteGroup = (name: string) =>
  extract<'deletePolicyGroup'>(apiClient.DELETE('/policygroups/{name}', { params: { path: { name } } }))
export const setGroupEnabled = (name: string, enabled: boolean) =>
  extract<'setPolicyGroupEnabled'>(apiClient.PUT('/policygroups/{name}/enabled', { params: { path: { name } }, body: { enabled } }))
