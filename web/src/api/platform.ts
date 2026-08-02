import { request } from './http'

export type PlatformHealth = 'ok' | 'pending' | 'degraded' | 'unavailable' | 'disabled'
export type PlatformRole = 'leader' | 'standby' | 'none'

export interface PlatformInstance {
  instance: string
  role: PlatformRole
  health: string
  healthReason: string
  uptimeSeconds: number
  restartCount: number
  memoryBytes: number
  memoryLimitBytes: number
  cpuMillis: number
  goroutines: number
  heapAllocBytes: number
  heapSysBytes: number
  lastObservedAt: string
  dbStats?: Record<string, number>
}

export interface PlatformComponent {
  component: string
  health: PlatformHealth
  healthReason: string
  hasLeadership: boolean
  noLeader: boolean
  singleReplica: boolean
  instances: PlatformInstance[]
  lastLeadershipChange?: string
}

export interface PlatformCluster {
  name: string
  phase: string
  health: PlatformHealth
  components: PlatformComponent[]
}

export interface PlatformOverview {
  enabled: boolean
  platformHealth: PlatformHealth
  gatewayPlane: PlatformComponent[]
  memberPlane: {
    clusters: PlatformCluster[]
    total: number
    page: number
    pageSize: number
  }
}

export interface PlatformTrendPoint {
  bucketStartedAt: string
  memoryBytes: number
  cpuUsageMicros: number
  goroutines: number
  gcPauseMicros: number
}

export interface PlatformTrend {
  items: PlatformTrendPoint[]
  stepSeconds: number
  from: string
  to: string
}

export interface PlatformLeadershipEvent {
  id: number
  component: string
  clusterId: number
  clusterName: string
  instance: string
  roleFrom: string
  roleTo: string
  changedAt: string
  detectedBy: string
}

export interface PlatformLeadershipPage {
  items: PlatformLeadershipEvent[]
  total: number
  page: number
  pageSize: number
}

export interface PlatformSettings {
  enabled: boolean
  retentionDays: number
  maxRetentionDays: number
}

export interface PlatformSettingsInput {
  enabled: boolean
  retentionDays: number
}

const cleanParams = (query: Record<string, string | undefined>): Record<string, string> =>
  Object.fromEntries(Object.entries(query).filter((entry): entry is [string, string] => Boolean(entry[1])))

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

export const getPlatformOverview = (query: { page: string; pageSize: string; health?: string }) =>
  request<PlatformOverview>('GET', '/platform/overview', undefined, cleanParams(query))
    .then(normalizePlatformOverview)

export const getPlatformInstanceTrend = (
  component: string,
  instance: string,
  query: { from: string; to: string; cluster?: string },
) => request<PlatformTrend>(
  'GET',
  `/platform/instances/${encodeURIComponent(component)}/${encodeURIComponent(instance)}/trend`,
  undefined,
  cleanParams(query),
).then(value => ({ ...value, items: value.items ?? [] }))

export const listPlatformLeadershipEvents = (
  query: { page: string; pageSize: string; component?: string; cluster?: string; from: string; to: string },
) => request<PlatformLeadershipPage>('GET', '/platform/leadership-events', undefined, cleanParams(query))
  .then(value => ({ ...value, items: value.items ?? [] }))

export const getPlatformSettings = () =>
  request<PlatformSettings>('GET', '/platform/settings')

export const updatePlatformSettings = (body: PlatformSettingsInput) =>
  request<PlatformSettings>('PUT', '/platform/settings', body)
