import type { TFunction } from 'i18next'
import type { MyNotificationChannel, MyNotificationChannelCreatePayload, MyNotificationChannelUpdatePayload, MyNotificationPreset, NotificationDelivery } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { FlaskConical, Pencil, Plus, Save, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { CheckboxField } from '@/components/common/checkbox-field'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormActions } from '@/components/common/form-actions'
import { FormField as Field } from '@/components/common/form-field'
import { Section } from '@/components/common/section'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'

const pageSize = 10
const failureEventTypes = [
  'build.failed',
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
  name: z.string().trim().min(1, i18next.t('accountPage.notifications.channelNameRequired')),
  presetId: z.string(),
  secretText: z.string(),
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

  const preferences = useQuery({ queryKey: ['my-notification-preferences'], queryFn: api.getMyNotificationPreferences })
  const presets = useQuery({ queryKey: ['my-notification-presets'], queryFn: api.listMyNotificationPresets })
  const channels = useQuery({
    queryKey: ['my-notification-channels', channelPage],
    queryFn: () => api.listMyNotificationChannels({ page: channelPage, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const deliveries = useQuery({
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
      render: item => (
        <div className="flex items-center gap-1">
          <Button aria-label={t('accountPage.notifications.testChannel', { name: item.name })} disabled={testChannel.isPending} size="icon" variant="ghost" onClick={() => testChannel.mutate(item.id)}><FlaskConical className="size-4" /></Button>
          <Button aria-label={t('accountPage.notifications.editChannel', { name: item.name })} size="icon" variant="ghost" onClick={() => setChannelDialog({ channel: item })}><Pencil className="size-4" /></Button>
          <Button aria-label={t('accountPage.notifications.deleteChannel', { name: item.name })} size="icon" variant="ghost" onClick={() => setDeleteTarget(item)}><Trash2 className="size-4" /></Button>
        </div>
      ),
    },
  ], [t, testChannel])
  const deliveryColumns = useMemo<DataListColumn<NotificationDelivery>[]>(() => [
    { key: 'eventType', header: t('accountPage.notifications.eventType'), width: 'primary', render: item => <span className="font-medium">{eventTypeLabel(item.eventType, t)}</span> },
    { key: 'status', header: t('common.status'), width: 'status', render: item => <StatusValueBadge value={item.status} /> },
    { key: 'attemptCount', header: t('accountPage.notifications.attempts'), width: 'number', render: item => item.attemptCount },
    { key: 'createdAt', header: t('accountPage.notifications.deliveredAt'), width: 'compact', render: item => formatSmartDateTime(item.finishedAt ?? item.createdAt, t) },
  ], [t])

  if (preferences.isError)
    return <ErrorState title={t('accountPage.notifications.loadFailedTitle')} description={t('accountPage.notifications.loadFailedDescription')} />

  return (
    <div className="grid gap-4">
      <Section description={t('accountPage.notifications.preferencesDescription')} title={t('accountPage.notifications.preferencesTitle')} variant="bordered">
        <form className="grid gap-4" onSubmit={preferenceForm.handleSubmit(values => savePreferences.mutate(values))}>
          <Field hint={t('accountPage.notifications.accountEmailHint')} label={t('accountPage.notifications.accountEmail')}>
            <Input readOnly value={user?.email ?? ''} type="email" />
          </Field>
          <CheckboxField description={t('accountPage.notifications.emailEnabledHint')} {...preferenceForm.register('emailEnabled')}>
            {t('accountPage.notifications.emailEnabled')}
          </CheckboxField>
          <fieldset className="grid gap-3 rounded-container bg-surface-inset p-4 sm:grid-cols-2">
            <legend className="px-1 text-sm font-medium">{t('accountPage.notifications.failureEvents')}</legend>
            {failureEventTypes.map(eventType => (
              <CheckboxField key={eventType} value={eventType} {...preferenceForm.register('eventTypes')}>
                {eventTypeLabel(eventType, t)}
              </CheckboxField>
            ))}
          </fieldset>
          <FormActions>
            <Button disabled={savePreferences.isPending || !preferenceForm.formState.isDirty || !preferenceForm.formState.isValid} type="submit">
              <Save className="size-4" />
              {t('accountPage.notifications.savePreferences')}
            </Button>
          </FormActions>
        </form>
      </Section>

      <Section
        description={t('accountPage.notifications.channelsDescription')}
        title={t('accountPage.notifications.channelsTitle')}
        tools={(
          <Button disabled={presets.isLoading || presets.isError || !presets.data?.length} type="button" variant="outline" onClick={() => setChannelDialog({})}>
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
                items={channels.data?.items ?? []}
                loading={channels.isLoading}
                pagination={pageFor(channels.data, channelPage, setChannelPage, t)}
                rowKey={item => item.id}
              />
            )}
      </Section>

      <Section description={t('accountPage.notifications.deliveriesDescription')} title={t('accountPage.notifications.deliveriesTitle')} variant="bordered">
        {deliveries.isError
          ? <ErrorState title={t('accountPage.notifications.deliveriesLoadFailedTitle')} description={t('accountPage.notifications.deliveriesLoadFailedDescription')} />
          : (
              <DataList
                columns={deliveryColumns}
                emptyTitle={t('accountPage.notifications.emptyDeliveries')}
                items={deliveries.data?.items ?? []}
                loading={deliveries.isLoading}
                pagination={pageFor(deliveries.data, deliveryPage, setDeliveryPage, t)}
                rowKey={item => item.id}
              />
            )}
      </Section>

      {channelDialog && (
        <ChannelDialog
          channel={channelDialog.channel}
          presets={presets.data ?? []}
          saving={saveChannel.isPending}
          onClose={() => setChannelDialog(null)}
          onSave={request => saveChannel.mutate(request)}
        />
      )}
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
  const secretFields = selectedPreset?.secretFields ?? Object.keys(channel?.secretSet ?? {})

  const submit = (values: ChannelForm) => {
    const secrets = parseSecretLines(values.secretText)
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
      <DialogContent className="max-w-3xl">
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
                    form.setValue('secretText', '', { shouldDirty: true })
                  }}
                >
                  {presets.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
                </NativeSelect>
              </Field>
            )}
          </div>
          <Field
            hint={channel ? t('accountPage.notifications.secretEditHint') : t('accountPage.notifications.secretHint')}
            label={t('accountPage.notifications.secrets')}
            required={!channel && secretFields.length > 0}
          >
            <Textarea
              {...form.register('secretText')}
              className="min-h-24 font-mono text-sm"
              placeholder={secretFields.map(field => `${field}=`).join('\n')}
            />
          </Field>
          <CheckboxField {...form.register('enabled')}>{t('accountPage.notifications.channelEnabled')}</CheckboxField>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
            <Button disabled={saving || !form.formState.isValid} type="submit">{t('common.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function channelFormValues(channel: MyNotificationChannel | undefined, presets: MyNotificationPreset[]): ChannelForm {
  const preset = presets[0]
  return {
    name: channel?.name ?? '',
    presetId: preset?.id ?? '',
    secretText: '',
    enabled: channel?.enabled ?? true,
  }
}

function eventTypeLabel(eventType: string, t: TFunction) {
  switch (eventType) {
    case 'build.failed': return t('accountPage.notifications.events.buildFailed')
    case 'release.failed': return t('accountPage.notifications.events.releaseFailed')
    case 'hook.failed': return t('accountPage.notifications.events.hookFailed')
    case 'gateway.apply_failed': return t('accountPage.notifications.events.gatewayApplyFailed')
    case 'certificate.failed': return t('accountPage.notifications.events.certificateFailed')
    case 'certificate.expired': return t('accountPage.notifications.events.certificateExpired')
    default: return eventType
  }
}

function parseSecretLines(value: string) {
  return Object.fromEntries(value.split('\n').map((line) => {
    const index = line.indexOf('=')
    return index < 0 ? ['', ''] : [line.slice(0, index).trim(), line.slice(index + 1).trim()]
  }).filter(([key, secret]) => key && secret))
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
