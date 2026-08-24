import type { components, operations } from './generated/console-api'

type OperationResponse<Operation extends keyof operations> =
  operations[Operation]['responses'][200] extends { content: { 'application/json': infer Response } }
    ? Response
    : never

export type OperationData<Operation extends keyof operations> =
  OperationResponse<Operation> extends { data?: infer Data } ? Data : undefined

type GeneratedEnvelope =
  { [Operation in keyof operations]: OperationResponse<Operation> }[keyof operations]
  | components['schemas']['ErrorResponse']

type GeneratedClientResult = {
  response: Response
  data?: GeneratedEnvelope
  error?: components['schemas']['ErrorResponse']
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: number,
    public data?: components['schemas']['ErrorResponse']['data'],
    public details?: components['schemas']['ErrorResponse']['details'],
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

// A non-zero code is the single failure signal — an HTTP 200 carrying
// code 50000 is still an error, so the code is checked rather than the status.
export function unwrap<Operation extends keyof operations>(
  status: number,
  env: GeneratedEnvelope | undefined,
  fallbackMsg: string,
): OperationData<Operation> {
  if (env && env.code === 0) {
    return (env as OperationResponse<Operation> & { data?: OperationData<Operation> }).data as OperationData<Operation>
  }
  const error = env as components['schemas']['ErrorResponse'] | undefined
  throw new ApiError(status, env?.message ?? fallbackMsg, env?.code, error?.data, error?.details)
}

export async function extract<Operation extends keyof operations>(
  result: Promise<GeneratedClientResult>,
): Promise<OperationData<Operation>> {
  try {
    const response = await result
    return unwrap<Operation>(response.response.status, response.data ?? response.error, `HTTP ${response.response.status}`)
  } catch (e) {
    if (e instanceof ApiError) throw e
    const err = e as Error
    throw new ApiError(0, err.message)
  }
}
