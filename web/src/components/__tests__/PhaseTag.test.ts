// @vitest-environment jsdom

import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import { afterEach, describe, expect, it } from 'vitest'
import PhaseTag from '../PhaseTag.vue'
import zhCN from '../../locales/zh-CN'

let unmount: (() => void) | undefined

afterEach(() => {
  unmount?.()
  unmount = undefined
  document.body.innerHTML = ''
})

function render(phase: string, kind?: 'policy' | 'group') {
  const target = document.createElement('div')
  document.body.appendChild(target)
  const app = createApp(PhaseTag, { phase, kind })
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.mount(target)
  unmount = app.unmount
  return target.textContent
}

describe('PhaseTag', () => {
  it('uses domain-specific labels for policy and policy group phases', () => {
    expect(render('Ready', 'policy')).toContain('校验通过')
    unmount?.()
    expect(render('Invalid', 'group')).toContain('异常')
  })
})
