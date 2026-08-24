// This file is maintained beside the openapi-typescript output so every
// browser request is issued through the reviewed OpenAPI paths type.
import createClient from 'openapi-fetch'
import type { paths } from './console-api'

export const apiClient = createClient<paths>({
  baseUrl: '/api/v1',
  headers: { Accept: 'application/json' },
})

export type { paths } from './console-api'
