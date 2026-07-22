import { describe, expect, it } from 'vitest'
import { pageForCurrentVersion, rollbackActionFor } from '../timeline'

const tl = (cursor: number, prevEnabled: boolean, nextEnabled: boolean, inFlight = false) => ({
  cursor, prevEnabled, nextEnabled, inFlight,
})

describe('pageForCurrentVersion', () => {
  const versions = Array.from({ length: 25 }, (_, index) => ({
    version: index + 1,
    isCurrent: index === 4,
  }))

  it('finds the current version in descending and ascending pages', () => {
    expect(pageForCurrentVersion(versions, 5, 10, 'desc')).toBe(3)
    expect(pageForCurrentVersion(versions, 5, 10, 'asc')).toBe(1)
  })

  it('falls back to the first page when current metadata is missing', () => {
    expect(pageForCurrentVersion([], 99, 10, 'desc')).toBe(1)
  })
})

describe('rollbackActionFor', () => {
  it('prev button only on cursor-1 node', () => {
    expect(rollbackActionFor(1, tl(2, true, false))).toBe('prev')
    expect(rollbackActionFor(2, tl(2, true, false))).toBeNull()
  })

  it('next button only on cursor+1 node', () => {
    expect(rollbackActionFor(2, tl(1, false, true))).toBe('next')
    expect(rollbackActionFor(3, tl(1, false, true))).toBeNull()
  })

  it('no buttons when disabled by server', () => {
    expect(rollbackActionFor(1, tl(2, false, false))).toBeNull()
  })

  it('no buttons while in flight', () => {
    expect(rollbackActionFor(1, tl(2, true, false, true))).toBeNull()
  })
})
