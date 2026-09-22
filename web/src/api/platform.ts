import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type PlatformHealth = components['schemas']['dto.PlatformOverviewView']['platformHealth'] | components['schemas']['dto.PlatformComponentView']['health']
export type PlatformRole = components['schemas']['dto.PlatformInstanceView']['role']
export type PlatformInstance = components['schemas']['dto.PlatformInstanceView']
export type PlatformComponent = components['schemas']['dto.PlatformComponentView']
export type PlatformCluster = components['schemas']['dto.PlatformClusterView']
export type PlatformOverview = components['schemas']['dto.PlatformOverviewView']
export type PlatformTrendPoint = components['schemas']['dto.PlatformTrendPointView']
export type PlatformTrend = components['schemas']['dto.PlatformTrendView']
export type PlatformLeadershipEvent = components['schemas']['dto.PlatformLeadershipEventView']
export type PlatformLeadershipPage = components['schemas']['dto.PlatformLeadershipPageView']
export type PlatformSettings = components['schemas']['dto.PlatformSettingsView']
export type PlatformSettingsInput = components['schemas']['dto.PlatformSettingsInput']
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
