import { describe, expect, it } from 'vitest'
import { normalizeLinkIncidentDetail, normalizeLinkIncidentPage } from '../observability'

describe('link incident API normalization', () => {
  it('normalizes a nullable incident page from an older backend', async () => {
    expect(normalizeLinkIncidentPage({ items: null, total: 0, page: 1, pageSize: 20 })).toMatchObject({ items: [], total: 0 })
  })

  it('normalizes nullable incident samples', async () => {
    expect(normalizeLinkIncidentDetail({
      id: '1', clusterName: 'c', reporterInstanceId: 'i', component: 'webhook', reason: 'queue_full',
      startedAt: '2026-08-24T00:00:00Z', lastSeenAt: '2026-08-24T00:00:00Z', affectedCount: 0,
      status: 'active', detail: '', samples: null,
    })).toMatchObject({ id: '1', samples: [] })
  })
})
