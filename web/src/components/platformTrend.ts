import type { PlatformInstance, PlatformTrendPoint } from '../api/platform'

export type PlatformTrendKey = 'memoryBytes' | 'cpuUsageMicros' | 'goroutines' | 'gcPauseMicros'

export function platformMetricText(value: number, unit = ''): string {
  if (value < 0 || !Number.isFinite(value)) return '—'
  return `${value.toLocaleString()}${unit}`
}

// A no_data instance still carries whatever its last sample said, which may be
// days old. Rendering those numbers reads as "this instance is alive and using
// 50 MiB" — exactly the wrong conclusion. Point-in-time metrics are only
// meaningful while samples are fresh, so suppress them alongside the -1
// unavailable marker.
export function platformMetricsStale(instance: Pick<PlatformInstance, 'health'>): boolean {
  return instance.health === 'no_data'
}

// Each series has its own unit and its own magnitude — bytes next to goroutine
// counts — so they cannot share one scale. Every line is normalized against its
// own peak, which means the vertical position shows shape, not value. Reading a
// number requires the tooltip, so formatSeriesValue below is the only place a
// raw figure becomes human-readable.
export function seriesPeak(points: PlatformTrendPoint[], key: PlatformTrendKey): number {
  const available = points.filter(point => point[key] >= 0).map(point => point[key])
  return available.length ? Math.max(...available) : -1
}

// flatBaseline is where a series with no variation is drawn, as a fraction of
// the plot height measured from the top.
//
// A quiet Go process reports a zero GC-pause delta every minute. Normalizing
// that against a peak of zero put the whole line on the bottom axis, where it
// was both invisible and unhoverable — and where it overlapped every other
// all-zero series. Drawing a flat series mid-plot keeps "this metric did not
// move" visible and separable, which is real information, not absence of it.
export const flatBaseline = 0.5

// seriesY is the vertical position of one sample, in plot-local coordinates
// (0 at the top, height at the bottom). Returns null when the value is
// unavailable so callers can break the line rather than plot a fake point.
export function seriesY(
  points: PlatformTrendPoint[],
  key: PlatformTrendKey,
  index: number,
  height: number,
): number | null {
  const value = points[index]?.[key] ?? -1
  if (value < 0) return null
  const peak = seriesPeak(points, key)
  // peak <= 0 means every sample in the window is zero.
  if (peak <= 0) return height * flatBaseline
  return height - (value / peak) * height
}

export function formatSeriesValue(key: PlatformTrendKey, value: number): string {
  if (value < 0 || !Number.isFinite(value)) return '—'
  switch (key) {
    case 'memoryBytes':
      return `${(value / 1024 / 1024).toFixed(1)} MiB`
    case 'cpuUsageMicros':
      // Samples are one minute apart, so the per-minute delta divided by the
      // 60s window gives millicores — the same unit Kubernetes limits use.
      return `${Math.round(value / 60_000)}m`
    case 'goroutines':
      return value.toLocaleString()
    case 'gcPauseMicros':
      return `${(value / 1000).toFixed(1)} ms`
  }
}

// nearestPointIndex maps a pointer position to a bucket. Callers pass the
// fraction across the plot area rather than pixels so the maths stays testable
// without a DOM.
export function nearestPointIndex(fraction: number, count: number): number {
  if (count <= 0) return -1
  const clamped = Math.min(Math.max(fraction, 0), 1)
  return Math.min(count - 1, Math.round(clamped * (count - 1)))
}

export function platformLineSegments(
  points: PlatformTrendPoint[],
  key: PlatformTrendKey,
  width: number,
  height: number,
): string[] {
  const denominator = Math.max(points.length - 1, 1)
  const result: string[] = []
  let current: string[] = []
  points.forEach((point, index) => {
    const y = seriesY(points, key, index, height)
    if (y === null) {
      if (current.length) result.push(current.join(' '))
      current = []
      return
    }
    const x = (index / denominator) * width
    current.push(`${current.length ? 'L' : 'M'} ${x.toFixed(2)} ${y.toFixed(2)}`)
  })
  if (current.length) result.push(current.join(' '))
  return result
}
