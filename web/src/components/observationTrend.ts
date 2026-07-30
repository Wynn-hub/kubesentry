import type { ObservationTrendPoint } from '../api/observability'

export type ObservationTrendKey = 'generated' | 'written' | 'faults' | 'policyFiltered'

export function observationTrendMax(points: ObservationTrendPoint[], keys: ObservationTrendKey[]): number {
  let maximum = 0
  for (const point of points) {
    if (!point.present) continue
    for (const key of keys) maximum = Math.max(maximum, point[key])
  }
  return Math.max(maximum, 1)
}

export function observationLineSegments(
  points: ObservationTrendPoint[],
  key: ObservationTrendKey,
  maximum: number,
  width: number,
  height: number,
): string[] {
  const segments: string[] = []
  let current: string[] = []
  const denominator = Math.max(points.length - 1, 1)
  points.forEach((point, index) => {
    if (!point.present) {
      if (current.length) segments.push(current.join(' '))
      current = []
      return
    }
    const x = (index / denominator) * width
    const y = height - (point[key] / Math.max(maximum, 1)) * height
    current.push(`${current.length ? 'L' : 'M'} ${x.toFixed(2)} ${y.toFixed(2)}`)
  })
  if (current.length) segments.push(current.join(' '))
  return segments
}
