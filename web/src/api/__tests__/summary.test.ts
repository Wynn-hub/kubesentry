import { describe, expect, it } from 'vitest'
import { normalizeSummary } from '../summary'

const liveSummaryPayload = {
  totals: {
    clusters: 3,
    exceptions: 1,
    policies: 37,
    policygroups: 1,
  },
  clusterPhases: {
    online: 1,
    offline: 1,
    registering: 1,
  },
  groups: [
    {
      name: 'default',
      displayName: 'Default',
      enabled: true,
      resolvedCount: 37,
      enforceCount: 17,
      auditCount: 20,
    },
  ],
}

describe('normalizeSummary', () => {
  it('accepts the current PostgreSQL-backed summary response', () => {
    expect(normalizeSummary(liveSummaryPayload)).toEqual(liveSummaryPayload)
  })

  it('rejects a response without the required totals and groups', () => {
    expect(() => normalizeSummary({ totals: {} })).toThrow('Invalid summary response')
  })

  it('rejects missing or negative cluster statistics', () => {
    const { clusterPhases: _, ...withoutPhases } = liveSummaryPayload
    expect(() => normalizeSummary(withoutPhases)).toThrow('Invalid summary response')
    expect(() =>
      normalizeSummary({
        ...liveSummaryPayload,
        clusterPhases: { ...liveSummaryPayload.clusterPhases, offline: -1 },
      }),
    ).toThrow('Invalid summary response')
  })
})
