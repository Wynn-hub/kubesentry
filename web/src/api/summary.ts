import { request } from './http'

export interface SummaryData {
  totals: Record<string, number>
  policyPhases: Record<string, number>
  policyModes: Record<string, number>
  groupPhases: Record<string, number>
  exceptionPhases: Record<string, number>
}

export const getSummary = () => request<SummaryData>('GET', '/summary')
