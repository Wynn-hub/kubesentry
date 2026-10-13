import { describe, expect, it } from 'vitest'
import { helpContent } from '../helpContent'

describe('help center content', () => {
  it('keeps complete Chinese and English sections in sync', () => {
    const chinese = helpContent['zh-CN']
    const english = helpContent['en-US']

    expect(chinese.sections.map(({ id }) => id)).toEqual(english.sections.map(({ id }) => id))
    expect(chinese.sections.length).toBeGreaterThanOrEqual(8)
    for (const section of [...chinese.sections, ...english.sections]) {
      expect(section.title).not.toBe('')
      expect(section.paragraphs.length + section.items.length).toBeGreaterThan(0)
    }
  })

  it('documents the network boundary and security-rule reference', () => {
    for (const content of Object.values(helpContent)) {
      expect(content.sections.some(({ id }) => id === 'network')).toBe(true)
      expect(content.sections.some(({ id }) => id === 'security-reference')).toBe(true)
    }
  })
})
