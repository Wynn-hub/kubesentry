import { apiClient } from './generated/client'
import type { components } from './generated/console-api'
import { extract } from './http'

export type FieldNode = components['schemas']['FieldNode']

export const getFieldSchema = (group: string, version: string, resource: string, refresh = false) =>
  extract<'getFieldSchema'>(apiClient.GET('/schema/fields', {
    params: { query: { group, version, resource, ...(refresh ? { refresh } : {}) } },
  }))
