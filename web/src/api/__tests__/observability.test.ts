import { describe, expect, it } from 'vitest'
import {
  normalizeAdmissionObservationOverview,
  normalizeObservationClusterPage,
  normalizeObservationTrend,
  type AdmissionObservationOverview,
  type ObservationClusterPage,
  type ObservationTrend,
} from '../observability'

describe('admission observability response normalization', () => {
  it('normalizes nullable overview collections from an older backend', () => {
    const overview = {
      faultReasons: null,
      windowFaults: null,
      retryCount: null,
      seriesStarted: null,
    } as unknown as AdmissionObservationOverview

    expect(normalizeAdmissionObservationOverview(overview)).toMatchObject({
      faultReasons: [],
      windowFaults: {},
      retryCount: {},
      seriesStarted: {},
    })
  })

  it('normalizes nullable trend and cluster collections', () => {
    const trend = { items: null } as unknown as ObservationTrend
    const page = {
      items: [{
        freshness: null,
        queues: null,
        majorFaultReasons: null,
      }],
    } as unknown as ObservationClusterPage

    expect(normalizeObservationTrend(trend).items).toEqual([])
    expect(normalizeObservationClusterPage(page).items[0]).toMatchObject({
      freshness: {},
      queues: {},
      majorFaultReasons: [],
    })
  })
})
