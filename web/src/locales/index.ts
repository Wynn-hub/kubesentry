import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN'
import enUS from './en-US'

const stored = localStorage.getItem('kubesentry-locale')

export const i18n = createI18n({
  legacy: false,
  locale: stored === 'en-US' ? 'en-US' : 'zh-CN',
  fallbackLocale: 'en-US',
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
})

export function setLocale(l: 'zh-CN' | 'en-US') {
  i18n.global.locale.value = l
  localStorage.setItem('kubesentry-locale', l)
}
