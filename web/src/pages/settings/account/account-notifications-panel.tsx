import type { TFunction } from 'i18next'
import type { MyNotificationChannel, MyNotificationChannelCreatePayload, MyNotificationChannelUpdatePayload, MyNotificationPreset, NotificationDelivery } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { FlaskConical, MoreHorizontal, Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { CheckboxField, ControlledCheckboxField } from '@/components/common/checkbox-field'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { HelpTip } from '@/components/common/help-tip'
import { PageChromeTools } from '@/components/common/page-chrome'
import { Section } from '@/components/common/section'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { NotificationDeliveryDetailSheet } from '@/pages/notifications/notification-delivery-detail-sheet'
import { SettingsTabSaveButton } from '@/pages/settings/settings-tab-save-button'

const pageSize = 10
const failureEventTypes = [
  'runtime.pod_oom_killed',
  'runtime.pod_crash_loop',
  'runtime.pod_start_failed',
  'runtime.workload_degraded',
  'runtime.recovered',
  'build.failed',
  'build.webhook_rejected',
  'release.failed',
  'hook.failed',
  'gateway.apply_failed',
  'certificate.failed',
  'certificate.expired',
] as const

const preferenceSchema = z.object({
  emailEnabled: z.boolean(),
  eventTypes: z.array(z.string()),
})

const channelSchema = z.object({
  name: z.string().trim().min(1, { error: () => i18next.t('accountPage.notifications.channelNameRequired') }),
  presetId: z.string(),
  webhookUrl: z.string(),
  signSecret: z.string(),
  enabled: z.boolean(),
})

type PreferenceForm = z.infer<typeof preferenceSchema>
type ChannelForm = z.infer<typeof channelSchema>
type SaveChannelRequest
  = | { mode: 'create', payload: MyNotificationChannelCreatePayload }
    | { mode: 'update', channelId: string, payload: MyNotificationChannelUpdatePayload }

export function AccountNotificationsPanel() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [channelPage, setChannelPage] = useState(1)
  const [deliveryPage, setDeliveryPage] = useState(1)
  const [channelDialog, setChannelDialog] = useState<{ channel?: MyNotificationChannel } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<MyNotificationChannel | null>(null)
  const [selectedDeliveryId, setSelectedDeliveryId] = useState<string | null>(null)

  const preferences = useQuery({ queryKey: ['my-notification-preferences'], queryFn: api.getMyNotificationPreferences })
  const presets = useQuery({ queryKey: ['my-notification-presets'], queryFn: api.listMyNotificationPresets })
  const channels = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['my-notification-channels', channelPage],
    queryFn: () => api.listMyNotificationChannels({ page: channelPage, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const deliveries = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['my-notification-deliveries', deliveryPage],
    queryFn: () => api.listMyNotificationDeliveries({ page: deliveryPage, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const preferenceForm = useForm<PreferenceForm>({
    defaultValues: { emailEnabled: true, eventTypes: [...failureEventTypes] },
    mode: 'onChange',
    resolver: zodResolver(preferenceSchema),
  })

  useEffect(() => {
    if (preferences.data)
      preferenceForm.reset(preferences.data)
  }, [preferenceForm, preferences.data])

  const selectedDelivery = useMemo(
    () => (deliveries.data?.items ?? []).find(item => item.id === selectedDeliveryId) ?? null,
    [deliveries.data?.items, selectedDeliveryId],
  )

  const savePreferences = useMutation({
    mutationFn: api.updateMyNotificationPreferences,
    onSuccess: (result) => {
      queryClient.setQueryData(['my-notification-preferences'], result)
      preferenceForm.reset(result)
      toast.success(t('accountPage.notifications.preferencesSaved'))
    },
    onError: error => toast.error(error.message),
  })
  const saveChannel = useMutation({
    mutationFn: (request: SaveChannelRequest) => request.mode === 'update'
      ? api.updateMyNotificationChannel(request.channelId, request.payload)
      : api.createMyNotificationChannel(request.payload),
    onSuccess: () => {
      setChannelDialog(null)
      void queryClient.invalidateQueries({ queryKey: ['my-notification-channels'] })
      toast.success(t('accountPage.notifications.channelSaved'))
    },
    onError: error => toast.error(error.message),
  })
  const deleteChannel = useMutation({
    mutationFn: api.deleteMyNotificationChannel,
    onSuccess: () => {
      setDeleteTarget(null)
      void queryClient.invalidateQueries({ queryKey: ['my-notification-channels'] })
      toast.success(t('accountPage.notifications.channelDeleted'))
    },
    onError: error => toast.error(error.message),
  })
  const testChannel = useMutation({
    mutationFn: api.testMyNotificationChannel,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['my-notification-channels'] })
      void queryClient.invalidateQueries({ queryKey: ['my-notification-deliveries'] })
      toast.success(t('accountPage.notifications.testSent'))
    },
    onError: error => toast.error(error.message),
  })
  const retryDelivery = useMutation({
    mutationFn: (delivery: NotificationDelivery) => api.retryMyNotificationDelivery(delivery.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['my-notification-deliveries'] })
      toast.success(t('notificationsPage.retryQueued'))
    },
    onError: error => toast.error(error.message),
  })

  const channelColumns = useMemo<DataListColumn<MyNotificationChannel>[]>(() => [
    {
      key: 'name',
      header: t('accountPage.notifications.channel'),
      width: 'primary',
      render: item => (
        <div className="min-w-0">
          <p className="truncate font-medium">{item.name}</p>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {item.lastDeliveryError
              ? t('accountPage.notifications.channelDeliveryFailed')
              : t('accountPage.notifications.webhookChannel')}
          </p>
        </div>
      ),
    },
    { key: 'enabled', header: t('common.status'), width: 'status', render: item => <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>{item.enabled ? t('common.enabled') : t('common.disabled')}</StatusBadge> },
    { key: 'lastDeliveryStatus', header: t('accountPage.notifications.lastDelivery'), width: 'status', render: item => item.lastDeliveryStatus ? <StatusValueBadge value={item.lastDeliveryStatus} /> : '-' },
    {
      key: 'actions',
      header: t('common.actions'),
      sticky: 'right',
      width: 'actions',
      className: 'text-right whitespace-nowrap',
      render: item => (
        <div className="flex justify-end">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button aria-label={t('accountPage.notifications.channelActions', { name: item.name })} size="icon" variant="ghost">
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem disabled={testChannel.isPending} onSelect={() => testChannel.mutate(item.id)}>
                <FlaskConical className="size-4" />
                {t('common.test')}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => setChannelDialog({ channel: item })}>
                <Pencil className="size-4" />
                {t('common.edit')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(item)}>
                <Trash2 className="size-4" />
                {t('common.delete')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ),
    },
  ], [t, testChannel])

  const deliveryColumns = useMemo<DataListColumn<NotificationDelivery>[]>(() => [
    {
      key: 'eventType',
      header: t('accountPage.notifications.eventType'),
      width: 'primary',
      render: item => (
        <div className="min-w-0">
          <p className="truncate font-medium">{eventTypeLabel(item.eventType, t)}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {item.channelName || item.recipientSummary || '-'}
          </p>
        </div>
      ),
    },
    { key: 'status', header: t('common.status'), width: 'status', render: item => <StatusValueBadge value={item.status} /> },
    {
      key: 'attemptCount',
      header: t('accountPage.notifications.attempts'),
      width: 'compact',
      render: item => (
        <span className="tabular-nums text-xs text-muted-foreground">
          {t('notificationsPage.attemptsOfMax', { count: item.attemptCount })}
        </span>
      ),
    },
    {
      key: 'createdAt',
      header: t('accountPage.notifications.deliveredAt'),
      width: 'compact',
      render: item => (
        <div className="min-w-0">
          <p className="truncate text-xs text-foreground">{formatSmartDateTime(item.finishedAt ?? item.queuedAt ?? item.createdAt, t)}</p>
          <p className="mt-0.5 truncate tabular-nums text-xs text-muted-foreground">
            {item.durationMillis > 0 ? t('notificationsPage.durationMs', { ms: item.durationMillis }) : '-'}
          </p>
        </div>
      ),
    },
    {
      key: 'actions',
      header: t('common.actions'),
      sticky: 'right',
      width: 'actions',
      className: 'text-right',
      render: (item) => {
        const active = item.status === 'pending' || item.status === 'sending'
        return (
          <div className="flex justify-end">
            <Button
              aria-label={t('notificationsPage.retryDelivery')}
              disabled={retryDelivery.isPending || active}
              size="sm"
              type="button"
              variant="ghost"
              onClick={(event) => {
                event.stopPropagation()
                retryDelivery.mutate(item)
              }}
            >
              <RotateCw className="size-3.5" />
              <span className="sr-only sm:not-sr-only">{t('notificationsPage.retryDelivery')}</span>
            </Button>
          </div>
        )
      },
    },
  ], [retryDelivery, t])

  if (preferences.isError)
    return <ErrorState title={t('accountPage.notifications.loadFailedTitle')} description={t('accountPage.notifications.loadFailedDescription')} />

  return (
    <div className="grid gap-4">
      <PageChromeTools>
        <SettingsTabSaveButton
          disabled={!preferenceForm.formState.isDirty || !preferenceForm.formState.isValid}
          label={t('accountPage.notifications.savePreferences')}
          pending={savePreferences.isPending}
          type="button"
          onClick={() => void preferenceForm.handleSubmit(values => savePreferences.mutate(values))()}
        />
      </PageChromeTools>
      <Section title={<SectionTitle description={t('accountPage.notifications.preferencesDescription')} title={t('accountPage.notifications.preferencesTitle')} />} variant="bordered">
        <form className="grid gap-4" onSubmit={preferenceForm.handleSubmit(values => savePreferences.mutate(values))}>
          <Field hint={t('accountPage.notifications.accountEmailHint')} label={t('accountPage.notifications.accountEmail')}>
            <Input readOnly value={user?.email ?? ''} type="email" />
          </Field>
          <Controller
            control={preferenceForm.control}
            name="emailEnabled"
            render={({ field }) => (
              <ControlledCheckboxField description={t('accountPage.notifications.emailEnabledHint')} field={field}>
                {t('accountPage.notifications.emailEnabled')}
              </ControlledCheckboxField>
            )}
          />
          <Controller
            control={preferenceForm.control}
            name="eventTypes"
            render={({ field }) => (
              <fieldset className="grid gap-3 @lg/workspace:grid-cols-2">
                <legend className="mb-3 text-sm font-medium">{t('accountPage.notifications.failureEvents')}</legend>
                {failureEventTypes.map(eventType => (
                  <CheckboxField
                    key={eventType}
                    checked={field.value.includes(eventType)}
                    name={field.name}
                    onBlur={field.onBlur}
                    onCheckedChange={(checked) => {
                      field.onChange(checked === true
                        ? [...field.value, eventType]
                        : field.value.filter(value => value !== eventType))
                    }}
                  >
                    {eventTypeLabel(eventType, t)}
                  </CheckboxField>
                ))}
              </fieldset>
            )}
          />
        </form>
      </Section>

      <Section
        title={<SectionTitle description={t('accountPage.notifications.channelsDescription')} title={t('accountPage.notifications.channelsTitle')} />}
        tools={(
          <Button disabled={presets.isLoading || presets.isError || !presets.data?.length} size="sm" type="button" variant="outline" onClick={() => setChannelDialog({})}>
            <Plus className="size-4" />
            {t('accountPage.notifications.createChannel')}
          </Button>
        )}
        variant="bordered"
      >
        {channels.isError
          ? <ErrorState title={t('accountPage.notifications.channelsLoadFailedTitle')} description={t('accountPage.notifications.channelsLoadFailedDescription')} />
          : (
              <DataList
                columns={channelColumns}
                emptyDescription={t('accountPage.notifications.emptyChannelsDescription')}
                emptyTitle={t('accountPage.notifications.emptyChannels')}
                framed={false}
                items={channels.data?.items ?? []}
                loading={channels.isLoading}
                pagination={pageFor(channels.data, channelPage, setChannelPage, t)}
                rowKey={item => item.id}
              />
            )}
      </Section>

      <Section title={<SectionTitle description={t('accountPage.notifications.deliveriesDescription')} title={t('accountPage.notifications.deliveriesTitle')} />} variant="bordered">
        {deliveries.isError
          ? <ErrorState title={t('accountPage.notifications.deliveriesLoadFailedTitle')} description={t('accountPage.notifications.deliveriesLoadFailedDescription')} />
          : (
              <DataList
                columns={deliveryColumns}
                emptyTitle={t('accountPage.notifications.emptyDeliveries')}
                framed={false}
                items={deliveries.data?.items ?? []}
                loading={deliveries.isLoading}
                pagination={pageFor(deliveries.data, deliveryPage, setDeliveryPage, t)}
                rowKey={item => item.id}
                onRowClick={item => setSelectedDeliveryId(item.id)}
              />
            )}
      </Section>

      {channelDialog && (
        <ChannelDialog
          key={channelDialog.channel?.id ?? 'new-personal-channel'}
          channel={channelDialog.channel}
          presets={presets.data ?? []}
          saving={saveChannel.isPending}
          onClose={() => setChannelDialog(null)}
          onSave={request => saveChannel.mutate(request)}
        />
      )}

      <NotificationDeliveryDetailSheet
        canRetry
        delivery={selectedDelivery}
        retrying={retryDelivery.isPending}
        onClose={() => setSelectedDeliveryId(null)}
        onRetry={delivery => retryDelivery.mutate(delivery)}
      />

      <ConfirmDialog
        description={t('accountPage.notifications.deleteConfirmDescription', { name: deleteTarget?.name ?? '' })}
        open={Boolean(deleteTarget)}
        pending={deleteChannel.isPending}
        title={t('accountPage.notifications.deleteConfirmTitle')}
        onConfirm={() => deleteTarget && deleteChannel.mutateAsync(deleteTarget.id)}
        onOpenChange={open => !open && setDeleteTarget(null)}
      />
    </div>
  )
}

function ChannelDialog({ channel, presets, saving, onClose, onSave }: {
  channel?: MyNotificationChannel
  presets: MyNotificationPreset[]
  saving: boolean
  onClose: () => void
  onSave: (request: SaveChannelRequest) => void
}) {
  const { t } = useTranslation()
  const form = useForm<ChannelForm>({
    defaultValues: channelFormValues(channel, presets),
    mode: 'onChange',
    resolver: zodResolver(channelSchema),
  })
  const presetId = form.watch('presetId')
  const selectedPreset = presets.find(item => item.id === presetId)
  const supportsSignSecret = Boolean(
    selectedPreset?.optionalSecretFields?.includes('signSecret')
    || channel?.secretSet?.signSecret,
  )

  const submit = (values: ChannelForm) => {
    const secrets: Record<string, string> = {}
    if (values.webhookUrl.trim())
      secrets.webhookUrl = values.webhookUrl.trim()
    if (values.signSecret.trim())
      secrets.signSecret = values.signSecret.trim()

    if (!channel && !selectedPreset) {
      toast.error(t('accountPage.notifications.presetRequired'))
      return
    }
    if (!channel && selectedPreset?.secretFields.some(field => !secrets[field])) {
      toast.error(t('accountPage.notifications.secretRequired'))
      return
    }
    const payload = {
      name: values.name.trim(),
      secrets,
      enabled: values.enabled,
    }
    if (channel) {
      onSave({ mode: 'update', channelId: channel.id, payload })
      return
    }
    onSave({ mode: 'create', payload: { ...payload, presetId: values.presetId } })
  }

  return (
    <Dialog open onOpenChange={open => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{channel ? t('accountPage.notifications.editChannelTitle') : t('accountPage.notifications.createChannelTitle')}</DialogTitle>
          <DialogDescription>{t('accountPage.notifications.channelDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-4" onSubmit={form.handleSubmit(submit)}>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field error={form.formState.errors.name?.message} label={t('accountPage.notifications.channelName')} required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} />
            </Field>
            {!channel && (
              <Field label={t('accountPage.notifications.preset')} required>
                <NativeSelect
                  value={presetId}
                  onChange={(event) => {
                    form.setValue('presetId', event.target.value, { shouldDirty: true, shouldValidate: true })
                    form.setValue('webhookUrl', '', { shouldDirty: true })
                    form.setValue('signSecret', '', { shouldDirty: true })
                  }}
                >
                  {presets.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
                </NativeSelect>
              </Field>
            )}
          </div>
          <Field
            hint={channel ? t('accountPage.notifications.webhookUrlEditHint') : undefined}
            label={t('accountPage.notifications.webhookUrl')}
            required={!channel}
          >
            <Input
              {...form.register('webhookUrl')}
              autoComplete="off"
              placeholder="https://"
              type="url"
            />
          </Field>
          {supportsSignSecret && (
            <Field
              hint={t('accountPage.notifications.signSecretHint')}
              label={t('accountPage.notifications.signSecret')}
            >
              <Input
                {...form.register('signSecret')}
                autoComplete="new-password"
                type="password"
              />
            </Field>
          )}
          <Controller
            control={form.control}
            name="enabled"
            render={({ field }) => <ControlledCheckboxField field={field}>{t('accountPage.notifications.channelEnabled')}</ControlledCheckboxField>}
          />
          <DialogFooter>
            <Button disabled={saving} type="button" variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
            <Button disabled={saving || !form.formState.isValid} type="submit">{channel ? t('common.save') : t('accountPage.notifications.createChannelTitle')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function SectionTitle({ description, title }: { description: string, title: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      {title}
      <HelpTip label={title}>{description}</HelpTip>
    </span>
  )
}

function channelFormValues(channel: MyNotificationChannel | undefined, presets: MyNotificationPreset[]): ChannelForm {
  const presetId = channel?.presetId || presets[0]?.id || ''
  return {
    name: channel?.name ?? '',
    presetId,
    webhookUrl: '',
    signSecret: '',
    enabled: channel?.enabled ?? true,
  }
}

function eventTypeLabel(eventType: string, t: TFunction) {
  switch (eventType) {
    case 'build.failed': return t('accountPage.notifications.events.buildFailed')
    case 'build.webhook_rejected': return t('accountPage.notifications.events.buildWebhookRejected')
    case 'release.failed': return t('accountPage.notifications.events.releaseFailed')
    case 'runtime.pod_oom_killed': return t('accountPage.notifications.events.runtimePodOomKilled')
    case 'runtime.pod_crash_loop': return t('accountPage.notifications.events.runtimePodCrashLoop')
    case 'runtime.pod_start_failed': return t('accountPage.notifications.events.runtimePodStartFailed')
    case 'runtime.workload_degraded': return t('accountPage.notifications.events.runtimeWorkloadDegraded')
    case 'runtime.recovered': return t('accountPage.notifications.events.runtimeRecovered')
    case 'hook.failed': return t('accountPage.notifications.events.hookFailed')
    case 'gateway.apply_failed': return t('accountPage.notifications.events.gatewayApplyFailed')
    case 'certificate.failed': return t('accountPage.notifications.events.certificateFailed')
    case 'certificate.expired': return t('accountPage.notifications.events.certificateExpired')
    default: return eventType
  }
}

function pageFor(
  data: { page: number, pageSize: number, total: number, totalPages: number } | undefined,
  page: number,
  onPageChange: (page: number) => void,
  t: TFunction,
) {
  if (!data || data.total === 0)
    return undefined
  return {
    page: data.page ?? page,
    pageSize: data.pageSize ?? pageSize,
    total: data.total,
    totalPages: data.totalPages,
    pageInfoLabel: t('accountPage.notifications.pageInfo', { page: data.page, totalPages: data.totalPages, total: data.total }),
    onPageChange,
  }
}
