// @vitest-environment jsdom
import { createApp, nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import router from '../../router'

const { createApiReference, destroy } = vi.hoisted(() => ({
  createApiReference: vi.fn(),
  destroy: vi.fn(),
}))

vi.mock('@scalar/api-reference', () => ({ createApiReference }))
vi.mock('@scalar/api-reference/style.css', () => ({}))
vi.mock('../../components/PageHeader.vue', () => ({
  default: { template: '<header><slot /></header>' },
}))

import ApiDocsView from '../ApiDocsView.vue'

describe('API documentation route', () => {
  it('registers the documentation center under system settings', () => {
    const route = router.resolve('/system/api-docs')

    expect(route.name).toBe('api-docs')
    expect(route.matched).toHaveLength(2)
  })
})

describe('API documentation view', () => {
  beforeEach(() => {
    createApiReference.mockReset()
    destroy.mockReset()
    createApiReference.mockReturnValue({ destroy })
  })

  it('mounts Scalar with the same-origin specification and releases it on unmount', async () => {
    const target = document.createElement('div')
    const app = createApp(ApiDocsView)
    app.config.globalProperties.$t = (key: string) => key
    app.component('el-card', { template: '<article><slot /></article>' })

    app.mount(target)
    await nextTick()

    expect(createApiReference).toHaveBeenCalledWith(
      expect.any(HTMLElement),
      expect.objectContaining({
        url: '/api/docs/openapi.json',
        darkMode: true,
        theme: 'none',
        hideTestRequestButton: false,
        tagsSorter: 'alpha',
      }),
    )
    expect(target.textContent).toContain('apiDocs.gatewayTitle')
    expect(target.textContent).toContain('apiDocs.crdTitle')

    app.unmount()
    expect(destroy).toHaveBeenCalledOnce()
  })
})
