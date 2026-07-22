import { describe, expect, it } from 'vitest'
import {
  buildListStateQuery,
  parseListState,
  type ListStateSpec,
} from '../useRouteListState'

const spec: ListStateSpec = {
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name', 'phase'],
}

describe('route list state', () => {
  it('parses defaults and supported values', () => {
    expect(parseListState({}, spec)).toEqual({ page: 1, pageSize: 20, sortBy: 'name', sortOrder: 'asc' })
    expect(
      parseListState({ page: '3', pageSize: '50', sortBy: 'phase', sortOrder: 'desc' }, spec),
    ).toEqual({ page: 3, pageSize: 50, sortBy: 'phase', sortOrder: 'desc' })
  })

  it('normalizes invalid values and preserves unrelated query fields', () => {
    expect(
      buildListStateQuery(
        { page: '0', pageSize: '25', sortBy: 'unsafe', sortOrder: 'sideways', keyword: 'pod', tab: 'a' },
        spec,
      ),
    ).toEqual({ page: '1', pageSize: '20', sortBy: 'name', sortOrder: 'asc', keyword: 'pod', tab: 'a' })
  })

  it('uses independent prefixed keys', () => {
    const prefixed = { ...spec, prefix: 'clusters' }
    const query = buildListStateQuery(
      { page: '7', clustersPage: '2', clustersPageSize: '10', clustersSortBy: 'phase' },
      prefixed,
    )
    expect(parseListState(query, prefixed)).toEqual({
      page: 2,
      pageSize: 10,
      sortBy: 'phase',
      sortOrder: 'asc',
    })
    expect(query.page).toBe('7')
  })

  it('updates one prefixed collection without resetting another', () => {
    const clusters = { ...spec, prefix: 'clusters' }
    const query = {
      clustersPage: '2', clustersPageSize: '20', clustersSortBy: 'name', clustersSortOrder: 'asc',
      recordsPage: '4', recordsPageSize: '50', recordsSortBy: 'phase', recordsSortOrder: 'desc',
    }
    expect(buildListStateQuery(query, clusters, { sortBy: 'phase' })).toMatchObject({
      clustersPage: '1', clustersSortBy: 'phase',
      recordsPage: '4', recordsPageSize: '50', recordsSortBy: 'phase', recordsSortOrder: 'desc',
    })
  })

  it('resets page for page-size, sort, and explicit filter changes', () => {
    const query = { page: '4', pageSize: '20', sortBy: 'name', sortOrder: 'asc', keyword: 'old' }
    expect(buildListStateQuery(query, spec, { pageSize: 50 }).page).toBe('1')
    expect(buildListStateQuery(query, spec, { sortBy: 'phase' }).page).toBe('1')
    expect(buildListStateQuery(query, spec, {}, { resetPage: true, extra: { keyword: 'new' } })).toMatchObject({
      page: '1',
      keyword: 'new',
    })
    expect(buildListStateQuery(query, spec, { page: 2 }).page).toBe('2')
  })

  it('preserves filters when restoring a paged list URL', () => {
    expect(
      buildListStateQuery(
        { page: '3', pageSize: '50', sortBy: 'phase', sortOrder: 'desc', source: 'builtin', keyword: 'pod' },
        spec,
        { page: 2 },
      ),
    ).toEqual({
      page: '2', pageSize: '50', sortBy: 'phase', sortOrder: 'desc', source: 'builtin', keyword: 'pod',
    })
  })
})
