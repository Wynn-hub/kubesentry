import { request } from './http'
import type { LabelSelector } from './group'

export interface DistributionClusters {
  byName?: string[]
  clusterSelector?: LabelSelector | null
}

export interface DistributionOverride {
  cluster: string
  enforcementMode: string
}

export interface PolicyDistributionSpec {
  clusters?: DistributionClusters
  policyGroups: string[]
  overrides?: DistributionOverride[]
}

export interface DistributionListItem {
  name: string
  policyGroups: string[]
}

export interface DistributionDetail {
  name: string
  resourceVersion: string
  spec: PolicyDistributionSpec
}

export interface DistributionRequest {
  name?: string
  spec: PolicyDistributionSpec
  resourceVersion?: string
}

export const listDistributions = () => request<DistributionListItem[]>('GET', '/distributions')
export const getDistribution = (name: string) => request<DistributionDetail>('GET', `/distributions/${name}`)
export const createDistribution = (req: DistributionRequest) =>
  request<{ name: string }>('POST', '/distributions', req)
export const updateDistribution = (name: string, req: DistributionRequest) =>
  request<{ name: string }>('PUT', `/distributions/${name}`, req)
export const deleteDistribution = (name: string) => request<null>('DELETE', `/distributions/${name}`)
