// @vitest-environment jsdom
import { createApp, nextTick } from 'vue'
import ElementPlus from 'element-plus'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@scalar/api-reference', () => ({
  createApiReference: () => ({ destroy() {} }),
}))
vi.mock('@scalar/api-reference/style.css', () => ({}))
import AppLayout from '../AppLayout.vue'
import { authState } from '../../auth/session'
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
    authState.status = 'authenticated'
    authState.me = { id: '1', username: 'admin', role: 'system_admin' }
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

  it('navigates to the authenticated help center from the AppLayout menu', async () => {
    authState.status = 'authenticated'
    authState.me = { id: '1', username: 'admin', role: 'system_admin' }
    await router.push('/')
    await router.isReady()
    const target = document.createElement('div')
    const app = createApp(AppLayout).use(router).use(i18n).use(ElementPlus)
    unmount = app.unmount
    app.mount(target)
    await nextTick()

    const menuItem = Array.from(target.querySelectorAll<HTMLElement>('.el-menu-item'))
      .find((item) => item.textContent?.includes(i18n.global.t('nav.help')))
    expect(menuItem).not.toBeNull()

    const navigated = new Promise<void>((resolve) => {
      const removeAfterEach = router.afterEach((to) => {
        if (to.fullPath === '/help') {
          removeAfterEach()
          resolve()
        }
      })
    })
    menuItem?.click()
    await navigated

    expect(router.currentRoute.value.fullPath).toBe('/help')
  })

  it('navigates to user management from the AppLayout menu', async () => {
    authState.status = 'authenticated'
    authState.me = { id: '1', username: 'admin', role: 'system_admin' }
    await router.push('/')
    await router.isReady()
    const target = document.createElement('div')
    const app = createApp(AppLayout).use(router).use(i18n).use(ElementPlus)
    unmount = app.unmount
    app.mount(target)
    await nextTick()

    const menuItem = Array.from(target.querySelectorAll<HTMLElement>('.el-menu-item'))
      .find((item) => item.textContent?.includes(i18n.global.t('nav.users')))
    expect(menuItem).not.toBeNull()

    const navigated = new Promise<void>((resolve) => {
      const removeAfterEach = router.afterEach((to) => {
        if (to.fullPath === '/users') {
          removeAfterEach()
          resolve()
        }
      })
    })
    menuItem?.click()
    await navigated

    expect(router.currentRoute.value.fullPath).toBe('/users')
  })

  it('hides management navigation from viewers', async () => {
    authState.status = 'authenticated'
    authState.me = { id: '2', username: 'viewer', role: 'viewer', tenantId: '1' }
    const target = document.createElement('div')
    const app = createApp(AppLayout).use(router).use(i18n).use(ElementPlus)
    unmount = app.unmount
    app.mount(target)
    await nextTick()

    const labels = Array.from(target.querySelectorAll('.el-menu-item')).map(item => item.textContent?.trim())
    expect(labels).not.toContain(i18n.global.t('nav.tenants'))
    expect(labels).not.toContain(i18n.global.t('nav.users'))
    expect(labels).toContain(i18n.global.t('nav.clusters'))
  })

  it('shows the signed-in user as an avatar menu', async () => {
	 authState.status = 'authenticated'
	 authState.me = { id: '1', username: 'admin', role: 'system_admin', lastLoginAt: '2026-10-08T01:00:00Z' }
	 const target = document.createElement('div')
	 const app = createApp(AppLayout).use(router).use(i18n).use(ElementPlus)
	 unmount = app.unmount
	 app.mount(target)
	 await nextTick()
	 expect(target.querySelector('.avatar')?.textContent).toBe('AD')
	 expect(target.querySelector('.profile-trigger')?.textContent).toContain('admin')
  })
})
