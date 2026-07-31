import { describe, expect, it } from 'vitest'
import type { PlatformTrendPoint } from '../../api/platform'
import { platformLineSegments, platformMetricText } from '../platformTrend'

const point = (memoryBytes: number): PlatformTrendPoint => ({
  bucketStartedAt: '2026-07-30T00:00:00Z',
  memoryBytes,
  cpuUsageMicros: 0,
  goroutines: 0,
  gcPauseMicros: 0,
})

describe('platform trend presentation', () => {
  it('renders unavailable values as an em dash', () => {
    expect(platformMetricText(-1)).toBe('—')
    expect(platformMetricText(12, 'm')).toBe('12m')
  })

  it('breaks SVG lines around unavailable samples', () => {
    expect(platformLineSegments([point(10), point(-1), point(20)], 'memoryBytes', 100, 50))
      .toEqual(['M 0.00 25.00', 'M 100.00 0.00'])
  })
})
