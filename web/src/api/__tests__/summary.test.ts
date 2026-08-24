import { describe, expect, it } from 'vitest'
import { normalizeSummary } from '../summary'

const liveSummaryPayload = {
  totals: { clusters: 3, exceptions: 1, policies: 37, policygroups: 1 },
  clusterPhases: { online: 1, offline: 1, registering: 1 },
  groups: [{ name: 'default', displayName: 'Default', enabled: true, resolvedCount: 37, enforceCount: 17, auditCount: 20 }],
}

describe('normalizeSummary', () => {
  it('accepts the contract-compatible summary response', () => {
    expect(normalizeSummary(liveSummaryPayload)).toEqual(liveSummaryPayload)
  })

  it('rejects a malformed summary response', () => {
    expect(() => normalizeSummary({ totals: {} })).toThrow('Invalid summary response')
  })
})
