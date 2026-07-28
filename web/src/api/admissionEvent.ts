import { request } from './http'
import type { ListQuery, PageResult } from './pagination'

export interface AdmissionEvent {
  id: string; eventId: string; clusterName: string; occurredAt: string; verdict: 'audit' | 'deny'
  apiGroup: string; apiVersion: string; resource: string; operation: string; namespace: string; resourceName: string
  username: string; policyNames: string[]; violations: string[]
}
export interface AdmissionEventDetail extends AdmissionEvent {
  requestUid: string; userGroups: string[]; policyGroupNames: string[]
  metadataSnapshot: Record<string, unknown>; specSnapshot: Record<string, unknown>; storagePolicyVersion: string
}
export interface AdmissionSettings {
  defaultRetentionDays: number
  retryEnabled: boolean
  maxRetryAttempts: number
  queueCapacity: number
  storagePolicyVersion: number
  storagePolicies: AdmissionStoragePolicy[]
}
export interface AdmissionStoragePolicy {
  matchMode: 'include' | 'exclude'
  verdicts: string[]
  clusters: string[]
  gvks: string[]
  namespaces: string[]
  action: 'retain' | 'discard'
  retentionDays?: number
}
export const listAdmissionEvents = (params: ListQuery & Record<string, string> = {}) => request<PageResult<AdmissionEvent>>('GET', '/admission-events', undefined, params)
export const getAdmissionEvent = (id: string) => request<AdmissionEventDetail>('GET', `/admission-events/${id}`)
export const getAdmissionSettings = () => request<AdmissionSettings>('GET', '/admission-event-settings')
export const updateAdmissionSettings = (body: AdmissionSettings) => request<AdmissionSettings>('PUT', '/admission-event-settings', body)
