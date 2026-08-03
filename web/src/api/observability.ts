import { request } from './http'
import type { PageResult } from './pagination'

export type LinkIncidentStatus = 'active' | 'interrupted' | 'closed'
export type LinkIncidentComponent = 'webhook' | 'operator' | 'gateway'
export type LinkIncidentReason = 'queue_full' | 'retry_exhausted' | 'ack_timeout' | 'auth_failure' | 'batch_failure' | 'write_failed' | 'report_dropped'

export interface LinkIncident {
  id: string
  clusterName: string
  reporterInstanceId: string
  component: LinkIncidentComponent
  reason: LinkIncidentReason
  startedAt: string
  lastSeenAt: string
  affectedCount: number
  status: LinkIncidentStatus
}

export interface LinkIncidentSample {
  eventId: string
  eventDatabaseId?: string
  expired: boolean
}

export interface LinkIncidentDetail extends LinkIncident {
  detail: string
  samples: LinkIncidentSample[]
}

export type LinkIncidentQuery = Record<string, string>

export const listLinkIncidents = (query: LinkIncidentQuery = {}) =>
  request<PageResult<LinkIncident>>('GET', '/admission-link-incidents', undefined, query)
    .then(page => ({ ...page, items: page.items ?? [] }))

export const getLinkIncident = (id: string) =>
  request<LinkIncidentDetail>('GET', `/admission-link-incidents/${id}`)
    .then(value => ({ ...value, samples: value.samples ?? [] }))
