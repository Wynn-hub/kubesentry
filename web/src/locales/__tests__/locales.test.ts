import { describe, expect, it } from 'vitest'
import enUS from '../en-US'
import zhCN from '../zh-CN'

function keys(value: object, prefix = ''): string[] {
  return Object.entries(value).flatMap(([key, child]) => {
    const path = prefix ? `${prefix}.${key}` : key
    return child && typeof child === 'object' ? keys(child, path) : [path]
  })
}

describe('locales', () => {
  it('keeps Chinese and English message keys in sync', () => {
    expect(keys(zhCN).sort()).toEqual(keys(enUS).sort())
  })
})
