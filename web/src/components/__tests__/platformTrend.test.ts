import { describe, expect, it } from 'vitest'
import type { PlatformTrendPoint } from '../../api/platform'
import {
  formatSeriesValue,
  nearestPointIndex,
  platformLineSegments,
  platformMetricsStale,
  platformMetricText,
  seriesPeak,
  seriesY,
} from '../platformTrend'

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

  // A no_data instance keeps the numbers from its last sample, which can be
  // days old. Showing them makes a dead instance look alive and healthy.
  it('suppresses point-in-time metrics once samples go stale', () => {
    expect(platformMetricsStale({ health: 'no_data' })).toBe(true)
    expect(platformMetricsStale({ health: 'ok' })).toBe(false)
    expect(platformMetricsStale({ health: 'degraded' })).toBe(false)
  })

  // Each series is normalized against its own peak, so line height carries no
  // absolute meaning — the hover readout is the only way to get a number, and
  // it has to be in the unit the operator recognizes.
  it('formats each series in its own unit', () => {
    expect(formatSeriesValue('memoryBytes', 52_428_800)).toBe('50.0 MiB')
    // A per-minute CPU delta of 60s of core time is one full core = 1000m.
    expect(formatSeriesValue('cpuUsageMicros', 60_000_000)).toBe('1000m')
    expect(formatSeriesValue('goroutines', 1234)).toBe('1,234')
    expect(formatSeriesValue('gcPauseMicros', 2500)).toBe('2.5 ms')
    expect(formatSeriesValue('memoryBytes', -1)).toBe('—')
  })

  it('reports the window peak per series, ignoring unavailable samples', () => {
    expect(seriesPeak([point(10), point(-1), point(40)], 'memoryBytes')).toBe(40)
    expect(seriesPeak([point(-1)], 'memoryBytes')).toBe(-1)
  })

  // A quiet Go process reports a zero GC-pause delta every minute. Drawing that
  // on the bottom axis made the line invisible and unhoverable, and stacked it
  // on top of every other all-zero series.
  it('draws a series with no variation mid-plot, not on the axis', () => {
    const flat = [point(0), point(0), point(0)]
    expect(seriesY(flat, 'memoryBytes', 1, 200)).toBe(100)
    expect(platformLineSegments(flat, 'memoryBytes', 100, 200))
      .toEqual(['M 0.00 100.00 L 50.00 100.00 L 100.00 100.00'])
  })

  it('still scales a varying series against its own peak', () => {
    const varying = [point(0), point(50), point(100)]
    expect(seriesY(varying, 'memoryBytes', 0, 200)).toBe(200)
    expect(seriesY(varying, 'memoryBytes', 2, 200)).toBe(0)
  })

  it('reports no position for an unavailable sample', () => {
    expect(seriesY([point(-1)], 'memoryBytes', 0, 200)).toBeNull()
  })

  it('snaps a pointer position to the nearest bucket', () => {
    expect(nearestPointIndex(0, 5)).toBe(0)
    expect(nearestPointIndex(1, 5)).toBe(4)
    expect(nearestPointIndex(0.5, 5)).toBe(2)
    // Pointer events fire in the axis gutter too, so out-of-range must clamp
    // rather than produce a negative index.
    expect(nearestPointIndex(-0.3, 5)).toBe(0)
    expect(nearestPointIndex(1.4, 5)).toBe(4)
    expect(nearestPointIndex(0.5, 0)).toBe(-1)
  })
})
