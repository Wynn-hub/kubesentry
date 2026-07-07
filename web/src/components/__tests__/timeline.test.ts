import { describe, expect, it } from 'vitest'
import { rollbackActionFor } from '../timeline'

const tl = (cursor: number, prevEnabled: boolean, nextEnabled: boolean, inFlight = false) => ({
  cursor, prevEnabled, nextEnabled, inFlight,
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
