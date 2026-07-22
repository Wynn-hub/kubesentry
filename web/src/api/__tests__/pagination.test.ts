import { describe, expect, it, vi } from 'vitest'
import { collectAllPages, type PageResult } from '../pagination'

const page = <T>(items: T[], total: number, currentPage = 1): PageResult<T> => ({
  items,
  total,
  page: currentPage,
  pageSize: 100,
})

describe('collectAllPages', () => {
  it('collects a single page and requests the maximum page size', async () => {
    const fetchPage = vi.fn(async () => page(['a', 'b'], 2))
    await expect(collectAllPages(fetchPage)).resolves.toEqual(['a', 'b'])
    expect(fetchPage).toHaveBeenCalledWith({ page: '1', pageSize: '100' })
  })

  it('collects multiple pages until the initial total is reached', async () => {
    const fetchPage = vi.fn(async (params: Record<string, string>) =>
      params.page === '1' ? page(['a', 'b'], 3) : page(['c'], 3, 2),
    )
    await expect(collectAllPages(fetchPage)).resolves.toEqual(['a', 'b', 'c'])
    expect(fetchPage).toHaveBeenCalledTimes(2)
  })

  it('returns an empty array for an empty result', async () => {
    await expect(collectAllPages(async () => page([], 0))).resolves.toEqual([])
  })

  it('stops with an error when total changes between pages', async () => {
    const fetchPage = vi.fn(async (params: Record<string, string>) =>
      params.page === '1' ? page(['a'], 2) : page(['b'], 3, 2),
    )
    await expect(collectAllPages(fetchPage)).rejects.toThrow('total changed')
    expect(fetchPage).toHaveBeenCalledTimes(2)
  })

  it('stops with an error when a page is empty before total is reached', async () => {
    const fetchPage = vi.fn(async (params: Record<string, string>) =>
      params.page === '1' ? page(['a'], 2) : page([], 2, 2),
    )
    await expect(collectAllPages(fetchPage)).rejects.toThrow('empty page')
    expect(fetchPage).toHaveBeenCalledTimes(2)
  })

  it.each([
    { ...page(['a'], 1), page: 2 },
    { ...page(['a'], 1), pageSize: 20 },
    { ...page(['a'], 1), total: -1 },
    { ...page(['a'], 1), items: null },
  ])('rejects an anomalous response %#', async (response) => {
    await expect(collectAllPages(async () => response as PageResult<string>)).rejects.toThrow(
      'Invalid pagination response',
    )
  })
})
