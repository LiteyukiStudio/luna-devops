import type { InboxDecision, InboxListParams, InboxMessage, InboxUnreadCount, PaginatedResponse } from '../types'
import { paginationQuery, request } from '../core'

function inboxQuery(params: InboxListParams) {
  const search = new URLSearchParams(paginationQuery(params))
  search.set('filter', params.filter)
  if (params.category)
    search.set('category', params.category)
  return search.toString()
}

export const inboxApi = {
  listInboxMessages: (params: InboxListParams) =>
    request<PaginatedResponse<InboxMessage>>(`/inbox?${inboxQuery(params)}`),
  getInboxMessage: (messageId: string) =>
    request<InboxMessage>(`/inbox/${encodeURIComponent(messageId)}`),
  getInboxUnreadCount: () => request<InboxUnreadCount>('/inbox/unread-count'),
  markInboxMessageRead: (messageId: string) =>
    request<void>(`/inbox/${encodeURIComponent(messageId)}/read`, { method: 'POST' }),
  markAllInboxMessagesRead: () => request<void>('/inbox/read-all', { method: 'POST' }),
  archiveInboxMessage: (messageId: string) =>
    request<void>(`/inbox/${encodeURIComponent(messageId)}/archive`, { method: 'POST' }),
  decideInboxActionRequest: (requestId: string, decision: InboxDecision, expectedVersion: number) =>
    request<void>(`/inbox/action-requests/${encodeURIComponent(requestId)}/decision`, {
      method: 'POST',
      body: JSON.stringify({ decision, expectedVersion }),
    }),
}
