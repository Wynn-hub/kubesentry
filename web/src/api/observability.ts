import { request } from './http'

export type ObservationHealth = 'normal' | 'warning' | 'partial_no_data' | 'no_data' | 'not_onboarded' | 'offline'

export interface ObservationFreshness {
  lastObservedAt: string | null
  fresh: boolean
}

export interface ObservationFaults {
  dropped: number
  retryExhausted: number
  batchFailure: number
  authFailure: number
  ackTimeout: number
  reportDropped: number
}

export interface ObservationQueue {
  depthMax: number
  capacity: number
}

export interface ObservationReason {
  component: string
  reason: string
  count: number
}

export interface ObservationClusterCounts {
  total: number
  online: number
  normal: number
  warning: number
  partialNoData: number
  noData: number
  notOnboarded: number
  offline: number
}

export interface AdmissionObservationOverview {
  disabled?: boolean
  health: ObservationHealth
  dataIncomplete: boolean
  baselineReset: boolean
  observabilityOnboarded: boolean
  platformHealth: {
    health: 'normal' | 'warning'
    cleanupFailure: number
    observationWriteFailed: number
    authFailure: number
  }
  freshness: Record<string, ObservationFreshness>
  generated: number
  operatorReceived: number
  gatewayReceived: number
  written: number
  policyFilteredMember: number
  policyFilteredGateway: number
  retentionElapsed: number
  duplicate: number
  queueRejected: Record<string, number>
  retryCount: Record<string, number>
  windowFaults: Record<string, ObservationFaults>
  windowFaultTotal: number
  seriesStarted: Record<string, number>
  sequenceGap: Record<string, number>
  baselineResetByComponent: Record<string, number>
  queues: Record<string, ObservationQueue>
  faultReasons: ObservationReason[]
  clusterCounts: ObservationClusterCounts
  from: string
  to: string
}

export interface ObservationTrendPoint {
  bucketStartedAt: string
  present: boolean
  generated: number
  operatorReceived: number
  gatewayReceived: number
  written: number
  policyFiltered: number
  faults: number
}

export interface ObservationTrend {
  stepSeconds: number
  items: ObservationTrendPoint[]
  from: string
  to: string
}

export interface ObservationCluster {
  name: string
  phase: string
  health: ObservationHealth
  observabilityOnboarded: boolean
  freshness: Record<string, ObservationFreshness>
  queues: Record<string, ObservationQueue>
  windowFaults: ObservationFaults
  majorFaultReasons: ObservationReason[]
  dataIncomplete: boolean
}

export interface ObservationClusterPage {
  items: ObservationCluster[]
  total: number
  page: number
  pageSize: number
}

export type ObservationQuery = {
  cluster?: string
  from?: string
  to?: string
}

const paramsOf = (query: ObservationQuery): Record<string, string> =>
  Object.fromEntries(Object.entries(query).filter((entry): entry is [string, string] => Boolean(entry[1])))

export function normalizeAdmissionObservationOverview(
  value: AdmissionObservationOverview,
): AdmissionObservationOverview {
  return {
    ...value,
    freshness: value.freshness ?? {},
    queueRejected: value.queueRejected ?? {},
    retryCount: value.retryCount ?? {},
    windowFaults: value.windowFaults ?? {},
    seriesStarted: value.seriesStarted ?? {},
    sequenceGap: value.sequenceGap ?? {},
    baselineResetByComponent: value.baselineResetByComponent ?? {},
    queues: value.queues ?? {},
    faultReasons: value.faultReasons ?? [],
  }
}

export function normalizeObservationTrend(value: ObservationTrend): ObservationTrend {
  return { ...value, items: value.items ?? [] }
}

export function normalizeObservationClusterPage(value: ObservationClusterPage): ObservationClusterPage {
  return {
    ...value,
    items: (value.items ?? []).map(item => ({
      ...item,
      freshness: item.freshness ?? {},
      queues: item.queues ?? {},
      majorFaultReasons: item.majorFaultReasons ?? [],
    })),
  }
}

export const getAdmissionObservationOverview = (query: ObservationQuery & { currentOnly?: string } = {}) =>
  request<AdmissionObservationOverview>('GET', '/observability/admission/overview', undefined, paramsOf(query))
    .then(normalizeAdmissionObservationOverview)

export const getAdmissionObservationTrend = (query: ObservationQuery = {}) =>
  request<ObservationTrend>('GET', '/observability/admission/trend', undefined, paramsOf(query))
    .then(normalizeObservationTrend)

export const listAdmissionObservationClusters = (
  query: ObservationQuery & { page: string; pageSize: string; health?: string },
) => request<ObservationClusterPage>('GET', '/observability/admission/clusters', undefined, paramsOf(query))
  .then(normalizeObservationClusterPage)
