import { describe, expect, it } from 'vitest'
import { apiClient } from '../generated/client'

function captureURL(response = { code: 0, message: 'success', data: { items: [], total: 0, page: 1, pageSize: 20 } }) {
  let received: Request | undefined
  return {
    fetch: async (request: Request) => {
      received = request
      return new Response(JSON.stringify(response), { headers: { 'Content-Type': 'application/json' } })
    },
    url: () => new URL(received!.url),
  }
}

describe('generated client query serialization', () => {
  it('serializes multi-value admission filters as one comma-separated form value', async () => {
    const capture = captureURL()
    await apiClient.GET('/admission-events', {
      params: { query: { cluster: ['prod', 'staging'], verdict: ['audit', 'deny'], namespace: ['default', 'system'], gvk: ['apps/v1/deployments', 'v1/pods'], policy: 'baseline', page: 2 } },
      baseUrl: 'https://console.test/api/v1',
      fetch: capture.fetch,
    })

    const url = capture.url()
    expect(url.searchParams.getAll('cluster')).toEqual(['prod,staging'])
    expect(url.searchParams.getAll('verdict')).toEqual(['audit,deny'])
    expect(url.searchParams.getAll('namespace')).toEqual(['default,system'])
    expect(url.searchParams.getAll('gvk')).toEqual(['apps/v1/deployments,v1/pods'])
    expect(url.searchParams.get('policy')).toBe('baseline')
    expect(url.searchParams.get('page')).toBe('2')
  })

  it('serializes multi-value policy filters as one comma-separated form value', async () => {
    const capture = captureURL()
    await apiClient.GET('/policies', {
      params: { query: { phase: ['Ready', 'Invalid'], mode: ['enforce', 'audit'], source: ['builtin', 'custom'], keyword: 'privileged' } },
      baseUrl: 'https://console.test/api/v1',
      fetch: capture.fetch,
    })

    const url = capture.url()
    expect(url.searchParams.getAll('phase')).toEqual(['Ready,Invalid'])
    expect(url.searchParams.getAll('mode')).toEqual(['enforce,audit'])
    expect(url.searchParams.getAll('source')).toEqual(['builtin,custom'])
    expect(url.searchParams.get('keyword')).toBe('privileged')
  })
})
