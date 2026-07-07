import { describe, expect, it } from 'vitest'
import { ApiError, unwrap } from '../http'

describe('unwrap', () => {
  it('returns data on success', () => {
    expect(unwrap(200, { success: true, data: 42, error: null }, 'x')).toBe(42)
  })

  it('throws ApiError with server message on failure', () => {
    try {
      unwrap(409, { success: false, data: { referencedBy: ['g'] }, error: 'referenced' }, 'x')
      expect.unreachable()
    } catch (e) {
      const err = e as ApiError
      expect(err).toBeInstanceOf(ApiError)
      expect(err.status).toBe(409)
      expect(err.message).toBe('referenced')
      expect(err.data).toEqual({ referencedBy: ['g'] })
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
