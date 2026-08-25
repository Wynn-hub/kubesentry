// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import router from '../../router'

describe('API documentation route', () => {
  it('registers the documentation center under system settings', () => {
    const route = router.resolve('/system/api-docs')

    expect(route.name).toBe('api-docs')
    expect(route.matched).toHaveLength(2)
  })
})
