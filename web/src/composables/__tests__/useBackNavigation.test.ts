import { describe, expect, it } from 'vitest'
import { buildBackFallbackTarget, isValidApplicationBack } from '../useBackNavigation'

describe('back navigation', () => {
  it('accepts a distinct application history entry', () => {
    expect(isValidApplicationBack('/policies?page=2', '/policies/demo', '/')).toBe(true)
    expect(isValidApplicationBack('/console/policies?page=2', '/policies/demo', '/console/')).toBe(true)
  })

  // createWebHistory() with no argument reports an empty base. Treating it as a
  // path prefix used to reject every in-app entry, so the back button always
  // fell through to the parent-page fallback instead of the previous page.
  it('treats an empty base as the domain root', () => {
    expect(isValidApplicationBack('/events', '/policies/demo', '')).toBe(true)
    expect(isValidApplicationBack('/policies?page=2', '/policies/demo', '')).toBe(true)
    expect(isValidApplicationBack('/policies/demo', '/policies/demo', '')).toBe(false)
  })

  it.each([
    [null, '/policies/demo', '/'],
    ['', '/policies/demo', '/'],
    ['https://example.com/policies', '/policies/demo', '/'],
    ['//example.com/policies', '/policies/demo', '/'],
    ['/outside/policies', '/policies/demo', '/console/'],
    ['/policies/demo', '/policies/demo', '/'],
  ])('rejects invalid, external, or current history entry %#', (back, current, base) => {
    expect(isValidApplicationBack(back, current, base)).toBe(false)
  })

  it('builds static and dynamic parent fallbacks from route params', () => {
    expect(buildBackFallbackTarget({ name: 'policies' }, {})).toEqual({ name: 'policies' })
    expect(
      buildBackFallbackTarget(
        { name: 'policy-detail', inheritParams: ['name'], fallbackName: 'policies' },
        { name: 'demo', ignored: 'value' },
      ),
    ).toEqual({ name: 'policy-detail', params: { name: 'demo' } })
  })

  it('uses the static fallback when a required dynamic parameter is missing', () => {
    expect(
      buildBackFallbackTarget(
        { name: 'policy-detail', inheritParams: ['name'], fallbackName: 'policies' },
        {},
      ),
    ).toEqual({ name: 'policies' })
  })
})
