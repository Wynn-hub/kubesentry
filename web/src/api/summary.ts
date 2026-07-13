import { request } from './http'

export interface WebhookSummary {
  configured: boolean
  caBundleSet: boolean
  ruleCount: number
}

export interface NamespaceCoverage {
  protected: number
  total: number
}

export interface GroupSummary {
  name: string
  displayName: string
  enabled: boolean
  phase: string
  resolvedCount: number
  enforceCount: number
  auditCount: number
  matchedNamespaces: number
}

export interface RecentVersion {
  policy: string
  version: number
  createdAt: string
}

export interface SummaryData {
  totals: Record<string, number>
  policyPhases: Record<string, number>
  policyModes: Record<string, number>
  groupPhases: Record<string, number>
  exceptionPhases: Record<string, number>
  webhook: WebhookSummary
  namespaceCoverage: NamespaceCoverage
  groups: GroupSummary[]
  recentVersions: RecentVersion[]
}

export const getSummary = () => request<SummaryData>('GET', '/summary')
