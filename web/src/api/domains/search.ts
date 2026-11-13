import type { SearchQueryParams, SearchResponse } from '../types'
import { request } from '../core'

export const searchApi = {
  searchPlatformResources: (params: SearchQueryParams, signal?: AbortSignal) => {
    const query = new URLSearchParams({ q: params.q })
    if (params.types && params.types.length > 0)
      query.set('types', params.types.join(','))
    if (params.limitPerType != null)
      query.set('limitPerType', String(params.limitPerType))
    if (params.visibility)
      query.set('visibility', params.visibility)
    return request<SearchResponse>(`/search?${query.toString()}`, {
      cache: 'no-store',
      signal,
    })
  },
}
