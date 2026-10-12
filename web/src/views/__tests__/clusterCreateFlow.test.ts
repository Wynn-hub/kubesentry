// @vitest-environment jsdom
import { createApp, nextTick } from 'vue'
import ElementPlus from 'element-plus'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('../../api/cluster', () => ({
  DEFAULT_INSTALL_PARAMS: {
    namespace: 'kubesentry-system',
    webhookImage: { repository: 'webhook', tag: 'local', pullPolicy: 'IfNotPresent' },
    operatorImage: { repository: 'operator', tag: 'local', pullPolicy: 'IfNotPresent' },
    highAvailability: false,
    gatewayAddress: '',
  },
  listClusters: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 }),
  listInstallRecords: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 }),
  createCluster: vi.fn(),
  deleteCluster: vi.fn(),
  uninstallCluster: vi.fn(),
}))
vi.mock('../../api/identity', () => ({ listTenants: vi.fn().mockResolvedValue([]) }))

import ClusterListView from '../ClusterListView.vue'
import { authState } from '../../auth/session'
import { i18n } from '../../locales'
import router from '../../router'

describe('cluster creation flow', () => {
  let unmount: (() => void) | undefined

  afterEach(() => {
    unmount?.()
    unmount = undefined
    document.body.replaceChildren()
  })

  it('moves to installation parameters when Enter submits the cluster name', async () => {
    authState.status = 'authenticated'
    authState.me = { id: '1', username: 'admin', role: 'system_admin' }
    await router.push('/clusters')
    await router.isReady()

    const target = document.createElement('div')
    document.body.appendChild(target)
    const app = createApp(ClusterListView).use(router).use(i18n).use(ElementPlus)
    unmount = app.unmount
    app.mount(target)
    await nextTick()

    const createButton = Array.from(target.querySelectorAll<HTMLButtonElement>('button'))
      .find(button => button.textContent?.includes(i18n.global.t('common.create')))
    createButton?.click()
    await nextTick()

    const dialog = document.body.querySelector('.el-dialog')
    const nameInput = dialog?.querySelector<HTMLInputElement>('input')
    nameInput!.value = 'local'
    nameInput!.dispatchEvent(new Event('input', { bubbles: true }))
    dialog?.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await nextTick()

    expect(dialog?.textContent).toContain(i18n.global.t('cluster.namespace'))
  })
})
