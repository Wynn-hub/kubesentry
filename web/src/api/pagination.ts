export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

type PageQuery = { page: number; pageSize: 10 | 20 | 50 | 100 }
export type PageFetcher<T> = (params: PageQuery) => Promise<PageResult<T>>

const collectionPageSize = 100

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0
}

function assertPageResult<T>(value: PageResult<T>, requestedPage: number): void {
  if (
    !value ||
    !Array.isArray(value.items) ||
    !isNonNegativeInteger(value.total) ||
    value.page !== requestedPage ||
    value.pageSize !== collectionPageSize ||
    value.items.length > collectionPageSize
  ) {
    throw new Error('Invalid pagination response')
  }
}

export async function collectAllPages<T>(fetchPage: PageFetcher<T>): Promise<T[]> {
  const items: T[] = []
  let expectedTotal: number | undefined

  for (let page = 1; ; page += 1) {
    const result = await fetchPage({ page, pageSize: collectionPageSize })
    assertPageResult(result, page)

    if (expectedTotal === undefined) {
      expectedTotal = result.total
    } else if (result.total !== expectedTotal) {
      throw new Error('Pagination response total changed while collecting pages')
    }

    if (result.items.length === 0) {
      if (items.length === expectedTotal) return items
      throw new Error('Pagination response returned an empty page before total was reached')
    }

    items.push(...result.items)
    if (items.length === expectedTotal) return items
    if (items.length > expectedTotal) {
      throw new Error('Invalid pagination response')
    }
  }
}
