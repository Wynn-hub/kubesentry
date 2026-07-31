import type { PlatformTrendPoint } from '../api/platform'

export type PlatformTrendKey = 'memoryBytes' | 'cpuUsageMicros' | 'goroutines' | 'gcPauseMicros'

export function platformMetricText(value: number, unit = ''): string {
  if (value < 0 || !Number.isFinite(value)) return '—'
  return `${value.toLocaleString()}${unit}`
}

export function platformLineSegments(
  points: PlatformTrendPoint[],
  key: PlatformTrendKey,
  width: number,
  height: number,
): string[] {
  const available = points.filter(point => point[key] >= 0)
  const maximum = Math.max(...available.map(point => point[key]), 1)
  const denominator = Math.max(points.length - 1, 1)
  const result: string[] = []
  let current: string[] = []
  points.forEach((point, index) => {
    if (point[key] < 0) {
      if (current.length) result.push(current.join(' '))
      current = []
      return
    }
    const x = (index / denominator) * width
    const y = height - (point[key] / maximum) * height
    current.push(`${current.length ? 'L' : 'M'} ${x.toFixed(2)} ${y.toFixed(2)}`)
  })
  if (current.length) result.push(current.join(' '))
  return result
}
