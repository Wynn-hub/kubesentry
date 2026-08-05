import { describe, expect, it } from 'vitest'
import { ApiError, unwrap } from '../http'

describe('unwrap', () => {
  it('returns data when code is 0', () => {
    expect(unwrap(200, { code: 0, message: 'success', data: 42 }, 'x')).toBe(42)
  })

  it('throws ApiError carrying code, message and data on failure', () => {
    try {
      unwrap(409, { code: 40900, message: 'referenced', data: { referencedBy: ['g'] } }, 'x')
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
      unwrap(200, { code: 50000, message: 'boom' }, 'x')
      expect.unreachable()
    } catch (e) {
      expect((e as ApiError).code).toBe(50000)
    }
  })

  it('falls back to default message when envelope missing', () => {
    try {
      unwrap(500, undefined, 'network down')
      expect.unreachable()
    } catch (e) {
      expect((e as ApiError).message).toBe('network down')
    }
  })
})
