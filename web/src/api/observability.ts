import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type LinkIncidentStatus = components['schemas']['AdmissionLinkIncidentItem']['status']
export type LinkIncidentComponent = components['schemas']['AdmissionLinkIncidentItem']['component']
export type LinkIncidentReason = components['schemas']['AdmissionLinkIncidentItem']['reason']
export type LinkIncident = components['schemas']['AdmissionLinkIncidentItem']
export type LinkIncidentSample = components['schemas']['AdmissionLinkIncidentSample']
export type LinkIncidentDetail = components['schemas']['AdmissionLinkIncidentDetail']
export type LinkIncidentQuery = operations['listAdmissionLinkIncidents']['parameters']['query']

export function normalizeLinkIncidentPage(value: Omit<components['schemas']['AdmissionLinkIncidentListData'], 'items'> & {
  items: components['schemas']['AdmissionLinkIncidentListData']['items'] | null
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
