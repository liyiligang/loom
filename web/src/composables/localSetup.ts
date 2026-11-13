import type { Endpoint, LocalStatus } from '../types'

export function showLocalSetup(status: LocalStatus | undefined, endpoints: readonly Pick<Endpoint, 'local'>[], requested: boolean, remoteSelected: boolean): boolean {
  return Boolean(status && !status.confirmed && (requested || (!remoteSelected && !endpoints.some(endpoint => !endpoint.local))))
}
