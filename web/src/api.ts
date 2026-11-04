export class ApiError extends Error {
  constructor(message: string, public code: string) { super(message) }
}

export async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    method,
    headers: method === 'GET' ? {} : { 'Content-Type': 'application/json' },
    body: method === 'GET' ? undefined : JSON.stringify(body ?? {}),
  })
  const result = await response.json()
  if (!response.ok) throw new ApiError(result.error?.message ?? 'The request failed.', result.error?.code ?? 'request_failed')
  return result as T
}
