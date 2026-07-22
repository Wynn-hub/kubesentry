import { describe, expect, it } from 'vitest'
import { normalizeSummary } from '../summary'

const liveSummaryPayload = {
  totals: {
    exceptions: 1,
    policies: 37,
    policygroups: 1,
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
})
