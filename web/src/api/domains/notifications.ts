import type {
  MailSettings,
  MyNotificationChannel,
  MyNotificationChannelCreatePayload,
  MyNotificationChannelUpdatePayload,
  MyNotificationPreferences,
  MyNotificationPreset,
  NotificationChannel,
  NotificationChannelDraftTestPayload,
  NotificationChannelListParams,
  NotificationChannelPayload,
  NotificationDelivery,
  NotificationDeliveryListParams,
  NotificationPreset,
  NotificationRule,
  NotificationRulePayload,
  NotificationTemplate,
  NotificationTemplatePayload,
  PaginatedResponse,
  PaginationParams,
} from '../types'
import { paginationQuery, request } from '../core'

function notificationChannelQuery(params: NotificationChannelListParams) {
  const search = new URLSearchParams(paginationQuery(params))
  if (params.projectId)
    search.set('projectId', params.projectId)
  if (params.scope)
    search.set('scope', params.scope)
  return search.toString()
}

function notificationDeliveryQuery(params: NotificationDeliveryListParams) {
  const search = new URLSearchParams(paginationQuery(params))
  if (params.status)
    search.set('status', params.status)
  if (params.eventType)
    search.set('eventType', params.eventType)
  if (params.channelId)
    search.set('channelId', params.channelId)
  if (params.ruleId)
    search.set('ruleId', params.ruleId)
  if (params.eventId)
    search.set('eventId', params.eventId)
  if (params.projectId)
    search.set('projectId', params.projectId)
  return search.toString()
}

export const notificationsApi = {
  getMailSettings: () => request<MailSettings>('/mail/settings'),
  updateMailSettings: (payload: Omit<MailSettings, 'passwordSet'> & { password?: string }) =>
    request<MailSettings>('/mail/settings', { method: 'PUT', body: JSON.stringify(payload) }),
  testMailSettings: (recipient: string) =>
    request<{ status: string }>('/mail/settings/test', { method: 'POST', body: JSON.stringify({ recipient }) }),

  getMyNotificationPreferences: () => request<MyNotificationPreferences>('/me/notification-preferences'),
  updateMyNotificationPreferences: (payload: MyNotificationPreferences) =>
    request<MyNotificationPreferences>('/me/notification-preferences', { method: 'PUT', body: JSON.stringify(payload) }),
  listMyNotificationPresets: () => request<MyNotificationPreset[]>('/me/notification-presets'),
  listMyNotificationChannels: (params: PaginationParams) =>
    request<PaginatedResponse<MyNotificationChannel>>(`/me/notification-channels?${paginationQuery(params)}`),
  createMyNotificationChannel: (payload: MyNotificationChannelCreatePayload) =>
    request<MyNotificationChannel>('/me/notification-channels', { method: 'POST', body: JSON.stringify(payload) }),
  updateMyNotificationChannel: (channelId: string, payload: MyNotificationChannelUpdatePayload) =>
    request<MyNotificationChannel>(`/me/notification-channels/${encodeURIComponent(channelId)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteMyNotificationChannel: (channelId: string) =>
    request<void>(`/me/notification-channels/${encodeURIComponent(channelId)}`, { method: 'DELETE' }),
  testMyNotificationChannel: (channelId: string) =>
    request<{ status: string }>(`/me/notification-channels/${encodeURIComponent(channelId)}/test`, { method: 'POST' }),
  listMyNotificationDeliveries: (params: NotificationDeliveryListParams) =>
    request<PaginatedResponse<NotificationDelivery>>(`/me/notification-deliveries?${notificationDeliveryQuery(params)}`),
  retryMyNotificationDelivery: (deliveryId: string) =>
    request<NotificationDelivery>(`/me/notification-deliveries/${encodeURIComponent(deliveryId)}/retry`, { method: 'POST' }),

  listNotificationPresets: () => request<NotificationPreset[]>('/notifications/presets'),
  createNotificationChannelFromPreset: (presetId: string, payload: { name: string, secrets: Record<string, string>, enabled: boolean }) =>
    request<{ channel: NotificationChannel, template: NotificationTemplate }>(`/notifications/presets/${encodeURIComponent(presetId)}/channels`, { method: 'POST', body: JSON.stringify(payload) }),
  listNotificationChannels: (params: NotificationChannelListParams) =>
    request<PaginatedResponse<NotificationChannel>>(`/notifications/channels?${notificationChannelQuery(params)}`),
  createNotificationChannel: (payload: NotificationChannelPayload) =>
    request<NotificationChannel>('/notifications/channels', { method: 'POST', body: JSON.stringify(payload) }),
  updateNotificationChannel: (channelId: string, payload: NotificationChannelPayload) =>
    request<NotificationChannel>(`/notifications/channels/${encodeURIComponent(channelId)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteNotificationChannel: (channelId: string) =>
    request<void>(`/notifications/channels/${encodeURIComponent(channelId)}`, { method: 'DELETE' }),
  testNotificationChannel: (channelId: string, testRecipient?: string) =>
    request<{ status: string }>(`/notifications/channels/${encodeURIComponent(channelId)}/test`, {
      method: 'POST',
      ...(testRecipient ? { body: JSON.stringify({ testRecipient }) } : {}),
    }),
  listNotificationTemplates: (params: PaginationParams) =>
    request<PaginatedResponse<NotificationTemplate>>(`/notifications/templates?${paginationQuery(params)}`),
  createNotificationTemplate: (payload: NotificationTemplatePayload) =>
    request<NotificationTemplate>('/notifications/templates', { method: 'POST', body: JSON.stringify(payload) }),
  updateNotificationTemplate: (templateId: string, payload: NotificationTemplatePayload) =>
    request<NotificationTemplate>(`/notifications/templates/${encodeURIComponent(templateId)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteNotificationTemplate: (templateId: string) =>
    request<void>(`/notifications/templates/${encodeURIComponent(templateId)}`, { method: 'DELETE' }),
  listNotificationRules: (params: PaginationParams) =>
    request<PaginatedResponse<NotificationRule>>(`/notifications/rules?${paginationQuery(params)}`),
  createNotificationRule: (payload: NotificationRulePayload) =>
    request<NotificationRule>('/notifications/rules', { method: 'POST', body: JSON.stringify(payload) }),
  updateNotificationRule: (ruleId: string, payload: NotificationRulePayload) =>
    request<NotificationRule>(`/notifications/rules/${encodeURIComponent(ruleId)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteNotificationRule: (ruleId: string) =>
    request<void>(`/notifications/rules/${encodeURIComponent(ruleId)}`, { method: 'DELETE' }),
  listNotificationDeliveries: (params: NotificationDeliveryListParams) =>
    request<PaginatedResponse<NotificationDelivery>>(`/notifications/deliveries?${notificationDeliveryQuery(params)}`),
  retryNotificationDelivery: (deliveryId: string) =>
    request<NotificationDelivery>(`/notifications/deliveries/${encodeURIComponent(deliveryId)}/retry`, { method: 'POST' }),

  listScopedNotificationPresets: () => request<NotificationPreset[]>('/notification-channels/presets'),
  listScopedNotificationChannels: (params: NotificationChannelListParams) =>
    request<PaginatedResponse<NotificationChannel>>(`/notification-channels?${notificationChannelQuery(params)}`),
  createScopedNotificationChannel: (payload: NotificationChannelPayload) =>
    request<NotificationChannel>('/notification-channels', { method: 'POST', body: JSON.stringify(payload) }),
  updateScopedNotificationChannel: (channelId: string, payload: NotificationChannelPayload) =>
    request<NotificationChannel>(`/notification-channels/${encodeURIComponent(channelId)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteScopedNotificationChannel: (channelId: string) =>
    request<void>(`/notification-channels/${encodeURIComponent(channelId)}`, { method: 'DELETE' }),
  testScopedNotificationChannel: (channelId: string, testRecipient?: string) =>
    request<{ status: string }>(`/notification-channels/${encodeURIComponent(channelId)}/test`, {
      method: 'POST',
      ...(testRecipient ? { body: JSON.stringify({ testRecipient }) } : {}),
    }),
  testDraftScopedNotificationChannel: (payload: NotificationChannelDraftTestPayload) =>
    request<{ status: string }>('/notification-channels/test-draft', { method: 'POST', body: JSON.stringify(payload) }),

  listProjectNotificationChannels: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<NotificationChannel>>(`/projects/${encodeURIComponent(projectId)}/notifications/channels?${paginationQuery(params)}`),
  bindProjectNotificationChannel: (projectId: string, channelId: string) =>
    request<NotificationChannel>(`/projects/${encodeURIComponent(projectId)}/notifications/channels/${encodeURIComponent(channelId)}/bind`, { method: 'POST' }),
  unbindProjectNotificationChannel: (projectId: string, channelId: string) =>
    request<void>(`/projects/${encodeURIComponent(projectId)}/notifications/channels/${encodeURIComponent(channelId)}/unbind`, { method: 'POST' }),
  listProjectNotificationRules: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<NotificationRule>>(`/projects/${encodeURIComponent(projectId)}/notifications/rules?${paginationQuery(params)}`),
  createProjectNotificationRule: (projectId: string, payload: NotificationRulePayload) =>
    request<NotificationRule>(`/projects/${encodeURIComponent(projectId)}/notifications/rules`, { method: 'POST', body: JSON.stringify(payload) }),
  updateProjectNotificationRule: (projectId: string, ruleId: string, payload: NotificationRulePayload) =>
    request<NotificationRule>(`/projects/${encodeURIComponent(projectId)}/notifications/rules/${encodeURIComponent(ruleId)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProjectNotificationRule: (projectId: string, ruleId: string) =>
    request<void>(`/projects/${encodeURIComponent(projectId)}/notifications/rules/${encodeURIComponent(ruleId)}`, { method: 'DELETE' }),
  listProjectNotificationDeliveries: (projectId: string, params: NotificationDeliveryListParams) =>
    request<PaginatedResponse<NotificationDelivery>>(`/projects/${encodeURIComponent(projectId)}/notifications/deliveries?${notificationDeliveryQuery(params)}`),
  retryProjectNotificationDelivery: (projectId: string, deliveryId: string) =>
    request<NotificationDelivery>(`/projects/${encodeURIComponent(projectId)}/notifications/deliveries/${encodeURIComponent(deliveryId)}/retry`, { method: 'POST' }),
}
