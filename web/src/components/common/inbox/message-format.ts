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
  'inbox.runtime.oomKilled.title': 'inbox.catalog.runtimeOomKilled.title',
  'inbox.runtime.crashLoop.title': 'inbox.catalog.runtimeCrashLoop.title',
  'inbox.runtime.startFailed.title': 'inbox.catalog.runtimeStartFailed.title',
  'inbox.runtime.degraded.title': 'inbox.catalog.runtimeDegraded.title',
  'inbox.runtime.recovered.title': 'inbox.catalog.runtimeRecovered.title',
  'inbox.delivery.buildFailed.title': 'inbox.catalog.buildFailed.title',
  'inbox.delivery.releaseFailed.title': 'inbox.catalog.releaseFailed.title',
  'inbox.delivery.hookFailed.title': 'inbox.catalog.hookFailed.title',
  'inbox.delivery.gatewayFailed.title': 'inbox.catalog.gatewayFailed.title',
  'inbox.delivery.certificateFailed.title': 'inbox.catalog.certificateFailed.title',
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
  'inbox.runtime.anomaly.body': 'inbox.catalog.runtimeAnomaly.content',
  'inbox.runtime.recovered.body': 'inbox.catalog.runtimeRecovered.content',
  'inbox.delivery.buildFailed.body': 'inbox.catalog.buildFailed.content',
  'inbox.delivery.releaseFailed.body': 'inbox.catalog.releaseFailed.content',
  'inbox.delivery.hookFailed.body': 'inbox.catalog.hookFailed.content',
  'inbox.delivery.gatewayFailed.body': 'inbox.catalog.gatewayFailed.content',
  'inbox.delivery.certificateFailed.body': 'inbox.catalog.certificateFailed.content',
}

const dangerTitleKeys = new Set([
  'inbox.delivery.buildFailed.title',
  'inbox.delivery.releaseFailed.title',
  'inbox.delivery.hookFailed.title',
  'inbox.delivery.gatewayFailed.title',
  'inbox.delivery.certificateFailed.title',
  'inbox.runtime.oomKilled.title',
  'inbox.runtime.crashLoop.title',
  'inbox.runtime.startFailed.title',
])

export type InboxMessageVisualKind = 'danger' | 'warning' | 'success' | 'default'

export function inboxMessageVisualKind(message: Pick<InboxMessage, 'actionRequest' | 'priority' | 'titleKey'>): InboxMessageVisualKind {
  if (message.titleKey === 'inbox.runtime.recovered.title')
    return 'success'
  if (dangerTitleKeys.has(message.titleKey) || message.priority === 'critical')
    return 'danger'
  if (message.titleKey === 'inbox.runtime.degraded.title' || message.actionRequest?.status === 'pending')
    return 'warning'
  return 'default'
}

export function stripInlineMarkdown(raw: string, options?: { preserveNewlines?: boolean }): string {
  const cleaned = raw
    .replace(/\r\n?/g, '\n')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/`+([^`]+)`+/g, '$1')
    .replace(/`/g, '')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/(^|\s)\*([^*\s][^*]*)\*(?=\s|$)/g, '$1$2')

  if (options?.preserveNewlines) {
    return cleaned
      .replace(/[^\S\n]+/g, ' ')
      .replace(/ *\n */g, '\n')
      .replace(/\n{3,}/g, '\n\n')
      .trim()
  }

  return cleaned
    .replace(/\s+/g, ' ')
    .trim()
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
