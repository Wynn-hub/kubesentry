import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type LinkIncidentStatus = components['schemas']['dto.IncidentItem']['status']
export type LinkIncidentComponent = components['schemas']['dto.IncidentItem']['component']
export type LinkIncidentReason = components['schemas']['dto.IncidentItem']['reason']
export type LinkIncident = components['schemas']['dto.IncidentItem']
export type LinkIncidentSample = components['schemas']['dto.IncidentSampleView']
export type LinkIncidentDetail = components['schemas']['dto.IncidentView']
export type LinkIncidentQuery = operations['listAdmissionLinkIncidents']['parameters']['query']

export function normalizeLinkIncidentPage(value: Omit<components['schemas']['dto.IncidentPageView'], 'items'> & {
  items: components['schemas']['dto.IncidentPageView']['items'] | null
}) {
  return { ...value, items: value.items ?? [] }
}

export function normalizeLinkIncidentDetail(value: Omit<LinkIncidentDetail, 'samples'> & {
  samples: LinkIncidentDetail['samples'] | null
}): LinkIncidentDetail {
  return { ...value, samples: value.samples ?? [] }
}

export const listLinkIncidents = async (query: LinkIncidentQuery = {}) =>
  normalizeLinkIncidentPage(await extract<'listAdmissionLinkIncidents'>(
    apiClient.GET('/admission-link-incidents', { params: { query } }),
  ))

export const getLinkIncident = async (id: string) =>
  normalizeLinkIncidentDetail(await extract<'getAdmissionLinkIncident'>(
    apiClient.GET('/admission-link-incidents/{id}', { params: { path: { id } } }),
  ))
