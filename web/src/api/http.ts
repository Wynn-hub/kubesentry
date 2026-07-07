import axios, { type AxiosError } from 'axios'

export interface Envelope<T> {
  success: boolean
  data: T
  error: string | null
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public data?: unknown,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export function unwrap<T>(status: number, env: Envelope<T> | undefined, fallbackMsg: string): T {
  if (env && env.success) return env.data
  throw new ApiError(status, env?.error ?? fallbackMsg, env?.data)
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
