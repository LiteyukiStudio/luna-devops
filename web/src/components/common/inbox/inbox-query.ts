import type { QueryClient } from '@tanstack/react-query'

export const inboxKeys = {
  all: ['inbox'] as const,
  list: (filter: string, page: number, pageSize: number) => ['inbox', 'list', filter, page, pageSize] as const,
  unreadCount: ['inbox', 'unread-count'] as const,
  detail: (messageId: string) => ['inbox', 'detail', messageId] as const,
}

export function invalidateInbox(queryClient: QueryClient) {
  return queryClient.invalidateQueries({ queryKey: inboxKeys.all })
}
