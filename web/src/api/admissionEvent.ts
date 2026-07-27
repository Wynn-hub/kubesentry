import { request } from './http'
import type { ListQuery, PageResult } from './pagination'

export interface AdmissionEvent {
  id: number; eventId: string; clusterName: string; occurredAt: string; verdict: 'audit' | 'deny'
  apiGroup: string; apiVersion: string; resource: string; operation: string; namespace: string; resourceName: string
  username: string; policyNames: string[]; violations: string[]
}
export interface AdmissionSettings { defaultRetentionDays: number; storagePolicyVersion: number; storagePolicies: AdmissionStoragePolicy[] }
export interface AdmissionStoragePolicy { verdicts: string[]; clusters: string[]; gvks: string[]; namespaces: string[]; action: 'retain' | 'discard'; retentionDays?: number }
export const listAdmissionEvents = (params: ListQuery & Record<string, string> = {}) => request<PageResult<AdmissionEvent>>('GET', '/admission-events', undefined, params)
export const getAdmissionSettings = () => request<AdmissionSettings>('GET', '/admission-event-settings')
export const updateAdmissionSettings = (body: AdmissionSettings) => request<AdmissionSettings>('PUT', '/admission-event-settings', body)
