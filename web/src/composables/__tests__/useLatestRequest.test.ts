import { describe, expect, it } from 'vitest'
import { useLatestRequest } from '../useLatestRequest'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('useLatestRequest', () => {
  it('allows only the latest request to commit and finish loading', async () => {
    const latest = useLatestRequest()
    const first = deferred<string>()
    const second = deferred<string>()
    const committed: string[] = []

    const firstRun = latest.run(() => first.promise, (value) => committed.push(value))
    const secondRun = latest.run(() => second.promise, (value) => committed.push(value))
    first.resolve('old')
    await expect(firstRun).resolves.toBe(false)
    expect(committed).toEqual([])
    expect(latest.loading.value).toBe(true)

    second.resolve('new')
    await expect(secondRun).resolves.toBe(true)
    expect(committed).toEqual(['new'])
    expect(latest.loading.value).toBe(false)
  })

  it('suppresses stale errors but exposes the latest error', async () => {
    const latest = useLatestRequest()
    const stale = deferred<string>()
    const current = deferred<string>()
    const staleRun = latest.run(() => stale.promise, () => undefined)
    const currentRun = latest.run(() => current.promise, () => undefined)

    stale.reject(new Error('stale'))
    await expect(staleRun).resolves.toBe(false)
    current.reject(new Error('current'))
    await expect(currentRun).rejects.toThrow('current')
    expect(latest.loading.value).toBe(false)
  })
})
