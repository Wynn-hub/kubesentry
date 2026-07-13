import { describe, expect, it } from 'vitest'
import { relTime } from '../relTime'

describe('relTime', () => {
  const now = new Date('2026-07-13T12:00:00Z')

  it('formats hours ago', () => {
    expect(relTime('2026-07-13T10:00:00Z', 'en-US', now)).toBe('2 hours ago')
  })

  it('formats days ago in Chinese', () => {
    expect(relTime('2026-07-10T12:00:00Z', 'zh-CN', now)).toBe('3天前')
  })

  it('formats minutes ago', () => {
    expect(relTime('2026-07-13T11:59:00Z', 'en-US', now)).toBe('1 minute ago')
  })

  it('falls back to seconds for very recent times', () => {
    expect(relTime('2026-07-13T11:59:50Z', 'en-US', now)).toBe('10 seconds ago')
  })
})
