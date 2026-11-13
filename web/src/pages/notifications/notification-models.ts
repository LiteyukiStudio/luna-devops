import type { TFunction } from 'i18next'
import type {
  NotificationChannel,
  NotificationChannelDraftTestPayload,
  NotificationChannelPayload,
  NotificationChannelTargetOverride,
  NotificationPreset,
  NotificationRule,
  NotificationRuleAdvancedFilter,
  NotificationRulePayload,
} from '@/api'
import { z } from 'zod'

export const NOTIFICATION_PAGE_SIZE = 10

export type NotificationEventGroupKey = 'runtime' | 'build' | 'release' | 'gateway' | 'hook'

export const NOTIFICATION_EVENT_GROUPS: ReadonlyArray<{
  key: NotificationEventGroupKey
  events: readonly string[]
}> = [
  {
    key: 'runtime',
    events: [
      'runtime.pod_oom_killed',
      'runtime.pod_crash_loop',
      'runtime.pod_start_failed',
      'runtime.workload_degraded',
      'runtime.recovered',
    ],
  },
  {
    key: 'build',
    events: [
      'build.failed',
      'build.succeeded',
      'build.started',
      'build.webhook_rejected',
    ],
  },
  {
    key: 'release',
    events: [
      'release.failed',
      'release.succeeded',
      'release.started',
      'release.canceled',
    ],
  },
  {
    key: 'gateway',
    events: [
      'gateway.apply_failed',
      'gateway.applied',
      'certificate.failed',
      'certificate.expired',
      'certificate.issued',
      'certificate.renewed',
    ],
  },
  {
    key: 'hook',
    events: [
      'hook.failed',
      'hook.succeeded',
    ],
  },
]

export const ALL_NOTIFICATION_EVENTS: readonly string[] = NOTIFICATION_EVENT_GROUPS.flatMap(group => group.events)

export const RECOMMENDED_NOTIFICATION_EVENTS: readonly string[] = [
  'runtime.pod_oom_killed',
  'runtime.pod_crash_loop',
  'runtime.pod_start_failed',
  'runtime.workload_degraded',
  'runtime.recovered',
  'build.failed',
  'build.webhook_rejected',
  'release.failed',
  'gateway.apply_failed',
  'certificate.failed',
  'certificate.expired',
  'hook.failed',
]

export const COMMON_TIME_ZONES: readonly string[] = [
  'Asia/Shanghai',
  'UTC',
  'Asia/Taipei',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Asia/Singapore',
  'Europe/London',
  'Europe/Berlin',
  'America/New_York',
  'America/Los_Angeles',
]

export function notificationEventGroupLabel(group: NotificationEventGroupKey, t: TFunction): string {
  switch (group) {
    case 'runtime': return t('notificationsPage.eventGroups.runtime')
    case 'build': return t('notificationsPage.eventGroups.build')
    case 'release': return t('notificationsPage.eventGroups.release')
    case 'gateway': return t('notificationsPage.eventGroups.gateway')
    case 'hook': return t('notificationsPage.eventGroups.hook')
  }
}

export function notificationEventLabel(eventType: string, t: TFunction): string {
  switch (eventType) {
    case 'runtime.pod_oom_killed': return t('eventsPage.types.runtime_pod_oom_killed')
    case 'runtime.pod_crash_loop': return t('eventsPage.types.runtime_pod_crash_loop')
    case 'runtime.pod_start_failed': return t('eventsPage.types.runtime_pod_start_failed')
    case 'runtime.workload_degraded': return t('eventsPage.types.runtime_workload_degraded')
    case 'runtime.recovered': return t('eventsPage.types.runtime_recovered')
    case 'build.started': return t('eventsPage.types.build_started')
    case 'build.succeeded': return t('eventsPage.types.build_succeeded')
    case 'build.failed': return t('eventsPage.types.build_failed')
    case 'build.webhook_rejected': return t('eventsPage.types.build_webhook_rejected')
    case 'release.started': return t('eventsPage.types.release_started')
    case 'release.succeeded': return t('eventsPage.types.release_succeeded')
    case 'release.failed': return t('eventsPage.types.release_failed')
    case 'release.canceled': return t('eventsPage.types.release_canceled')
    case 'gateway.applied': return t('eventsPage.types.gateway_applied')
    case 'gateway.apply_failed': return t('eventsPage.types.gateway_apply_failed')
    case 'certificate.issued': return t('eventsPage.types.certificate_issued')
    case 'certificate.renewed': return t('eventsPage.types.certificate_renewed')
    case 'certificate.failed': return t('eventsPage.types.certificate_failed')
    case 'certificate.expired': return t('eventsPage.types.certificate_expired')
    case 'hook.succeeded': return t('eventsPage.types.hook_succeeded')
    case 'hook.failed': return t('eventsPage.types.hook_failed')
    default: return eventType
  }
}

export function presetSecretFieldLabel(field: string, t: TFunction): string {
  switch (field) {
    case 'webhookUrl': return t('notificationsPage.webhookUrl')
    case 'signSecret': return t('notificationsPage.signSecret')
    case 'appUrl': return t('notificationsPage.presetAppUrl')
    case 'token': return t('notificationsPage.presetToken')
    default: return field
  }
}

export type NotificationChannelMode = 'preset' | 'webhook' | 'smtp'

export const notificationChannelFormSchema = z.object({
  mode: z.enum(['preset', 'webhook', 'smtp']),
  scope: z.enum(['project', 'global']),
  projectIds: z.array(z.string()),
  name: z.string(),
  enabled: z.boolean(),
  presetId: z.string(),
  presetSecrets: z.record(z.string(), z.string()),
  webhookMethod: z.enum(['POST', 'PUT']),
  webhookUrlCustom: z.string(),
  webhookHeadersJson: z.string(),
  webhookBodyTemplate: z.string(),
  extraSecretsText: z.string(),
  smtpHost: z.string(),
  smtpPort: z.number().int().min(1).max(65535),
  smtpSecurity: z.enum(['starttls', 'tls', 'none']),
  smtpUsername: z.string(),
  smtpPassword: z.string(),
  smtpFrom: z.string(),
  smtpToText: z.string(),
  smtpCcText: z.string(),
  testRecipient: z.string(),
})

export type NotificationChannelFormValues = z.infer<typeof notificationChannelFormSchema>

export function defaultChannelFormValues(options: {
  channel?: NotificationChannel
  defaultMode?: NotificationChannelMode
  defaultScope?: 'project' | 'global'
  defaultProjectIds?: string[]
  presets: NotificationPreset[]
}): NotificationChannelFormValues {
  const {
    channel,
    defaultMode = 'preset',
    defaultScope = 'project',
    defaultProjectIds = [],
    presets,
  } = options
  const firstPresetId = presets[0]?.id ?? 'feishu-bot'
  if (!channel) {
    return {
      mode: presets.length > 0 ? defaultMode : (defaultMode === 'preset' ? 'webhook' : defaultMode),
      scope: defaultScope,
      projectIds: [...defaultProjectIds],
      name: '',
      enabled: true,
      presetId: firstPresetId,
      presetSecrets: {},
      webhookMethod: 'POST',
      webhookUrlCustom: '',
      webhookHeadersJson: '{\n  "Content-Type": "application/json"\n}',
      webhookBodyTemplate: '',
      extraSecretsText: '',
      smtpHost: '',
      smtpPort: 587,
      smtpSecurity: 'starttls',
      smtpUsername: '',
      smtpPassword: '',
      smtpFrom: '',
      smtpToText: '',
      smtpCcText: '',
      testRecipient: '',
    }
  }

  const parsedConfig = parseChannelConfigMap(channel)
  const mode: NotificationChannelMode = channel.presetId
    ? 'preset'
    : channel.adapterKind === 'smtp' ? 'smtp' : 'webhook'
  const rawSecurity = typeof parsedConfig.security === 'string' ? parsedConfig.security.toLowerCase() : 'starttls'
  const smtpSecurity: 'starttls' | 'tls' | 'none' = rawSecurity === 'tls' || rawSecurity === 'none' ? rawSecurity : 'starttls'
  const rawPort = typeof parsedConfig.port === 'number' ? parsedConfig.port : Number(parsedConfig.port)
  const smtpPort = Number.isInteger(rawPort) && rawPort > 0 && rawPort <= 65535 ? rawPort : (smtpSecurity === 'tls' ? 465 : 587)
  const rawMethod = typeof parsedConfig.method === 'string' && parsedConfig.method.toUpperCase() === 'PUT' ? 'PUT' : 'POST'
  const rawHeaders = parsedConfig.headers && typeof parsedConfig.headers === 'object' && !Array.isArray(parsedConfig.headers)
    ? parsedConfig.headers
    : { 'Content-Type': 'application/json' }

  return {
    mode,
    scope: channel.scope === 'global' ? 'global' : 'project',
    projectIds: channel.projectIds?.length ? [...channel.projectIds] : [...defaultProjectIds],
    name: channel.name,
    enabled: channel.enabled,
    presetId: channel.presetId || firstPresetId,
    presetSecrets: {},
    webhookMethod: rawMethod,
    webhookUrlCustom: typeof parsedConfig.url === 'string' ? parsedConfig.url : '',
    webhookHeadersJson: JSON.stringify(rawHeaders, null, 2),
    webhookBodyTemplate: typeof parsedConfig.jsonBodyTemplate === 'string' ? parsedConfig.jsonBodyTemplate : '',
    extraSecretsText: '',
    smtpHost: typeof parsedConfig.host === 'string' ? parsedConfig.host : '',
    smtpPort,
    smtpSecurity,
    smtpUsername: typeof parsedConfig.username === 'string' ? parsedConfig.username : '',
    smtpPassword: '',
    smtpFrom: typeof parsedConfig.from === 'string' ? parsedConfig.from : '',
    smtpToText: joinStringList(parsedConfig.to),
    smtpCcText: joinStringList(parsedConfig.cc),
    testRecipient: '',
  }
}

export function parseStringHeadersJson(value: string): Record<string, string> | null {
  const trimmed = value.trim()
  if (!trimmed)
    return {}
  const parsed = parseJsonRecord(trimmed)
  if (!parsed)
    return null
  const out: Record<string, string> = {}
  for (const [key, entry] of Object.entries(parsed)) {
    if (typeof entry !== 'string')
      return null
    if (key.trim())
      out[key.trim()] = entry
  }
  return out
}

export function buildScopedChannelPayload(
  values: NotificationChannelFormValues,
  existingChannel: NotificationChannel | undefined,
  presets: NotificationPreset[],
): NotificationChannelPayload | null {
  const scope = values.scope
  const projectIds = scope === 'project' ? normalizedStringList(values.projectIds) : []
  if (scope === 'project' && projectIds.length === 0)
    return null

  const existingSecretSet = existingChannel?.secretSet ?? {}

  if (values.mode === 'preset') {
    const preset = presets.find(item => item.id === values.presetId)
    if (!preset)
      return null
    const secrets: Record<string, string> = {}
    for (const field of preset.secretFields) {
      const provided = (values.presetSecrets[field] ?? '').trim()
      if (provided) {
        secrets[field] = provided
      }
      else if (!existingSecretSet[field]) {
        return null
      }
    }
    for (const field of preset.optionalSecretFields ?? []) {
      const provided = (values.presetSecrets[field] ?? '').trim()
      if (provided)
        secrets[field] = provided
    }
    return {
      name: values.name.trim() || preset.name,
      scope,
      projectIds,
      presetId: preset.id,
      adapterKind: preset.adapterKind,
      enabled: values.enabled,
      secrets,
    }
  }

  const name = values.name.trim()
  if (!name)
    return null

  if (values.mode === 'webhook') {
    const url = values.webhookUrlCustom.trim()
    const headers = parseStringHeadersJson(values.webhookHeadersJson)
    if (!url || !headers)
      return null
    const config: Record<string, unknown> = {
      method: values.webhookMethod,
      url,
      headers,
    }
    if (values.webhookBodyTemplate.trim())
      config.jsonBodyTemplate = values.webhookBodyTemplate.trim()
    return {
      name,
      scope,
      projectIds,
      adapterKind: 'webhook',
      config,
      secrets: parseKeyValueLines(values.extraSecretsText),
      enabled: values.enabled,
    }
  }

  const host = values.smtpHost.trim()
  const from = values.smtpFrom.trim()
  const password = values.smtpPassword.trim()
  if (!host || !from || !Number.isInteger(values.smtpPort) || values.smtpPort < 1 || values.smtpPort > 65535)
    return null
  const secrets: Record<string, string> = {}
  if (password)
    secrets.password = password
  const to = splitAddressList(values.smtpToText)
  const cc = splitAddressList(values.smtpCcText)
  const config: Record<string, unknown> = {
    host,
    port: values.smtpPort,
    security: values.smtpSecurity,
    username: values.smtpUsername.trim(),
    from,
    ...(to.length > 0 ? { to } : {}),
    ...(cc.length > 0 ? { cc } : {}),
  }
  return {
    name,
    scope,
    projectIds,
    adapterKind: 'smtp',
    config,
    secrets,
    enabled: values.enabled,
  }
}

export function buildChannelDraftTestPayload(
  values: NotificationChannelFormValues,
  existingChannel: NotificationChannel | undefined,
  presets: NotificationPreset[],
): NotificationChannelDraftTestPayload | null {
  const payload = buildScopedChannelPayload(
    {
      ...values,
      name: values.name.trim() || 'Draft test',
      projectIds: values.scope === 'project' && values.projectIds.length === 0 ? ['draft'] : values.projectIds,
    },
    existingChannel,
    presets,
  )
  if (!payload)
    return null
  return {
    ...(existingChannel ? { channelId: existingChannel.id } : {}),
    scope: payload.scope,
    presetId: payload.presetId,
    adapterKind: payload.adapterKind,
    config: payload.config,
    secrets: payload.secrets,
    ...(values.testRecipient.trim() ? { testRecipient: values.testRecipient.trim() } : {}),
  }
}

export function channelHasDefaultSmtpRecipients(channel: NotificationChannel): boolean {
  if (channel.adapterKind !== 'smtp')
    return true
  const parsed = parseChannelConfigMap(channel)
  const recipients = Array.isArray(parsed.to)
    ? parsed.to.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
    : []
  return recipients.length > 0
}

export const notificationRuleTargetDraftSchema = z.object({
  toText: z.string(),
  ccText: z.string(),
})

export const notificationRuleFormSchema = z.object({
  name: z.string(),
  enabled: z.boolean(),
  eventTypes: z.array(z.string()),
  scope: z.enum(['projects', 'all']),
  projectIds: z.array(z.string()),
  applicationIds: z.array(z.string()),
  deploymentTargetIds: z.array(z.string()),
  filterText: z.string(),
  channelIds: z.array(z.string()),
  channelTargets: z.record(z.string(), notificationRuleTargetDraftSchema),
  templateId: z.string(),
  locale: z.string(),
  timeZone: z.string(),
  remindIntervalMinutes: z.number().int().min(0).max(10080),
  maxReminders: z.number().int().min(0).max(100),
  notifyOnRecovery: z.boolean(),
})

export type NotificationRuleFormValues = z.infer<typeof notificationRuleFormSchema>

export function defaultRuleFormValues(options: {
  contextMode: 'project' | 'global'
  fixedProjectId?: string
  rule?: NotificationRule
}): NotificationRuleFormValues {
  const { contextMode, fixedProjectId = '', rule } = options
  if (!rule) {
    return {
      name: '',
      enabled: true,
      eventTypes: [...RECOMMENDED_NOTIFICATION_EVENTS],
      scope: 'projects',
      projectIds: contextMode === 'project' && fixedProjectId ? [fixedProjectId] : [],
      applicationIds: [],
      deploymentTargetIds: [],
      filterText: '{}',
      channelIds: [],
      channelTargets: {},
      templateId: '',
      locale: '',
      timeZone: 'Asia/Shanghai',
      remindIntervalMinutes: 60,
      maxReminders: 3,
      notifyOnRecovery: true,
    }
  }

  const storedFilter = parseJsonRecord(rule.filterJson)
  const parsedTargets = decodeChannelTargets(rule.channelTargetsJson)
  const channelTargets: Record<string, { toText: string, ccText: string }> = {}
  for (const [channelId, target] of Object.entries(parsedTargets)) {
    channelTargets[channelId] = {
      toText: (target.to ?? []).join(', '),
      ccText: (target.cc ?? []).join(', '),
    }
  }

  if (!storedFilter) {
    return {
      name: rule.name,
      enabled: rule.enabled,
      eventTypes: decodeStringList(rule.eventTypesJson),
      scope: 'projects',
      projectIds: contextMode === 'project' && fixedProjectId ? [fixedProjectId] : [],
      applicationIds: [],
      deploymentTargetIds: [],
      filterText: rule.filterJson,
      channelIds: decodeStringList(rule.channelIdsJson),
      channelTargets,
      templateId: rule.templateId ?? '',
      locale: rule.locale ?? '',
      timeZone: rule.timeZone || 'Asia/Shanghai',
      remindIntervalMinutes: rule.remindIntervalMinutes ?? 60,
      maxReminders: rule.maxReminders ?? 3,
      notifyOnRecovery: rule.notifyOnRecovery ?? true,
    }
  }

  const advancedFilter = { ...storedFilter }
  delete advancedFilter.scope
  delete advancedFilter.projectIds
  const projectIds = optionalStringList(storedFilter.projectIds) ?? []
  const applicationIds = optionalStringList(storedFilter.applicationIds) ?? []
  const deploymentTargetIds = optionalStringList(storedFilter.deploymentTargetIds) ?? []

  return {
    name: rule.name,
    enabled: rule.enabled,
    eventTypes: decodeStringList(rule.eventTypesJson),
    scope: storedFilter.scope === 'all' ? 'all' : 'projects',
    projectIds: contextMode === 'project' && fixedProjectId ? [fixedProjectId] : projectIds,
    applicationIds,
    deploymentTargetIds,
    filterText: JSON.stringify(advancedFilter, null, 2),
    channelIds: decodeStringList(rule.channelIdsJson),
    channelTargets,
    templateId: rule.templateId ?? '',
    locale: rule.locale ?? '',
    timeZone: rule.timeZone || 'Asia/Shanghai',
    remindIntervalMinutes: rule.remindIntervalMinutes ?? 60,
    maxReminders: rule.maxReminders ?? 3,
    notifyOnRecovery: rule.notifyOnRecovery ?? true,
  }
}

export function smtpChannelMissingRuleRecipient(
  channel: NotificationChannel,
  targetDraft: { toText: string, ccText: string } | undefined,
): boolean {
  if (channel.adapterKind !== 'smtp')
    return false
  if (channel.canManage === false)
    return false
  if (channelHasDefaultSmtpRecipients(channel))
    return false
  return splitAddressList(targetDraft?.toText ?? '').length === 0
}

export function buildNotificationRulePayload(options: {
  availableChannels: NotificationChannel[]
  contextMode: 'project' | 'global'
  fixedProjectId?: string
  values: NotificationRuleFormValues
}): NotificationRulePayload | null {
  const { availableChannels, contextMode, fixedProjectId = '', values } = options
  const name = values.name.trim()
  const eventTypes = normalizedStringList(values.eventTypes)
  const channelIds = normalizedStringList(values.channelIds)
  if (!name || eventTypes.length === 0 || channelIds.length === 0)
    return null
  if (
    !Number.isInteger(values.remindIntervalMinutes)
    || values.remindIntervalMinutes < 0
    || values.remindIntervalMinutes > 10080
    || !Number.isInteger(values.maxReminders)
    || values.maxReminders < 0
    || values.maxReminders > 100
  ) {
    return null
  }

  const channelById = new Map(availableChannels.map(channel => [channel.id, channel]))
  const channelTargets: Record<string, NotificationChannelTargetOverride> = {}
  for (const channelId of channelIds) {
    const channel = channelById.get(channelId)
    const draft = values.channelTargets[channelId]
    if (channel && smtpChannelMissingRuleRecipient(channel, draft))
      return null
    if (!draft)
      continue
    const to = splitAddressList(draft.toText)
    const cc = splitAddressList(draft.ccText)
    if (to.length > 0 || cc.length > 0) {
      channelTargets[channelId] = {
        ...(to.length > 0 ? { to } : {}),
        ...(cc.length > 0 ? { cc } : {}),
      }
    }
  }

  let filter: NotificationRulePayload['filter']
  if (contextMode === 'project') {
    if (!fixedProjectId)
      return null
    const applicationIds = normalizedStringList(values.applicationIds)
    const deploymentTargetIds = normalizedStringList(values.deploymentTargetIds)
    filter = {
      scope: 'projects',
      projectIds: [fixedProjectId],
      ...(applicationIds.length > 0 ? { applicationIds } : {}),
      ...(deploymentTargetIds.length > 0 ? { deploymentTargetIds } : {}),
    }
  }
  else {
    const advancedFilter = parseNotificationRuleAdvancedFilter(values.filterText)
    const projectIds = normalizedStringList(values.projectIds)
    if (!advancedFilter || (values.scope === 'projects' && projectIds.length === 0))
      return null
    filter = values.scope === 'all'
      ? { ...advancedFilter, scope: 'all' }
      : { ...advancedFilter, projectIds, scope: 'projects' }
  }

  return {
    name,
    enabled: values.enabled,
    eventTypes,
    filter,
    channelIds,
    ...(Object.keys(channelTargets).length > 0 ? { channelTargets } : {}),
    templateId: contextMode === 'global' ? values.templateId.trim() : '',
    locale: values.locale.trim(),
    timeZone: values.timeZone.trim() || 'Asia/Shanghai',
    remindIntervalMinutes: values.remindIntervalMinutes,
    maxReminders: values.maxReminders,
    notifyOnRecovery: values.notifyOnRecovery,
  }
}

export function parseNotificationRuleAdvancedFilter(value: string): NotificationRuleAdvancedFilter | null {
  const parsed = parseJsonRecord(value)
  if (!parsed)
    return null
  const allowedKeys = new Set(['scope', 'projectIds', 'severities', 'applicationIds', 'deploymentTargetIds'])
  if (Object.keys(parsed).some(key => !allowedKeys.has(key)))
    return null
  if (parsed.scope !== undefined && parsed.scope !== 'projects' && parsed.scope !== 'all')
    return null
  if (parsed.projectIds !== undefined && optionalStringList(parsed.projectIds) === null)
    return null
  const severities = optionalStringList(parsed.severities)
  const applicationIds = optionalStringList(parsed.applicationIds)
  const deploymentTargetIds = optionalStringList(parsed.deploymentTargetIds)
  if (severities === null || applicationIds === null || deploymentTargetIds === null)
    return null
  if (severities?.some(severity => !['info', 'warning', 'error'].includes(severity)))
    return null
  return {
    ...(severities === undefined ? {} : { severities }),
    ...(applicationIds === undefined ? {} : { applicationIds }),
    ...(deploymentTargetIds === undefined ? {} : { deploymentTargetIds }),
  }
}

export function hasAdvancedFilter(value: string): boolean {
  const parsed = parseJsonRecord(value)
  return parsed !== null && Object.keys(parsed).length > 0
}

export function decodeStringList(value: string | undefined): string[] {
  if (!value)
    return []
  try {
    const parsed = JSON.parse(value) as unknown
    if (!Array.isArray(parsed))
      return []
    return parsed.filter((item): item is string => typeof item === 'string')
  }
  catch {
    return []
  }
}

export function decodeChannelTargets(value: string | undefined): Record<string, NotificationChannelTargetOverride> {
  const parsed = value ? parseJsonRecord(value) : null
  if (!parsed)
    return {}
  const out: Record<string, NotificationChannelTargetOverride> = {}
  for (const [channelId, rawTarget] of Object.entries(parsed)) {
    if (!rawTarget || typeof rawTarget !== 'object' || Array.isArray(rawTarget))
      continue
    const record = rawTarget as Record<string, unknown>
    const to = optionalStringList(record.to) ?? undefined
    const cc = optionalStringList(record.cc) ?? undefined
    if ((to && to.length > 0) || (cc && cc.length > 0))
      out[channelId] = { ...(to ? { to } : {}), ...(cc ? { cc } : {}) }
  }
  return out
}

export function splitAddressList(value: string): string[] {
  return normalizedStringList(value.split(/[\n,;]+/))
}

export function parseKeyValueLines(value: string): Record<string, string> {
  return Object.fromEntries(value.split('\n').map((line) => {
    const index = line.indexOf('=')
    return index < 0 ? ['', ''] : [line.slice(0, index).trim(), line.slice(index + 1).trim()]
  }).filter(([key, val]) => key && val))
}

export function formatSnapshotBlock(value: string | undefined): string {
  const trimmed = (value ?? '').trim()
  if (!trimmed)
    return ''
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  }
  catch {
    return trimmed
  }
}

export const notificationTemplateFormSchema = z.object({
  name: z.string(),
  eventType: z.string(),
  adapterKind: z.enum(['webhook', 'smtp']),
  locale: z.string(),
  subjectTemplate: z.string(),
  bodyTemplate: z.string(),
  jsonBodyTemplate: z.string(),
  enabled: z.boolean(),
})

export type NotificationTemplateFormValues = z.infer<typeof notificationTemplateFormSchema>

function parseChannelConfigMap(channel: NotificationChannel): Record<string, unknown> {
  if (channel.config && typeof channel.config === 'object' && !Array.isArray(channel.config))
    return channel.config as Record<string, unknown>
  return parseJsonRecord(channel.configJson) ?? {}
}

function parseJsonRecord(value: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(value) as unknown
    return parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed as Record<string, unknown>
      : null
  }
  catch {
    return null
  }
}

function optionalStringList(value: unknown): string[] | null | undefined {
  if (value === undefined)
    return undefined
  if (!Array.isArray(value) || value.some(item => typeof item !== 'string'))
    return null
  return normalizedStringList(value as string[])
}

function normalizedStringList(values: string[]): string[] {
  return [...new Set(values.map(value => value.trim()).filter(Boolean))]
}

function joinStringList(value: unknown): string {
  if (!Array.isArray(value))
    return ''
  return value.filter((item): item is string => typeof item === 'string' && item.trim().length > 0).join(', ')
}
