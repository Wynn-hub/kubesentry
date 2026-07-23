import { request } from './http'
import type { ListQuery, PageResult } from './pagination'
import type { LabelSelector } from './group'
import type { ClusterScope } from './group'

export interface ExceptionMatch {
  namespaces?: string[]
  namespaceSelector?: LabelSelector | null
  resourceSelector?: LabelSelector | null
}

export interface ExceptionListItem {
  name: string
  phase: string
  reason: string
  duration: string
  targetSummary: string
  expiresAt?: string
}

export interface ExceptionSpec {
  policyRefs?: string[]
  policyGroupRefs?: string[]
  allPolicies?: boolean
  match: ExceptionMatch
  duration: string
  retainAfterExpiry?: string
  reason: string
}

export interface ExceptionStatus {
  phase?: string
  message?: string
  effectiveAt?: string
  expiresAt?: string
}

export interface ExceptionDetail {
  name: string
  resourceVersion: string
  spec: ExceptionSpec
  status: ExceptionStatus
  scope: ClusterScope
}

export interface ExceptionRequest extends ExceptionSpec {
  name?: string
  resourceVersion?: string
  allClusters: boolean
  clusterNames?: string[]
}

export const listExceptions = (params: ListQuery = {}) =>
  request<PageResult<ExceptionListItem>>('GET', '/exceptions', undefined, params)
export const getException = (name: string) => request<ExceptionDetail>('GET', `/exceptions/${name}`)
export const createException = (req: ExceptionRequest) =>
  request<{ name: string }>('POST', '/exceptions', req)
export const updateException = (name: string, req: ExceptionRequest) =>
  request<{ name: string }>('PUT', `/exceptions/${name}`, req)
export const deleteException = (name: string) => request<null>('DELETE', `/exceptions/${name}`)
