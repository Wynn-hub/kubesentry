import { request } from './http'
import type { FieldNode } from '../components/fieldTree'

export const getFieldSchema = (group: string, version: string, resource: string, refresh = false) =>
  request<FieldNode[]>('GET', '/schema/fields', undefined, {
    group, version, resource, ...(refresh ? { refresh: 'true' } : {}),
  })
