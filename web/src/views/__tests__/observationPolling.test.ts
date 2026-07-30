import { describe, expect, it, vi } from 'vitest'
import { createObservationPoller } from '../observationPolling'

describe('admission observation polling', () => {
  it('uses separate 10, 30, and 60 second refresh tiers', () => {
    const intervals: number[] = []
    const poller = createObservationPoller(
      [
        { interval: 10_000, run: vi.fn() },
        { interval: 10_000, run: vi.fn() },
        { interval: 30_000, run: vi.fn() },
        { interval: 60_000, run: vi.fn() },
      ],
      (_run, interval) => { intervals.push(interval); return intervals.length },
      vi.fn(),
    )
    poller.start(false)
    expect(intervals).toEqual([10_000, 10_000, 30_000, 60_000])
  })

  it('does not schedule work while the page is hidden and cancels active timers', () => {
    const schedule = vi.fn(() => 7)
    const cancel = vi.fn()
    const poller = createObservationPoller([{ interval: 10_000, run: vi.fn() }], schedule, cancel)
    poller.start(true)
    expect(schedule).not.toHaveBeenCalled()
    poller.start(false)
    poller.stop()
    expect(cancel).toHaveBeenCalledWith(7)
  })
})
