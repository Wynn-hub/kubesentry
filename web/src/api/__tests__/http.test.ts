import { describe, expect, it } from 'vitest'
import { apiClient } from '../generated/client'
import type { components } from '../generated/console-api'
import { ApiError, unwrap } from '../http'

// An arbitrary API path must never cross the browser transport boundary.
function assertReviewedPathsOnly() {
  // @ts-expect-error arbitrary paths are not part of the reviewed OpenAPI contract
  return apiClient.GET('/not-in-the-contract')
}

describe('unwrap', () => {
  it('extracts generated operation data when code is 0', () => {
    const response: components['schemas']['ClusterListResponse'] = {
      code: 0,
      message: 'success',
      data: { items: [], total: 0, page: 1, pageSize: 20 },
    }
    expect(unwrap<'listClusters'>(200, response, 'x')).toEqual(response.data)
  })

  it('throws ApiError carrying code, message and data on failure', () => {
    try {
      unwrap<'deletePolicy'>(409, { code: 40900, message: 'referenced', data: { referencedBy: ['g'] } }, 'x')
      expect.unreachable()
    } catch (e) {
      const err = e as ApiError
      expect(err).toBeInstanceOf(ApiError)
      expect(err.status).toBe(409)
      expect(err.code).toBe(40900)
      expect(err.message).toBe('referenced')
      expect(err.data).toEqual({ referencedBy: ['g'] })
    }
  })

  it('treats a non-zero code as failure even on HTTP 200', () => {
    try {
      unwrap<'getSummary'>(200, { code: 50000, message: 'boom' }, 'x')
      expect.unreachable()
    } catch (e) {
      expect((e as ApiError).code).toBe(50000)
    }
  })

  it('falls back to default message when envelope missing', () => {
    try {
      unwrap<'getSummary'>(500, undefined, 'network down')
      expect.unreachable()
    } catch (e) {
      expect((e as ApiError).message).toBe('network down')
    }
  })
})
