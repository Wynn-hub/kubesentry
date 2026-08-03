import { afterEach, describe, expect, it, vi } from 'vitest'
import { getLinkIncident, listLinkIncidents } from '../observability'
import { request } from '../http'

vi.mock('../http', () => ({ request: vi.fn() }))

describe('link incident API normalization', () => {
  afterEach(() => vi.resetAllMocks())

  it('normalizes a nullable incident page from an older backend', async () => {
    vi.mocked(request).mockResolvedValue({ items: null, total: 0, page: 1, pageSize: 20 })

    await expect(listLinkIncidents()).resolves.toMatchObject({ items: [], total: 0 })
  })

  it('normalizes nullable incident samples', async () => {
    vi.mocked(request).mockResolvedValue({ id: '1', samples: null })

    await expect(getLinkIncident('1')).resolves.toMatchObject({ id: '1', samples: [] })
  })
})
