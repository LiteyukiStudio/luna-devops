import type { PaginatedResponse, PlatformEvent, PlatformEventCatalogEntry, PlatformEventListParams } from '../types'
import { paginationQuery, request } from '../core'

function platformEventQuery(params: PlatformEventListParams) {
  const search = new URLSearchParams(paginationQuery(params))
  const filters: Array<[string, string | undefined]> = [
    ['projectId', params.projectId],
    ['applicationId', params.applicationId],
    ['deploymentTargetId', params.deploymentTargetId],
    ['category', params.category],
    ['type', params.type],
    ['severity', params.severity],
    ['status', params.status],
    ['dateFrom', params.dateFrom],
    ['dateTo', params.dateTo],
  ]
  for (const [key, value] of filters) {
    if (value)
      search.set(key, value)
  }
  const multiFilters: Array<[string, string[] | undefined]> = [
    ['projectIds', params.projectIds],
    ['applicationIds', params.applicationIds],
    ['deploymentTargetIds', params.deploymentTargetIds],
    ['categories', params.categories],
    ['types', params.types],
    ['severities', params.severities],
    ['statuses', params.statuses],
  ]
  for (const [key, values] of multiFilters) {
    for (const value of values ?? []) {
      if (value)
        search.append(key, value)
    }
  }
  return search.toString()
}

export const eventsApi = {
  listPlatformEvents: (params: PlatformEventListParams) =>
    request<PaginatedResponse<PlatformEvent>>(`/events?${platformEventQuery(params)}`),
  getPlatformEvent: (eventId: string) =>
    request<PlatformEvent>(`/events/${encodeURIComponent(eventId)}`),
  listPlatformEventCatalog: () =>
    request<PlatformEventCatalogEntry[]>('/events/catalog'),
}
