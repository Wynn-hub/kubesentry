import { describe, expect, it } from 'vitest'
import { computeMembership } from '../groupMembership'

describe('computeMembership', () => {
  it('removes a byName-sourced member: goes to exclude, drops from byName', () => {
    const result = computeMembership({
      initialResolvedNames: ['a', 'b'],
      initialByNameNames: ['a', 'b'],
      initialExclude: [],
      finalSelectedNames: ['a'],
      modeOverrides: {},
    })
    expect(result.byName).toEqual([{ name: 'a', enforcementMode: undefined }])
    expect(result.exclude).toEqual(['b'])
  })

  it('removes a bySelector-sourced member: goes to exclude without ever touching byName', () => {
    // Reproduces the original bug: 'b' only ever matched via bySelector.
    const result = computeMembership({
      initialResolvedNames: ['a', 'b'],
      initialByNameNames: ['a'],
      initialExclude: [],
      finalSelectedNames: ['a'],
      modeOverrides: {},
    })
    expect(result.byName).toEqual([{ name: 'a', enforcementMode: undefined }])
    expect(result.exclude).toEqual(['b'])
  })

  it('re-selecting a previously excluded member clears it from exclude and adds it to byName', () => {
    const result = computeMembership({
      initialResolvedNames: ['a'],
      initialByNameNames: ['a'],
      initialExclude: ['b'],
      finalSelectedNames: ['a', 'b'],
      modeOverrides: {},
    })
    expect(result.byName).toEqual([
      { name: 'a', enforcementMode: undefined },
      { name: 'b', enforcementMode: undefined },
    ])
    expect(result.exclude).toEqual([])
  })

  it('setting a mode override on an unchanged bySelector member pins it into byName', () => {
    const result = computeMembership({
      initialResolvedNames: ['a', 'b'],
      initialByNameNames: ['a'],
      initialExclude: [],
      finalSelectedNames: ['a', 'b'],
      modeOverrides: { b: 'enforce' },
    })
    expect(result.byName).toEqual([
      { name: 'a', enforcementMode: undefined },
      { name: 'b', enforcementMode: 'enforce' },
    ])
    expect(result.exclude).toEqual([])
  })

  it('leaves an unchanged bySelector member untouched (no byName, no exclude)', () => {
    const result = computeMembership({
      initialResolvedNames: ['a', 'b'],
      initialByNameNames: ['a'],
      initialExclude: [],
      finalSelectedNames: ['a', 'b'],
      modeOverrides: {},
    })
    expect(result.byName).toEqual([{ name: 'a', enforcementMode: undefined }])
    expect(result.exclude).toEqual([])
  })
})
