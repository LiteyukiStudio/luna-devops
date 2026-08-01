import type { TFunction } from 'i18next'
import type { InboxMessage } from '@/api'

const titleCatalog: Record<string, string> = {
  'inbox.messages.project.member_added.title': 'inbox.catalog.projectMemberAdded.title',
  'inbox.messages.project.member_role_changed.title': 'inbox.catalog.projectMemberRoleChanged.title',
  'inbox.messages.project.member_removed.title': 'inbox.catalog.projectMemberRemoved.title',
  'inbox.messages.project.billingOwnerTransferRequested.title': 'inbox.catalog.billingTransferRequested.title',
  'inbox.messages.project.billingOwnerTransfer.completed.title': 'inbox.catalog.billingTransferCompleted.title',
  'inbox.messages.project.billingOwnerTransfer.rejected.title': 'inbox.catalog.billingTransferRejected.title',
  'inbox.messages.project.billingOwnerTransfer.cancelled.title': 'inbox.catalog.billingTransferCancelled.title',
  'inbox.system.announcement.title': 'inbox.catalog.systemAnnouncement.title',
}

const contentCatalog: Record<string, string> = {
  'inbox.messages.project.member_added.content': 'inbox.catalog.projectMemberAdded.content',
  'inbox.messages.project.member_role_changed.content': 'inbox.catalog.projectMemberRoleChanged.content',
  'inbox.messages.project.member_removed.content': 'inbox.catalog.projectMemberRemoved.content',
  'inbox.messages.project.billingOwnerTransferRequested.content': 'inbox.catalog.billingTransferRequested.content',
  'inbox.messages.project.billingOwnerTransfer.completed.content': 'inbox.catalog.billingTransferCompleted.content',
  'inbox.messages.project.billingOwnerTransfer.rejected.content': 'inbox.catalog.billingTransferRejected.content',
  'inbox.messages.project.billingOwnerTransfer.cancelled.content': 'inbox.catalog.billingTransferCancelled.content',
  'inbox.system.announcement.content': 'inbox.catalog.systemAnnouncement.content',
}

export function inboxMessageText(message: InboxMessage, t: TFunction) {
  const params = safeTranslationParams(message.params)
  const titleKey = titleCatalog[message.titleKey]
  const contentKey = contentCatalog[message.contentKey]
  const resource = message.resourceId || message.projectId

  return {
    title: titleKey ? t(titleKey, params) : t('inbox.fallback.title'),
    content: contentKey
      ? t(contentKey, params)
      : resource
        ? `${t('inbox.fallback.content')} ${t('inbox.fallback.resource', { resource })}`
        : t('inbox.fallback.content'),
  }
}

function safeTranslationParams(params: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(params).flatMap(([key, value]) => {
    if (typeof value === 'string' || typeof value === 'number')
      return [[key, value]]
    if (typeof value === 'boolean')
      return [[key, String(value)]]
    return []
  }))
}
