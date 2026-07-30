export type ObservationPollTask = {
  interval: number
  run: () => void
}

export function createObservationPoller(
  tasks: ObservationPollTask[],
  schedule: (run: () => void, interval: number) => number = window.setInterval,
  cancel: (timer: number) => void = window.clearInterval,
) {
  let timers: number[] = []
  const stop = () => {
    timers.forEach(timer => cancel(timer))
    timers = []
  }
  const start = (hidden: boolean) => {
    stop()
    if (hidden) return
    timers = tasks.map(task => schedule(task.run, task.interval))
  }
  return { start, stop }
}
