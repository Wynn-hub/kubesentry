import axios, { type AxiosError } from 'axios'

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

const client = axios.create({ baseURL: '/api/v1', validateStatus: () => true })

export async function request<T>(
  method: string,
  url: string,
  body?: unknown,
  params?: Record<string, string>,
): Promise<T> {
  try {
    const resp = await client.request<Envelope<T>>({ method, url, data: body, params })
    return unwrap(resp.status, resp.data, `HTTP ${resp.status}`)
  } catch (e) {
    if (e instanceof ApiError) throw e
    const err = e as AxiosError
    throw new ApiError(0, err.message)
  }
}
