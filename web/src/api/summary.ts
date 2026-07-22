import { request } from './http'

export interface GroupSummary {
  name: string
  displayName: string
  enabled: boolean
  resolvedCount: number
  enforceCount: number
  auditCount: number
}

export interface SummaryData {
  totals: Record<string, number> & {
    clusters: number
    policies: number
    policygroups: number
    exceptions: number
  }
  clusterPhases: {
    online: number
    offline: number
    registering: number
  }
  groups: GroupSummary[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isGroupSummary(value: unknown): value is GroupSummary {
  if (!isRecord(value)) return false
  return (
    typeof value.name === 'string' &&
    typeof value.displayName === 'string' &&
    typeof value.enabled === 'boolean' &&
    isNonNegativeNumber(value.resolvedCount) &&
    isNonNegativeNumber(value.enforceCount) &&
    isNonNegativeNumber(value.auditCount)
  )
}

function isNonNegativeNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

export function normalizeSummary(value: unknown): SummaryData {
  if (
    !isRecord(value) ||
    !isRecord(value.totals) ||
    !isRecord(value.clusterPhases) ||
    !Array.isArray(value.groups)
  ) {
    throw new Error('Invalid summary response')
  }
  const totals = value.totals
  const clusterPhases = value.clusterPhases
  if (
    !isNonNegativeNumber(totals.clusters) ||
    !isNonNegativeNumber(totals.policies) ||
    !isNonNegativeNumber(totals.policygroups) ||
    !isNonNegativeNumber(totals.exceptions) ||
    Object.values(totals).some((total) => !isNonNegativeNumber(total)) ||
    !isNonNegativeNumber(clusterPhases.online) ||
    !isNonNegativeNumber(clusterPhases.offline) ||
    !isNonNegativeNumber(clusterPhases.registering) ||
    !value.groups.every(isGroupSummary)
  ) {
    throw new Error('Invalid summary response')
  }
  return {
    totals: totals as SummaryData['totals'],
    clusterPhases: clusterPhases as SummaryData['clusterPhases'],
    groups: value.groups,
  }
}

export const getSummary = async () => normalizeSummary(await request<unknown>('GET', '/summary'))
