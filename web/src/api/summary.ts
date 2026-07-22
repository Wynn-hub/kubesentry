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
  totals: Record<string, number>
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
    typeof value.resolvedCount === 'number' &&
    typeof value.enforceCount === 'number' &&
    typeof value.auditCount === 'number'
  )
}

export function normalizeSummary(value: unknown): SummaryData {
  if (!isRecord(value) || !isRecord(value.totals) || !Array.isArray(value.groups)) {
    throw new Error('Invalid summary response')
  }
  const totals = value.totals
  if (
    typeof totals.policies !== 'number' ||
    typeof totals.policygroups !== 'number' ||
    typeof totals.exceptions !== 'number' ||
    Object.values(totals).some((total) => typeof total !== 'number') ||
    !value.groups.every(isGroupSummary)
  ) {
    throw new Error('Invalid summary response')
  }
  return { totals: totals as Record<string, number>, groups: value.groups }
}

export const getSummary = async () => normalizeSummary(await request<unknown>('GET', '/summary'))
