import { describe, expect, it } from 'vitest'
import { paginateClientItems } from '../useClientPagination'

interface Item {
  id: string
  rank: number
}

const compareRank = (a: Item, b: Item) => a.rank - b.rank

describe('paginateClientItems', () => {
  it('sorts the complete array before slicing a later page', () => {
    const items = [
      { id: 'c', rank: 3 },
      { id: 'a', rank: 1 },
      { id: 'd', rank: 4 },
      { id: 'b', rank: 2 },
    ]
    const result = paginateClientItems(items, { page: 2, pageSize: 2, sortOrder: 'asc' }, compareRank)
    expect(result.items.map((item) => item.id)).toEqual(['c', 'd'])
    expect(result.total).toBe(4)
  })

  it('supports descending stable sorting', () => {
    const items = [
      { id: 'first', rank: 1 },
      { id: 'second', rank: 1 },
      { id: 'high', rank: 2 },
    ]
    const result = paginateClientItems(items, { page: 1, pageSize: 10, sortOrder: 'desc' }, compareRank)
    expect(result.items.map((item) => item.id)).toEqual(['high', 'first', 'second'])
  })

  it('returns empty pages for empty and out-of-range inputs', () => {
    expect(paginateClientItems([], { page: 1, pageSize: 10, sortOrder: 'asc' }, compareRank).items).toEqual([])
    expect(
      paginateClientItems([{ id: 'a', rank: 1 }], { page: 2, pageSize: 10, sortOrder: 'asc' }, compareRank)
        .items,
    ).toEqual([])
  })

  it('keeps independent collection states separate', () => {
    const items = Array.from({ length: 6 }, (_, rank) => ({ id: String(rank), rank }))
    const first = paginateClientItems(items, { page: 1, pageSize: 2, sortOrder: 'asc' }, compareRank)
    const second = paginateClientItems(items, { page: 2, pageSize: 3, sortOrder: 'asc' }, compareRank)
    expect(first.items.map((item) => item.id)).toEqual(['0', '1'])
    expect(second.items.map((item) => item.id)).toEqual(['3', '4', '5'])
  })
})
