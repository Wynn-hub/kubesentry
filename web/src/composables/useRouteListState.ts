import { computed, watch, type ComputedRef } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import type { components } from '../api/generated/console-api'

export type SortOrder = 'asc' | 'desc'

export interface ListState {
  page: number
  pageSize: number
  sortBy: string
  sortOrder: SortOrder
}

export interface ListStateSpec {
  prefix?: string
  defaultSortBy: string
  defaultSortOrder: SortOrder
  allowedSort: readonly string[]
  allowedPageSizes?: readonly number[]
}

export interface BuildListStateOptions {
  resetPage?: boolean
  extra?: Record<string, unknown>
}

const defaultPageSizes = [10, 20, 50, 100] as const

function stateKey(spec: ListStateSpec, field: 'page' | 'pageSize' | 'sortBy' | 'sortOrder'): string {
  if (!spec.prefix) return field
  return spec.prefix + field.charAt(0).toUpperCase() + field.slice(1)
}

function queryString(value: unknown): string | undefined {
  if (Array.isArray(value)) return queryString(value[0])
  return typeof value === 'string' ? value : undefined
}

function positiveInteger(value: unknown, fallback: number): number {
  const raw = queryString(value)
  if (!raw || !/^\d+$/.test(raw)) return fallback
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : fallback
}

export function parseListState(query: Record<string, unknown>, spec: ListStateSpec): ListState {
  const allowedPageSizes = spec.allowedPageSizes ?? defaultPageSizes
  const page = positiveInteger(query[stateKey(spec, 'page')], 1)
  const requestedPageSize = positiveInteger(query[stateKey(spec, 'pageSize')], 20)
  const pageSize = allowedPageSizes.includes(requestedPageSize) ? requestedPageSize : 20
  const requestedSortBy = queryString(query[stateKey(spec, 'sortBy')])
  const sortBy = requestedSortBy && spec.allowedSort.includes(requestedSortBy)
    ? requestedSortBy
    : spec.defaultSortBy
  const requestedSortOrder = queryString(query[stateKey(spec, 'sortOrder')])
  const sortOrder = requestedSortOrder === 'asc' || requestedSortOrder === 'desc'
    ? requestedSortOrder
    : spec.defaultSortOrder
  return { page, pageSize, sortBy, sortOrder }
}

export function buildListStateQuery(
  query: Record<string, unknown>,
  spec: ListStateSpec,
  patch: Partial<ListState> = {},
  options: BuildListStateOptions = {},
): Record<string, unknown> {
  const next = { ...parseListState(query, spec), ...patch }
  const changesOrdering = patch.pageSize !== undefined || patch.sortBy !== undefined || patch.sortOrder !== undefined
  if (patch.page === undefined && (changesOrdering || options.resetPage)) next.page = 1

  const result: Record<string, unknown> = { ...query }
  for (const [key, value] of Object.entries(options.extra ?? {})) {
    if (value === undefined || value === null || value === '') delete result[key]
    else result[key] = value
  }
  result[stateKey(spec, 'page')] = String(next.page)
  result[stateKey(spec, 'pageSize')] = String(next.pageSize)
  result[stateKey(spec, 'sortBy')] = next.sortBy
  result[stateKey(spec, 'sortOrder')] = next.sortOrder
  return result
}

type GeneratedListQuery = {
  page?: number
  pageSize?: components['schemas']['PageMeta']['pageSize']
  sortBy?: string
  sortOrder?: SortOrder
}

export function listStateParams<Query extends GeneratedListQuery>(state: ListState): Query {
  return {
    page: state.page,
    pageSize: state.pageSize as components['schemas']['PageMeta']['pageSize'],
    sortBy: state.sortBy,
    sortOrder: state.sortOrder,
  } as Query
}

export function useRouteListState(spec: ListStateSpec): {
  state: ComputedRef<ListState>
  replace: (patch?: Partial<ListState>, options?: BuildListStateOptions) => Promise<unknown>
} {
  const route = useRoute()
  const router = useRouter()
  const state = computed(() => parseListState(route.query, spec))

  watch(
    () => route.query,
    (query) => {
      const normalized = buildListStateQuery(query, spec)
      const fields = ['page', 'pageSize', 'sortBy', 'sortOrder'] as const
      if (fields.some((field) => queryString(query[stateKey(spec, field)]) !== normalized[stateKey(spec, field)])) {
        void router.replace({ query: normalized as LocationQueryRaw })
      }
    },
    { immediate: true },
  )

  const replace = (patch: Partial<ListState> = {}, options: BuildListStateOptions = {}) =>
    router.replace({ query: buildListStateQuery(route.query, spec, patch, options) as LocationQueryRaw })

  return { state, replace }
}
