import { apiClient } from './generated/client'
import type { components, operations } from './generated/console-api'
import { extract } from './http'

export type AdmissionEvent = components['schemas']['AdmissionEventItem']
export type AdmissionEventDetail = components['schemas']['AdmissionEventDetail']
export type AdmissionSettings = components['schemas']['AdmissionEventSettings']
export type AdmissionStoragePolicy = components['schemas']['AdmissionStoragePolicy']
type AdmissionEventsQuery = operations['listAdmissionEvents']['parameters']['query']

type CsvCompatible<Query> = {
  [Key in keyof Query]: Query[Key] extends readonly unknown[] | undefined ? string | Query[Key] : Query[Key]
}

export type AdmissionEventQuery = CsvCompatible<NonNullable<AdmissionEventsQuery>>

function splitCSV(value: string | readonly string[] | undefined): string[] | undefined {
  if (typeof value === 'string') return value.split(',').filter(Boolean)
  return value ? [...value] : undefined
}

export const listAdmissionEvents = (params: AdmissionEventQuery = {}) =>
  extract<'listAdmissionEvents'>(apiClient.GET('/admission-events', {
    params: {
      query: {
        ...params,
        cluster: splitCSV(params.cluster),
        verdict: splitCSV(params.verdict) as NonNullable<NonNullable<AdmissionEventsQuery>['verdict']>,
        namespace: splitCSV(params.namespace),
        gvk: splitCSV(params.gvk),
      },
    },
  }))
export const getAdmissionEvent = (id: string) =>
  extract<'getAdmissionEvent'>(apiClient.GET('/admission-events/{id}', { params: { path: { id } } }))
export const getAdmissionSettings = () =>
  extract<'getAdmissionEventSettings'>(apiClient.GET('/admission-event-settings'))
export const updateAdmissionSettings = (body: components['schemas']['AdmissionEventSettingsRequest']) =>
  extract<'updateAdmissionEventSettings'>(apiClient.PUT('/admission-event-settings', { body }))
