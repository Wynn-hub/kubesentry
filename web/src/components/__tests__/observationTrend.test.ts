import { describe, expect, it } from 'vitest'
import { observationLineSegments, observationTrendMax } from '../observationTrend'
import type { ObservationTrendPoint } from '../../api/observability'

const point = (present: boolean, generated: number): ObservationTrendPoint => ({
  bucketStartedAt: '2026-07-30T00:00:00Z',
  present,
  generated,
  operatorReceived: 0,
  gatewayReceived: 0,
  written: 0,
  policyFiltered: 0,
  faults: 0,
})

describe('observation trend geometry', () => {
  it('keeps real zero buckets but breaks the line across missing buckets', () => {
    const paths = observationLineSegments(
      [point(true, 0), point(true, 10), point(false, 0), point(true, 5)],
      'generated',
      10,
      300,
      100,
    )
    expect(paths).toHaveLength(2)
    expect(paths[0]).toContain('M 0.00 100.00')
    expect(paths[0]).toContain('L 100.00 0.00')
    expect(paths[1]).toContain('M 300.00 50.00')
  })

  it('ignores missing points when calculating the scale', () => {
    expect(observationTrendMax([point(true, 4), point(false, 99)], ['generated'])).toBe(4)
    expect(observationTrendMax([], ['generated'])).toBe(1)
  })
})
