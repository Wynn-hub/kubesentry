import { apiClient } from './generated/client'
import type { components } from './generated/console-api'
import { extract } from './http'

export type GroupSummary = components['schemas']['dto.SummaryGroupView']
export type SummaryData = components['schemas']['dto.SummaryView']

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isNonNegativeNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

export function normalizeSummary(value: unknown): SummaryData {
  if (!isRecord(value) || !isRecord(value.totals) || !isRecord(value.clusterPhases) || !Array.isArray(value.groups)) {
    throw new Error('Invalid summary response')
  }
  const { totals, clusterPhases, groups } = value
  if (
    !isNonNegativeNumber(totals.clusters) || !isNonNegativeNumber(totals.policies) ||
    !isNonNegativeNumber(totals.policygroups) || !isNonNegativeNumber(totals.exceptions) ||
    Object.values(totals).some(total => !isNonNegativeNumber(total)) ||
    !isNonNegativeNumber(clusterPhases.online) || !isNonNegativeNumber(clusterPhases.offline) ||
    !isNonNegativeNumber(clusterPhases.registering) ||
    !groups.every(group => isRecord(group) && typeof group.name === 'string' && typeof group.displayName === 'string' &&
      typeof group.enabled === 'boolean' && isNonNegativeNumber(group.resolvedCount) &&
      isNonNegativeNumber(group.enforceCount) && isNonNegativeNumber(group.auditCount))
  ) {
    throw new Error('Invalid summary response')
  }
  return value as SummaryData
}

export const getSummary = async () => normalizeSummary(await extract<'getSummary'>(apiClient.GET('/summary')))
