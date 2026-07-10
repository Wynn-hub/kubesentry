import { request } from './http'

export interface LabelSelector {
  matchLabels?: Record<string, string>
}

export interface PolicyRef {
  name: string
  enforcementMode?: string
}

export interface GroupPolicies {
  byName?: PolicyRef[]
  bySelector?: LabelSelector
  exclude?: string[]
}

export interface GroupListItem {
  name: string
  displayName: string
  source: string
  enabled: boolean
  phase: string
  resolvedCount: number
}

export interface EffectiveMember {
  name: string
  enforcementMode: string
  source: string
}

export interface GroupSpec {
  displayName?: string
  description?: string
  enabled: boolean
  namespaceSelector?: LabelSelector | null
  policies?: GroupPolicies
  selectorEnforcementMode?: string
}

export interface GroupStatus {
  phase?: string
  resolvedCount?: number
  resolvedPolicies?: EffectiveMember[]
}

export interface GroupDetail {
  name: string
  source: string
  resourceVersion: string
  spec: GroupSpec
  status: GroupStatus
}

export interface GroupRequest {
  name?: string
  displayName: string
  description: string
  enabled: boolean
  namespaceSelector?: LabelSelector | null
  policies: GroupPolicies
  selectorEnforcementMode?: string
  resourceVersion?: string
}

export const listGroups = () => request<GroupListItem[]>('GET', '/policygroups')
export const getGroup = (name: string) => request<GroupDetail>('GET', `/policygroups/${name}`)
export const createGroup = (req: GroupRequest) => request<{ name: string }>('POST', '/policygroups', req)
export const updateGroup = (name: string, req: GroupRequest) =>
  request<{ name: string }>('PUT', `/policygroups/${name}`, req)
export const deleteGroup = (name: string) => request<null>('DELETE', `/policygroups/${name}`)
export const setGroupEnabled = (name: string, enabled: boolean) =>
  request<{ enabled: boolean }>('PUT', `/policygroups/${name}/enabled`, { enabled })
