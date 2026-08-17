import type { APIMeta } from '../types'
import { request } from '../core'

export const metaApi = {
  getAPIMeta: () => request<APIMeta>('/meta', { cache: 'no-store' }),
}
