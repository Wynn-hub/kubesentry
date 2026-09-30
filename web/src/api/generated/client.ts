// This file is maintained beside the openapi-typescript output so every
// browser request is issued through the reviewed OpenAPI paths type.
import createClient from 'openapi-fetch'
import type { paths } from './console-api'

export const apiClient = createClient<paths>({
  baseUrl: '/api/v1',
  credentials: 'same-origin',
  headers: { Accept: 'application/json' },
  querySerializer: {
    array: { style: 'form', explode: false },
  },
})

apiClient.use({
  onRequest({ request }) {
    if (!['GET', 'HEAD', 'OPTIONS'].includes(request.method)) request.headers.set('X-KubeSentry-CSRF', '1')
    return request
  },
})

export type { paths } from './console-api'
