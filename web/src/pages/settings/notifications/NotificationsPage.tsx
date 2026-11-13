import type {
  NotificationChannel,
  NotificationChannelPayload,
  NotificationDelivery,
  NotificationRule,
  NotificationRulePayload,
  NotificationTemplate,
  NotificationTemplatePayload,
} from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import type { NotificationChannelMode } from '@/pages/notifications/notification-models'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, FileCode2, FlaskConical, MoreHorizontal, Pencil, Plus, RefreshCw, Trash2, Webhook } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { FormField as Field } from '@/components/common/form-field'
import { PageShell } from '@/components/common/page-shell'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { cn } from '@/lib/utils'
import { NotificationChannelDialog } from '@/pages/notifications/notification-channel-dialog'
import { NotificationDeliveriesPanel } from '@/pages/notifications/notification-deliveries-panel'
import {
  ALL_NOTIFICATION_EVENTS,
  decodeStringList,
  NOTIFICATION_PAGE_SIZE,
  notificationEventLabel,
  notificationTemplateFormSchema,
} from '@/pages/notifications/notification-models'
import { NotificationRuleDialog } from '@/pages/notifications/notification-rule-dialog'

export function NotificationsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [initialSearch] = useState(() => new URLSearchParams(window.location.search))
  const [activeTab, setActiveTab] = useState(() => {
    const requestedTab = initialSearch.get('tab')
    if (requestedTab === 'channels' || requestedTab === 'templates' || requestedTab === 'rules' || requestedTab === 'deliveries')
      return requestedTab
    return initialSearch.get('eventId') ? 'deliveries' : 'channels'
  })
  const [eventIdFilter, setEventIdFilter] = useState(() => initialSearch.get('eventId') ?? '')
  const [channelPage, setChannelPage] = useState(1)
  const [templatePage, setTemplatePage] = useState(1)
  const [rulePage, setRulePage] = useState(1)
  const [deliveryPage, setDeliveryPage] = useState(1)
  const [deliveryStatus, setDeliveryStatus] = useState('')
  const [deliveryEventType, setDeliveryEventType] = useState('')
  const [deliveryChannelId, setDeliveryChannelId] = useState('')

  const [channelDialog, setChannelDialog] = useState<{
    channel?: NotificationChannel
    mode: NotificationChannelMode
  } | null>(null)
  const [templateDialog, setTemplateDialog] = useState<{ template?: NotificationTemplate } | null>(null)
  const [ruleDialog, setRuleDialog] = useState<{ rule?: NotificationRule } | null>(null)
  const [testChannelTarget, setTestChannelTarget] = useState<NotificationChannel | null>(null)
  const [testRecipient, setTestRecipient] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<{ id: string, kind: 'channel' | 'rule' | 'template', name: string } | null>(null)

  const presets = useQuery({ queryKey: ['notifications', 'presets'], queryFn: api.listNotificationPresets })
  const channels = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['notifications', 'channels', channelPage],
    queryFn: () => api.listNotificationChannels({ page: channelPage, pageSize: NOTIFICATION_PAGE_SIZE, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const channelOptions = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['notifications', 'channels', 'options'],
    queryFn: () => api.listNotificationChannels({ page: 1, pageSize: 100, sortBy: 'name', sortOrder: 'asc' }),
  })
  const templates = useQuery({
    queryKey: ['notifications', 'templates', templatePage],
    queryFn: () => api.listNotificationTemplates({ page: templatePage, pageSize: NOTIFICATION_PAGE_SIZE, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const templateOptions = useQuery({
    queryKey: ['notifications', 'templates', 'options'],
    queryFn: () => api.listNotificationTemplates({ page: 1, pageSize: 100, sortBy: 'name', sortOrder: 'asc' }),
  })
  const rules = useQuery({
    queryKey: ['notifications', 'rules', rulePage],
    queryFn: () => api.listNotificationRules({ page: rulePage, pageSize: NOTIFICATION_PAGE_SIZE, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const projectOptions = useQuery({
    queryKey: ['projects', 'options', 'all', 'notification-rules'],
    queryFn: () => api.listProjects('all'),
  })
  const deliveries = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['notifications', 'deliveries', deliveryPage, deliveryStatus, deliveryEventType, deliveryChannelId, eventIdFilter],
    queryFn: () => api.listNotificationDeliveries({
      page: deliveryPage,
      pageSize: NOTIFICATION_PAGE_SIZE,
      status: deliveryStatus || undefined,
      eventType: deliveryEventType || undefined,
      channelId: deliveryChannelId || undefined,
      eventId: eventIdFilter || undefined,
      sortBy: 'createdAt',
      sortOrder: 'desc',
    }),
  })

  const globalChannelOptions = useMemo(
    () => (channelOptions.data?.items ?? []).filter(channel => (channel.scope ?? 'global') === 'global'),
    [channelOptions.data?.items],
  )

  const refreshNotifications = () => {
    void queryClient.invalidateQueries({ queryKey: ['notifications'] })
    void queryClient.invalidateQueries({ queryKey: ['scoped-notification-channels'] })
  }

  const clearEventIdFilter = () => {
    setEventIdFilter('')
    const params = new URLSearchParams(window.location.search)
    if (params.has('eventId')) {
      params.delete('eventId')
      const query = params.toString()
      window.history.replaceState(null, '', `${window.location.pathname}${query ? `?${query}` : ''}${window.location.hash}`)
    }
  }

  const saveChannel = useMutation({
    mutationFn: ({ channelId, payload }: { channelId?: string, payload: NotificationChannelPayload }) =>
      channelId
        ? api.updateScopedNotificationChannel(channelId, payload)
        : api.createScopedNotificationChannel(payload),
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setChannelDialog(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const saveTemplate = useMutation({
    mutationFn: ({ payload, templateId }: { payload: NotificationTemplatePayload, templateId?: string }) =>
      templateId ? api.updateNotificationTemplate(templateId, payload) : api.createNotificationTemplate(payload),
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setTemplateDialog(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const saveRule = useMutation({
    mutationFn: ({ payload, ruleId }: { payload: NotificationRulePayload, ruleId?: string }) =>
      ruleId ? api.updateNotificationRule(ruleId, payload) : api.createNotificationRule(payload),
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setRuleDialog(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const deleteChannel = useMutation({
    mutationFn: api.deleteScopedNotificationChannel,
    onSuccess: () => {
      setDeleteTarget(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const deleteTemplate = useMutation({
    mutationFn: api.deleteNotificationTemplate,
    onSuccess: () => {
      setDeleteTarget(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const deleteRule = useMutation({
    mutationFn: api.deleteNotificationRule,
    onSuccess: () => {
      setDeleteTarget(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const testChannel = useMutation({
    mutationFn: ({ channelId, recipient }: { channelId: string, recipient: string }) =>
      api.testNotificationChannel(channelId, recipient.trim() || undefined),
    onSuccess: () => {
      toast.success(t('notificationsPage.testSent'))
      setTestChannelTarget(null)
      setTestRecipient('')
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const retryDelivery = useMutation({
    mutationFn: (delivery: NotificationDelivery) => api.retryNotificationDelivery(delivery.id),
    onSuccess: () => {
      toast.success(t('notificationsPage.retryQueued'))
      void queryClient.invalidateQueries({ queryKey: ['notifications', 'deliveries'] })
    },
    onError: error => toast.error(error.message),
  })

  const channelColumns = useMemo<DataListColumn<NotificationChannel>[]>(() => [
    {
      key: 'name',
      header: t('notificationsPage.name'),
      width: 'primary',
      render: item => (
        <NameCell
          name={item.name}
          sub={item.presetId ? `${item.presetId} · ${item.adapterKind}` : item.adapterKind}
        />
      ),
    },
    {
      key: 'scope',
      header: t('notificationsPage.scope'),
      width: 'compact',
      render: item => item.scope === 'project'
        ? <StatusBadge tone="neutral">{t('notificationsPage.scopeProject')}</StatusBadge>
        : <StatusBadge tone="info">{t('notificationsPage.scopeGlobal')}</StatusBadge>,
    },
    {
      key: 'enabled',
      header: t('common.status'),
      width: 'status',
      render: item => (
        <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>
          {item.enabled ? t('common.enabled') : t('common.disabled')}
        </StatusBadge>
      ),
    },
    {
      key: 'lastDeliveryStatus',
      header: t('notificationsPage.lastDelivery'),
      width: 'normal',
      render: item => item.lastDeliveryStatus
        ? (
            <div className="min-w-0">
              <StatusValueBadge value={item.lastDeliveryStatus} />
              {item.lastDeliveredAt && (
                <p className="mt-0.5 truncate text-xs text-muted-foreground">
                  {formatSmartDateTime(item.lastDeliveredAt, t)}
                </p>
              )}
            </div>
          )
        : '-',
    },
    {
      key: 'secretSet',
      header: t('notificationsPage.secrets'),
      width: 'normal',
      render: item => Object.keys(item.secretSet ?? {}).filter(key => item.secretSet?.[key]).join(', ') || '-',
    },
    {
      key: 'actions',
      header: t('common.actions'),
      className: 'text-right',
      mobileActions: 'inline',
      sticky: 'right',
      width: 'actions',
      render: item => (
        <Actions
          name={item.name}
          onDelete={() => setDeleteTarget({ id: item.id, kind: 'channel', name: item.name })}
          onEdit={() => setChannelDialog({
            channel: item,
            mode: item.presetId ? 'preset' : (item.adapterKind === 'smtp' ? 'smtp' : 'webhook'),
          })}
          onTest={() => {
            setTestRecipient('')
            setTestChannelTarget(item)
          }}
        />
      ),
    },
  ], [t])

  const templateColumns = useMemo<DataListColumn<NotificationTemplate>[]>(() => [
    {
      key: 'name',
      header: t('notificationsPage.name'),
      width: 'primary',
      render: item => <NameCell name={item.name} sub={`${item.eventType} · ${item.adapterKind}`} />,
    },
    {
      key: 'enabled',
      header: t('common.status'),
      width: 'status',
      render: item => (
        <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>
          {item.enabled ? t('common.enabled') : t('common.disabled')}
        </StatusBadge>
      ),
    },
    {
      key: 'locale',
      header: t('notificationsPage.locale'),
      width: 'compact',
      render: item => item.locale || '-',
    },
    {
      key: 'actions',
      header: t('common.actions'),
      className: 'text-right',
      mobileActions: 'inline',
      sticky: 'right',
      width: 'actions',
      render: item => (
        <Actions
          name={item.name}
          onDelete={() => setDeleteTarget({ id: item.id, kind: 'template', name: item.name })}
          onEdit={() => setTemplateDialog({ template: item })}
        />
      ),
    },
  ], [t])

  const ruleColumns = useMemo<DataListColumn<NotificationRule>[]>(() => [
    {
      key: 'name',
      header: t('notificationsPage.name'),
      width: 'primary',
      render: (item) => {
        const events = decodeStringList(item.eventTypesJson)
        const preview = events.slice(0, 3).map(ev => notificationEventLabel(ev, t)).join(' · ')
        return <NameCell name={item.name} sub={`${preview}${events.length > 3 ? ` +${events.length - 3}` : ''}`} />
      },
    },
    {
      key: 'throttling',
      header: t('notificationsPage.throttlingSectionTitle'),
      width: 'normal',
      render: (item) => {
        const max = item.maxReminders ?? 3
        const interval = item.remindIntervalMinutes ?? 60
        return (
          <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <span>
              {max === 0 || interval === 0
                ? t('notificationsPage.throttleOnceSummary')
                : t('notificationsPage.throttleRepeatSummary', { max, minutes: interval })}
            </span>
            {item.notifyOnRecovery !== false && (
              <StatusBadge tone="info">{t('notificationsPage.recoveryBadge')}</StatusBadge>
            )}
          </div>
        )
      },
    },
    {
      key: 'enabled',
      header: t('common.status'),
      width: 'status',
      render: item => (
        <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>
          {item.enabled ? t('common.enabled') : t('common.disabled')}
        </StatusBadge>
      ),
    },
    {
      key: 'channels',
      header: t('notificationsPage.channels'),
      width: 'compact',
      render: item => decodeStringList(item.channelIdsJson).length,
    },
    {
      key: 'actions',
      header: t('common.actions'),
      className: 'text-right',
      mobileActions: 'inline',
      sticky: 'right',
      width: 'actions',
      render: item => (
        <Actions
          name={item.name}
          onDelete={() => setDeleteTarget({ id: item.id, kind: 'rule', name: item.name })}
          onEdit={() => setRuleDialog({ rule: item })}
        />
      ),
    },
  ], [t])

  return (
    <PageShell spacing="compact">
      <ContentTabs
        tabs={[
          { label: t('notificationsPage.channels'), value: 'channels' },
          { label: t('notificationsPage.templates'), value: 'templates' },
          { label: t('notificationsPage.rules'), value: 'rules' },
          { label: t('notificationsPage.deliveries'), value: 'deliveries' },
        ]}
        tools={(
          <>
            {activeTab === 'channels' && (
              <>
                <Button id="notification-channel-templates" variant="outline" onClick={() => setChannelDialog({ mode: 'preset' })}>
                  <Bell className="size-4" />
                  {t('notificationsPage.fromPreset')}
                </Button>
                <Button onClick={() => setChannelDialog({ mode: 'webhook' })}>
                  <Plus className="size-4" />
                  {t('notificationsPage.createChannel')}
                </Button>
              </>
            )}
            {activeTab === 'templates' && (
              <Button onClick={() => setTemplateDialog({})}>
                <Plus className="size-4" />
                {t('notificationsPage.createTemplate')}
              </Button>
            )}
            {activeTab === 'rules' && (
              <Button onClick={() => setRuleDialog({})}>
                <Plus className="size-4" />
                {t('notificationsPage.createRule')}
              </Button>
            )}
            {activeTab === 'deliveries' && (
              <Button
                disabled={deliveries.isFetching}
                type="button"
                variant="outline"
                onClick={() => void deliveries.refetch()}
              >
                <RefreshCw className={cn('size-4', deliveries.isFetching && 'animate-spin')} />
                {t('common.refresh')}
              </Button>
            )}
          </>
        )}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value="channels">
          <DataList
            columns={channelColumns}
            emptyDescription={t('notificationsPage.emptyChannelsDescription')}
            emptyIcon={<Webhook className="size-5" />}
            emptyTitle={t('notificationsPage.emptyChannels')}
            error={channels.isError
              ? {
                  description: t('notificationsPage.loadFailedDescription'),
                  retrying: channels.isFetching,
                  title: t('notificationsPage.loadFailedTitle'),
                  onRetry: () => void channels.refetch(),
                }
              : undefined}
            items={channels.data?.items ?? []}
            loading={channels.isLoading}
            pagination={pagination(channels.data, channelPage, setChannelPage, t)}
            rowKey={item => item.id}
          />
        </TabsContent>
        <TabsContent value="templates">
          <DataList
            columns={templateColumns}
            emptyIcon={<FileCode2 className="size-5" />}
            emptyTitle={t('notificationsPage.emptyTemplates')}
            error={templates.isError
              ? {
                  description: t('notificationsPage.loadFailedDescription'),
                  retrying: templates.isFetching,
                  title: t('notificationsPage.loadFailedTitle'),
                  onRetry: () => void templates.refetch(),
                }
              : undefined}
            items={templates.data?.items ?? []}
            loading={templates.isLoading}
            pagination={pagination(templates.data, templatePage, setTemplatePage, t)}
            rowKey={item => item.id}
          />
        </TabsContent>
        <TabsContent value="rules">
          <DataList
            columns={ruleColumns}
            emptyDescription={t('notificationsPage.emptyRulesDescription')}
            emptyIcon={<Bell className="size-5" />}
            emptyTitle={t('notificationsPage.emptyRules')}
            error={rules.isError
              ? {
                  description: t('notificationsPage.loadFailedDescription'),
                  retrying: rules.isFetching,
                  title: t('notificationsPage.loadFailedTitle'),
                  onRetry: () => void rules.refetch(),
                }
              : undefined}
            items={rules.data?.items ?? []}
            loading={rules.isLoading}
            pagination={pagination(rules.data, rulePage, setRulePage, t)}
            rowKey={item => item.id}
          />
        </TabsContent>
        <TabsContent value="deliveries">
          <NotificationDeliveriesPanel
            canRetry
            channelFilter={deliveryChannelId}
            channels={channelOptions.data?.items ?? []}
            data={deliveries.data}
            error={deliveries.isError}
            eventIdFilter={eventIdFilter}
            eventTypeFilter={deliveryEventType}
            loading={deliveries.isLoading}
            page={deliveryPage}
            retrying={retryDelivery.isPending}
            retryingLoad={deliveries.isFetching}
            statusFilter={deliveryStatus}
            onChannelFilterChange={setDeliveryChannelId}
            onClearEventIdFilter={clearEventIdFilter}
            onEventTypeFilterChange={setDeliveryEventType}
            onPageChange={setDeliveryPage}
            onRetry={delivery => retryDelivery.mutate(delivery)}
            onRetryLoad={() => void deliveries.refetch()}
            onStatusFilterChange={setDeliveryStatus}
          />
        </TabsContent>
      </ContentTabs>

      <NotificationChannelDialog
        allowGlobalScope
        channel={channelDialog?.channel}
        defaultMode={channelDialog?.mode ?? 'preset'}
        defaultScope="global"
        manageableProjects={projectOptions.data ?? []}
        open={Boolean(channelDialog)}
        presets={presets.data ?? []}
        saving={saveChannel.isPending}
        onClose={() => setChannelDialog(null)}
        onSave={(payload, channelId) => saveChannel.mutate({ channelId, payload })}
      />

      {templateDialog && (
        <TemplateDialog
          saving={saveTemplate.isPending}
          template={templateDialog.template}
          onClose={() => setTemplateDialog(null)}
          onSave={(payload, templateId) => saveTemplate.mutate({ payload, templateId })}
        />
      )}

      <NotificationRuleDialog
        channels={globalChannelOptions}
        contextMode="global"
        open={Boolean(ruleDialog)}
        projects={projectOptions.data ?? []}
        rule={ruleDialog?.rule}
        saving={saveRule.isPending}
        templates={templateOptions.data?.items ?? []}
        onClose={() => setRuleDialog(null)}
        onSave={(payload, ruleId) => saveRule.mutate({ payload, ruleId })}
      />

      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('notificationsPage.sendTest')}
        confirmVariant="default"
        content={(
          <div className="grid gap-3">
            {testChannelTarget?.adapterKind === 'smtp' && (
              <Field label={t('notificationsPage.testRecipientLabel')}>
                <Input
                  placeholder={t('notificationsPage.testRecipientPlaceholder')}
                  type="email"
                  value={testRecipient}
                  onChange={event => setTestRecipient(event.target.value)}
                />
              </Field>
            )}
            <TestNotificationVariables />
          </div>
        )}
        description={t('notificationsPage.testChannelDescription', { name: testChannelTarget?.name ?? '' })}
        open={Boolean(testChannelTarget)}
        pending={testChannel.isPending}
        title={t('notificationsPage.testChannelTitle')}
        onConfirm={() => testChannelTarget && testChannel.mutate({ channelId: testChannelTarget.id, recipient: testRecipient })}
        onOpenChange={(open) => {
          if (!open) {
            setTestChannelTarget(null)
            setTestRecipient('')
          }
        }}
      />

      <ConfirmDialog
        cancelText={t('common.cancel')}
        closeOnConfirm={false}
        confirmText={t('common.delete')}
        description={t('notificationsPage.deleteDescription', { name: deleteTarget?.name ?? '' })}
        open={Boolean(deleteTarget)}
        pending={deleteChannel.isPending || deleteTemplate.isPending || deleteRule.isPending}
        title={deleteTarget?.kind === 'rule'
          ? t('notificationsPage.deleteRuleTitle')
          : deleteTarget?.kind === 'template' ? t('notificationsPage.deleteTemplateTitle') : t('notificationsPage.deleteChannelTitle')}
        onConfirm={() => {
          if (deleteTarget?.kind === 'channel')
            deleteChannel.mutate(deleteTarget.id)
          else if (deleteTarget?.kind === 'template')
            deleteTemplate.mutate(deleteTarget.id)
          else if (deleteTarget?.kind === 'rule')
            deleteRule.mutate(deleteTarget.id)
        }}
        onOpenChange={open => !open && setDeleteTarget(null)}
      />
    </PageShell>
  )
}

function TemplateDialog({
  saving,
  template,
  onClose,
  onSave,
}: {
  saving: boolean
  template?: NotificationTemplate
  onClose: () => void
  onSave: (payload: NotificationTemplatePayload, templateId?: string) => void
}) {
  const { t } = useTranslation()
  const form = useForm({
    defaultValues: {
      name: template?.name ?? '',
      eventType: template?.eventType ?? 'build.failed',
      adapterKind: (template?.adapterKind === 'smtp' ? 'smtp' : 'webhook') as 'webhook' | 'smtp',
      locale: template?.locale ?? '',
      subjectTemplate: template?.subjectTemplate ?? '[{{.Event.Severity}}] {{.Event.Type}}',
      bodyTemplate: template?.bodyTemplate ?? '',
      jsonBodyTemplate: template?.jsonBodyTemplate ?? '{\n  "text": "[{{.Event.Severity}}] {{.Event.Type}}\\n{{.Event.Message}}"\n}',
      enabled: template?.enabled ?? true,
    },
    mode: 'onChange',
    resolver: zodResolver(notificationTemplateFormSchema),
  })
  const values = form.watch()

  return (
    <Dialog open onOpenChange={open => !open && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('notificationsPage.templateDialog')}</DialogTitle>
          <DialogDescription>{t('notificationsPage.templateDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-4 md:grid-cols-2"
          onSubmit={form.handleSubmit(submitted => onSave({
            adapterKind: submitted.adapterKind,
            bodyTemplate: submitted.bodyTemplate,
            enabled: submitted.enabled,
            eventType: submitted.eventType,
            jsonBodyTemplate: submitted.jsonBodyTemplate,
            locale: submitted.locale,
            name: submitted.name,
            subjectTemplate: submitted.subjectTemplate,
          }, template?.id))}
        >
          <Field label={t('notificationsPage.name')} required>
            <Input {...form.register('name')} />
          </Field>
          <Field label={t('notificationsPage.eventType')}>
            <Select value={values.eventType} onChange={event => form.setValue('eventType', event.target.value, { shouldDirty: true })}>
              {ALL_NOTIFICATION_EVENTS.map(event => <option key={event} value={event}>{event}</option>)}
            </Select>
          </Field>
          <Field label={t('notificationsPage.adapter')}>
            <Select
              value={values.adapterKind}
              onChange={event => form.setValue('adapterKind', event.target.value === 'smtp' ? 'smtp' : 'webhook', { shouldDirty: true })}
            >
              <option value="webhook">{t('notificationsPage.adapters.webhook')}</option>
              <option value="smtp">{t('notificationsPage.adapters.smtp')}</option>
            </Select>
          </Field>
          <Field label={t('notificationsPage.locale')}>
            <Input {...form.register('locale')} />
          </Field>
          <Field className="md:col-span-2" label={t('notificationsPage.subjectTemplate')}>
            <Input {...form.register('subjectTemplate')} />
          </Field>
          <Field className="md:col-span-2" label={t('notificationsPage.bodyTemplate')}>
            <Textarea {...form.register('bodyTemplate')} className="min-h-28 font-mono text-xs" />
          </Field>
          <Field className="md:col-span-2" label={t('notificationsPage.jsonBodyTemplate')}>
            <Textarea {...form.register('jsonBodyTemplate')} className="min-h-40 font-mono text-xs" />
          </Field>
          <DialogFooter className="md:col-span-2">
            <Button disabled={saving} type="button" variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
            <Button disabled={saving || !values.name.trim()} type="submit">
              {template ? t('common.save') : t('notificationsPage.createTemplate')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function Actions({ name, onDelete, onEdit, onTest }: { name: string, onDelete: () => void, onEdit: () => void, onTest?: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex justify-end">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button aria-label={t('notificationsPage.rowActions', { name })} size="icon" variant="ghost">
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {onTest && (
            <DropdownMenuItem onSelect={onTest}>
              <FlaskConical className="size-4" />
              {t('notificationsPage.testChannel')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={onEdit}>
            <Pencil className="size-4" />
            {t('common.edit')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={onDelete}>
            <Trash2 className="size-4" />
            {t('common.delete')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function TestNotificationVariables() {
  const { t } = useTranslation()
  const variables = [
    '.Event.Type',
    '.Event.Project.Name',
    '.Event.Application.Name',
    '.Event.DeploymentTarget.Name',
    '.Event.Runtime.Reason',
    '.Event.Build.ID',
    '.Event.Release.ID',
    '.Event.Gateway.Domain',
  ]
  return (
    <div className="grid gap-2 text-sm">
      <p className="text-muted-foreground">{t('notificationsPage.testVariablesIntro')}</p>
      <div className="flex flex-wrap gap-1.5">
        {variables.map(variable => (
          <code key={variable} className="rounded-control bg-surface-subtle px-2 py-1 font-mono text-xs text-foreground">
            {variable}
          </code>
        ))}
      </div>
    </div>
  )
}

function NameCell({ name, sub }: { name: string, sub: string }) {
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{name}</span>
      <span className="block truncate text-xs text-muted-foreground">{sub}</span>
    </span>
  )
}

function pagination(
  data: { page?: number, pageSize?: number, total?: number, totalPages?: number } | undefined,
  page: number,
  onPageChange: (page: number) => void,
  t: ReturnType<typeof useTranslation>['t'],
) {
  const total = data?.total ?? 0
  if (total === 0)
    return undefined
  return {
    page: data?.page ?? page,
    pageSize: data?.pageSize ?? NOTIFICATION_PAGE_SIZE,
    total,
    totalPages: data?.totalPages ?? 1,
    pageInfoLabel: t('notificationsPage.pageInfo', {
      page: data?.page ?? page,
      totalPages: data?.totalPages ?? 1,
      total,
    }),
    onPageChange,
  }
}
