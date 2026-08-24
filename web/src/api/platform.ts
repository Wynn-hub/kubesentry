import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type PlatformHealth = components['schemas']['PlatformOverviewHealth'] | components['schemas']['PlatformComponentHealth']
export type PlatformRole = components['schemas']['PlatformInstance']['role']
export type PlatformInstance = components['schemas']['PlatformInstance']
export type PlatformComponent = components['schemas']['PlatformComponent']
export type PlatformCluster = components['schemas']['PlatformCluster']
export type PlatformOverview = components['schemas']['PlatformOverview']
export type PlatformTrendPoint = components['schemas']['PlatformTrendPoint']
export type PlatformTrend = components['schemas']['PlatformTrend']
export type PlatformLeadershipEvent = components['schemas']['LeadershipEvent']
export type PlatformLeadershipPage = components['schemas']['LeadershipEventListResponse']['data']
export type PlatformSettings = components['schemas']['PlatformSettings']
export type PlatformSettingsInput = components['schemas']['PlatformSettingsRequest']
type PlatformOverviewQuery = operations['getPlatformOverview']['parameters']['query']
type PlatformTrendQuery = operations['getPlatformInstanceTrend']['parameters']['query']
type LeadershipQuery = operations['listPlatformLeadershipEvents']['parameters']['query']

export function normalizePlatformOverview(value: PlatformOverview): PlatformOverview {
  return {
    ...value,
    gatewayPlane: (value.gatewayPlane ?? []).map(normalizeComponent),
    memberPlane: {
      ...value.memberPlane,
      clusters: (value.memberPlane?.clusters ?? []).map(cluster => ({
        ...cluster,
        components: (cluster.components ?? []).map(normalizeComponent),
      })),
      total: value.memberPlane?.total ?? 0,
      page: value.memberPlane?.page ?? 1,
      pageSize: value.memberPlane?.pageSize ?? 20,
    },
  }
}

function normalizeComponent(component: PlatformComponent): PlatformComponent {
  return {
    ...component,
    instances: component.instances ?? [],
    healthReason: component.healthReason ?? '',
    noLeader: component.noLeader ?? false,
    singleReplica: component.singleReplica ?? false,
  }
}

export const getPlatformOverview = async (query: PlatformOverviewQuery) =>
  normalizePlatformOverview(await extract<'getPlatformOverview'>(
    apiClient.GET('/platform/overview', { params: { query } }),
  ))

export const getPlatformInstanceTrend = async (
  component: operations['getPlatformInstanceTrend']['parameters']['path']['component'],
  instance: string,
  query: PlatformTrendQuery,
) => {
  const value = await extract<'getPlatformInstanceTrend'>(apiClient.GET('/platform/instances/{component}/{instance}/trend', {
    params: { path: { component, instance }, query },
  }))
  return { ...value, items: value.items ?? [] }
}

export const listPlatformLeadershipEvents = async (query: LeadershipQuery) => {
  const value = await extract<'listPlatformLeadershipEvents'>(apiClient.GET('/platform/leadership-events', {
    params: { query },
  }))
  return { ...value, items: value.items ?? [] }
}

export const getPlatformSettings = () =>
  extract<'getPlatformSettings'>(apiClient.GET('/platform/settings'))
export const updatePlatformSettings = (body: PlatformSettingsInput) =>
  extract<'updatePlatformSettings'>(apiClient.PUT('/platform/settings', { body }))
