import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import type { PageResult } from '../api/pagination'
import type { SortOrder } from './useRouteListState'

export interface ClientPageState {
  page: number
  pageSize: number
  sortOrder: SortOrder
}

export function paginateClientItems<T>(
  source: readonly T[],
  state: ClientPageState,
  compare: (a: T, b: T) => number,
): PageResult<T> {
  const direction = state.sortOrder === 'desc' ? -1 : 1
  const sorted = source
    .map((item, index) => ({ item, index }))
    .sort((a, b) => direction * compare(a.item, b.item) || a.index - b.index)
    .map(({ item }) => item)
  const start = (state.page - 1) * state.pageSize
  return {
    items: start >= 0 ? sorted.slice(start, start + state.pageSize) : [],
    total: sorted.length,
    page: state.page,
    pageSize: state.pageSize,
  }
}

export function useClientPagination<T>(
  source: MaybeRefOrGetter<readonly T[]>,
  state: MaybeRefOrGetter<ClientPageState>,
  compare: (a: T, b: T) => number,
) {
  return computed(() => paginateClientItems(toValue(source), toValue(state), compare))
}
