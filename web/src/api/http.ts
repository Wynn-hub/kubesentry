import type { components, operations } from './generated/console-api'
import { i18n } from '../locales'

type OperationResponse<Operation extends keyof operations> =
  operations[Operation]['responses'][200] extends { content: { 'application/json': infer Response } }
    ? Response
    : never

export type OperationData<Operation extends keyof operations> =
  OperationResponse<Operation> extends { data?: infer Data } ? Data : undefined

type GeneratedClientResult<Operation extends keyof operations> = {
  response: Response
  data?: OperationResponse<Operation>
  error?: components['schemas']['dto.SwagErrorResponse']
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: number,
    public errorCode?: string,
    public data?: components['schemas']['dto.SwagErrorResponse']['data'],
    public details?: components['schemas']['dto.SwagErrorResponse']['details'],
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

function localizedMessage(errorCode: string | undefined, fallback: string) {
  if (!errorCode) return fallback
  const key = `errors.${errorCode}`
  return i18n.global.te(key) ? i18n.global.t(key) : fallback
}

let unauthorizedHandler: (() => void) | undefined

export function setUnauthorizedHandler(handler: () => void) {
  unauthorizedHandler = handler
}

// A non-zero code is the single failure signal — an HTTP 200 carrying
// code 50000 is still an error, so the code is checked rather than the status.
export function unwrap<Operation extends keyof operations>(
  status: number,
  env: OperationResponse<Operation> | components['schemas']['dto.SwagErrorResponse'] | undefined,
  fallbackMsg: string,
): OperationData<Operation> {
  const envelope = env as { code?: number; message?: string } | undefined
  if (envelope?.code === 0) {
    return (env as OperationResponse<Operation> & { data?: OperationData<Operation> }).data as OperationData<Operation>
  }
  const error = env as components['schemas']['dto.SwagErrorResponse'] | undefined
  throw new ApiError(
    status,
    localizedMessage(error?.errorCode, envelope?.message ?? fallbackMsg),
    envelope?.code,
    error?.errorCode,
    error?.data,
    error?.details,
  )
}

export async function extract<Operation extends keyof operations>(
  result: Promise<GeneratedClientResult<Operation>>,
): Promise<OperationData<Operation>> {
  try {
    const response = await result
    if (response.response.status === 401) unauthorizedHandler?.()
    return unwrap<Operation>(response.response.status, response.data ?? response.error, `HTTP ${response.response.status}`)
  } catch (e) {
    if (e instanceof ApiError) throw e
    const err = e as Error
    throw new ApiError(0, err.message)
  }
}
