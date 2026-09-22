import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type DistributionClusters = NonNullable<components['schemas']['dto.DistributionSpecView']['clusters']>
export type DistributionOverride = NonNullable<components['schemas']['dto.DistributionSpecView']['overrides']>[number]
export type PolicyDistributionSpec = components['schemas']['dto.DistributionSpecView']
export type DistributionListItem = components['schemas']['dto.DistributionItem']
export type DistributionDetail = components['schemas']['dto.DistributionView']
export type DistributionRequest = components['schemas']['dto.DistributionInput']
export type ListDistributionsQuery = NonNullable<operations['listDistributions']['parameters']['query']>

export const listDistributions = (params: ListDistributionsQuery = {}) =>
  extract<'listDistributions'>(apiClient.GET('/distributions', { params: { query: params } }))
export const getDistribution = (name: string) =>
  extract<'getDistribution'>(apiClient.GET('/distributions/{name}', { params: { path: { name } } }))
export const createDistribution = (body: DistributionRequest) =>
  extract<'createDistribution'>(apiClient.POST('/distributions', { body }))
export const updateDistribution = (name: string, body: DistributionRequest) =>
  extract<'updateDistribution'>(apiClient.PUT('/distributions/{name}', { params: { path: { name } }, body }))
export const deleteDistribution = (name: string) =>
  extract<'deleteDistribution'>(apiClient.DELETE('/distributions/{name}', { params: { path: { name } } }))
