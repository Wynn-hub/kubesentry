// @vitest-environment jsdom
import { createApp, nextTick } from 'vue'
import ElementPlus from 'element-plus'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@scalar/api-reference', () => ({
  createApiReference: () => ({ destroy() {} }),
}))
vi.mock('@scalar/api-reference/style.css', () => ({}))
import AppLayout from '../AppLayout.vue'
import { i18n } from '../../locales'
import router from '../../router'

describe('application navigation', () => {
  let unmount: (() => void) | undefined

  afterEach(() => {
    unmount?.()
    unmount = undefined
    document.body.replaceChildren()
  })

  it('navigates to the API documentation center from the AppLayout menu', async () => {
    await router.push('/')
    await router.isReady()
    const target = document.createElement('div')
    const app = createApp(AppLayout).use(router).use(i18n).use(ElementPlus)
    unmount = app.unmount
    app.mount(target)
    await nextTick()

    const menuItem = Array.from(target.querySelectorAll<HTMLElement>('.el-menu-item'))
      .find((item) => item.textContent?.includes(i18n.global.t('nav.apiDocs')))
    expect(menuItem).not.toBeNull()

    const navigated = new Promise<void>((resolve) => {
      const removeAfterEach = router.afterEach((to) => {
        if (to.fullPath === '/system/api-docs') {
          removeAfterEach()
          resolve()
        }
      })
    })
    menuItem?.click()
    await navigated

    expect(router.currentRoute.value.fullPath).toBe('/system/api-docs')
  })
})
