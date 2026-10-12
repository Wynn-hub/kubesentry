// @vitest-environment jsdom
import { createApp, nextTick } from 'vue'
import ElementPlus from 'element-plus'
import { afterEach, describe, expect, it } from 'vitest'
import AuthLayout from '../../layouts/AuthLayout.vue'
import { authState, avatarInitials } from '../../auth/session'
import { i18n } from '../../locales'
import router from '../../router'
import loginView from '../LoginView.vue?raw'
import ChangePasswordView from '../ChangePasswordView.vue'

describe('authentication experience', () => {
  afterEach(() => document.body.replaceChildren())

  it('renders the shared branded authentication shell', async () => {
    const target = document.createElement('div')
    createApp(AuthLayout, { default: () => 'form' }).use(i18n).use(ElementPlus).mount(target)
    await nextTick()
    expect(target.textContent).toContain('KubeSentry')
    expect(target.querySelector('.brand-panel')).not.toBeNull()
    expect(target.querySelector('.locale')).not.toBeNull()
  })

  it('redirects restricted sessions to password change', async () => {
    authState.status = 'authenticated'
    authState.me = { id: '1', username: 'admin', role: 'system_admin', mustChangePassword: true }
    await router.push('/clusters')
    expect(router.currentRoute.value.name).toBe('change-password')
  })

  it('allows signed-in users to open password change voluntarily', async () => {
    authState.status = 'authenticated'
    authState.me = { id: '1', username: 'admin', role: 'system_admin', mustChangePassword: false }
    await router.push('/')
    await router.push('/change-password')
    expect(router.currentRoute.value.name).toBe('change-password')
  })

  it('asks for the current password before a new password', async () => {
    authState.status = 'authenticated'
    authState.me = { id: '1', username: 'admin', role: 'system_admin', mustChangePassword: false }
    const target = document.createElement('div')
    createApp(ChangePasswordView).use(i18n).use(ElementPlus).use(router).mount(target)
    await nextTick()

    expect(target.textContent).toContain('当前密码')
    expect(target.querySelectorAll('input[type="password"]')).toHaveLength(3)
  })

  it('submits login when Enter is pressed in the password field', () => {
    expect(loginView).toContain('@keyup.enter="submit"')
  })

  it('derives stable avatar initials', () => {
    expect(avatarInitials('admin')).toBe('AD')
    expect(avatarInitials()).toBe('?')
  })
})
