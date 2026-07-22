import { request } from './http'
import type { ListQuery, PageResult } from './pagination'

export interface MatchResource {
  apiGroups: string[]
  apiVersions: string[]
  resources: string[]
}

export interface PolicyMatch {
  operations: string[]
  resources: MatchResource[]
}

export interface PolicyListItem {
  name: string
  source: string
  enforcementMode: string
  phase: string
  description: string
  createdAt: string
  updatedAt: string | null
}

export interface PolicyDetail {
  name: string
  source: string
  description: string
  labels: Record<string, string> | null
  annotations: Record<string, string> | null
  enforcementMode: string
  match: PolicyMatch
  rego: string
  phase: string
  compileError?: string
  referencedBy: string[] | null
  resourceVersion: string
  createdAt: string
  updatedAt: string | null
}

export interface PolicyRequest {
  name?: string
  description: string
  enforcementMode: string
  match: PolicyMatch
  rego: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  resourceVersion?: string
}

export interface VersionEntry {
  version: number
  createdAt: string
  phase: string
  rego: string
  match: PolicyMatch
  enforcementMode: string
  isCurrent: boolean
}

export interface VersionTimeline {
  currentVersion: number
  cursor: number
  head: number
  inFlight: boolean
  prevEnabled: boolean
  nextEnabled: boolean
  versions: VersionEntry[]
}

export const listPolicies = (params: ListQuery = {}) =>
  request<PageResult<PolicyListItem>>('GET', '/policies', undefined, params)
export const getPolicy = (name: string) => request<PolicyDetail>('GET', `/policies/${name}`)
export const createPolicy = (req: PolicyRequest) => request<{ name: string }>('POST', '/policies', req)
export const updatePolicy = (name: string, req: PolicyRequest) =>
  request<{ name: string }>('PUT', `/policies/${name}`, req)
export const deletePolicy = (name: string, force = false) =>
  request<null>('DELETE', `/policies/${name}`, undefined, force ? { force: 'true' } : {})
export const validateRego = (rego: string) => request<null>('POST', '/policies/validate', { rego })
export const getTimeline = async (name: string) => {
  const timeline = await request<VersionTimeline>('GET', `/policies/${name}/versions`)
  return { ...timeline, versions: [...timeline.versions].sort((a, b) => b.version - a.version) }
}
export const rollback = (name: string, direction: 'prev' | 'next') =>
  request<{ targetVersion: number }>('POST', `/policies/${name}/rollback`, { direction })

export interface ResourceSuggestions {
  apiGroups: string[]
  apiVersions: string[]
  resources: string[]
  resourcesByGroup: Record<string, string[]>
}
export const getResourceSuggestions = () =>
  request<ResourceSuggestions>('GET', '/policies/resource-suggestions')
