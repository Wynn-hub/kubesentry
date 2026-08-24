import { apiClient } from './generated/client'

export interface Envelope<T> {
  code: number
  message: string
  data?: T
  details?: string[]
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: number,
    public data?: unknown,
    public details?: string[],
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

// A non-zero code is the single failure signal — an HTTP 200 carrying
// code 50000 is still an error, so the code is checked rather than the status.
export function unwrap<T>(status: number, env: Envelope<T> | undefined, fallbackMsg: string): T {
  if (env && env.code === 0) return env.data as T
  throw new ApiError(status, env?.message ?? fallbackMsg, env?.code, env?.data, env?.details)
}

export async function request<T>(
  method: string,
  url: string,
  body?: unknown,
  params?: Record<string, string>,
): Promise<T> {
  try {
    // The operation wrappers retain their small legacy signature, while the
    // actual transport is the generated OpenAPI client. The cast is confined
    // to this compatibility boundary because callers supply a dynamic path;
    // generated operation-specific wrappers remain strongly typed.
    const client = apiClient as any
    const options: Record<string, unknown> = {}
    if (body !== undefined) options.body = body
    if (params !== undefined) options.params = { query: params }
    const resp = await client[method.toUpperCase()](url, options)
    return unwrap(resp.response.status, resp.data as Envelope<T>, `HTTP ${resp.response.status}`)
  } catch (e) {
    if (e instanceof ApiError) throw e
    const err = e as Error
    throw new ApiError(0, err.message)
  }
}
