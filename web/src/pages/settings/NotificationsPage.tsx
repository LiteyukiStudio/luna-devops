import type { ReactNode } from 'react'
import type { NotificationChannel, NotificationChannelPayload, NotificationDelivery, NotificationRule, NotificationRulePayload, NotificationTemplate, NotificationTemplatePayload } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, FlaskConical, Pencil, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

const PAGE_SIZE = 10
const FAILURE_EVENTS = ['build.failed', 'release.failed', 'hook.failed', 'gateway.apply_failed']

export function NotificationsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('channels')
  const [channelPage, setChannelPage] = useState(1)
  const [templatePage, setTemplatePage] = useState(1)
  const [rulePage, setRulePage] = useState(1)
  const [deliveryPage, setDeliveryPage] = useState(1)
  const [channelDialog, setChannelDialog] = useState<ChannelFormState | null>(null)
  const [presetDialog, setPresetDialog] = useState<PresetFormState | null>(null)
  const [templateDialog, setTemplateDialog] = useState<TemplateFormState | null>(null)
  const [ruleDialog, setRuleDialog] = useState<RuleFormState | null>(null)
  const [testChannelTarget, setTestChannelTarget] = useState<NotificationChannel | null>(null)

  const presets = useQuery({ queryKey: ['notifications', 'presets'], queryFn: api.listNotificationPresets })
  const channels = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['notifications', 'channels', channelPage],
    queryFn: () => api.listNotificationChannels({ page: channelPage, pageSize: PAGE_SIZE, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const channelOptions = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['notifications', 'channels', 'options'],
    queryFn: () => api.listNotificationChannels({ page: 1, pageSize: 100, sortBy: 'name', sortOrder: 'asc' }),
  })
  const templates = useQuery({
    queryKey: ['notifications', 'templates', templatePage],
    queryFn: () => api.listNotificationTemplates({ page: templatePage, pageSize: PAGE_SIZE, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const templateOptions = useQuery({
    queryKey: ['notifications', 'templates', 'options'],
    queryFn: () => api.listNotificationTemplates({ page: 1, pageSize: 100, sortBy: 'name', sortOrder: 'asc' }),
  })
  const rules = useQuery({
    queryKey: ['notifications', 'rules', rulePage],
    queryFn: () => api.listNotificationRules({ page: rulePage, pageSize: PAGE_SIZE, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const deliveries = useQuery({
    queryKey: ['notifications', 'deliveries', deliveryPage],
    queryFn: () => api.listNotificationDeliveries({ page: deliveryPage, pageSize: PAGE_SIZE, sortBy: 'createdAt', sortOrder: 'desc' }),
  })

  const openPresetDialog = () => setPresetDialog(emptyPresetState(presets.data?.[0]?.id ?? ''))
  const refreshNotifications = () => queryClient.invalidateQueries({ queryKey: ['notifications'] })
  const saveChannel = useMutation({
    mutationFn: (state: ChannelFormState) => {
      const payload: NotificationChannelPayload = {
        adapterKind: state.adapterKind,
        config: parseJSON(state.configText, {}),
        enabled: state.enabled,
        name: state.name,
        secrets: parseKeyValueLines(state.secretText),
      }
      return state.id ? api.updateNotificationChannel(state.id, payload) : api.createNotificationChannel(payload)
    },
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setChannelDialog(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })
  const createPreset = useMutation({
    mutationFn: (state: PresetFormState) => api.createNotificationChannelFromPreset(state.presetId, {
      enabled: true,
      name: state.name,
      secrets: parseKeyValueLines(state.secretText),
    }),
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setPresetDialog(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })
  const saveTemplate = useMutation({
    mutationFn: (state: TemplateFormState) => {
      const payload: NotificationTemplatePayload = {
        adapterKind: state.adapterKind,
        bodyTemplate: state.bodyTemplate,
        enabled: state.enabled,
        eventType: state.eventType,
        jsonBodyTemplate: state.jsonBodyTemplate,
        locale: state.locale,
        name: state.name,
        subjectTemplate: state.subjectTemplate,
      }
      return state.id ? api.updateNotificationTemplate(state.id, payload) : api.createNotificationTemplate(payload)
    },
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setTemplateDialog(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })
  const saveRule = useMutation({
    mutationFn: (state: RuleFormState) => {
      const payload: NotificationRulePayload = {
        channelIds: state.channelIds,
        enabled: state.enabled,
        eventTypes: state.eventTypes,
        filter: parseJSON(state.filterText, {}),
        locale: state.locale,
        name: state.name,
        templateId: state.templateId,
      }
      return state.id ? api.updateNotificationRule(state.id, payload) : api.createNotificationRule(payload)
    },
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setRuleDialog(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })
  const deleteChannel = useMutation({
    mutationFn: api.deleteNotificationChannel,
    onSuccess: refreshNotifications,
    onError: error => toast.error(error.message),
  })
  const deleteTemplate = useMutation({
    mutationFn: api.deleteNotificationTemplate,
    onSuccess: refreshNotifications,
    onError: error => toast.error(error.message),
  })
  const deleteRule = useMutation({
    mutationFn: api.deleteNotificationRule,
    onSuccess: refreshNotifications,
    onError: error => toast.error(error.message),
  })
  const testChannel = useMutation({
    mutationFn: api.testNotificationChannel,
    onSuccess: () => {
      toast.success(t('notificationsPage.testSent'))
      setTestChannelTarget(null)
      refreshNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const channelColumns = useMemo<DataListColumn<NotificationChannel>[]>(() => [
    { key: 'name', header: t('notificationsPage.name'), width: 'primary', render: item => <NameCell name={item.name} sub={item.adapterKind} /> },
    { key: 'enabled', header: t('common.status'), width: 'status', render: item => <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>{item.enabled ? t('common.enabled') : t('common.disabled')}</StatusBadge> },
    { key: 'lastDeliveryStatus', header: t('notificationsPage.lastDelivery'), width: 'status', render: item => item.lastDeliveryStatus ? <StatusValueBadge value={item.lastDeliveryStatus} /> : '-' },
    { key: 'secretSet', header: t('notificationsPage.secrets'), width: 'normal', render: item => Object.keys(item.secretSet ?? {}).filter(key => item.secretSet?.[key]).join(', ') || '-' },
    { key: 'actions', header: t('common.actions'), sticky: 'right', render: item => <Actions onDelete={() => deleteChannel.mutate(item.id)} onEdit={() => setChannelDialog(channelStateFromItem(item))} onTest={() => setTestChannelTarget(item)} /> },
  ], [deleteChannel, t])

  const templateColumns = useMemo<DataListColumn<NotificationTemplate>[]>(() => [
    { key: 'name', header: t('notificationsPage.name'), width: 'primary', render: item => <NameCell name={item.name} sub={`${item.eventType} · ${item.adapterKind}`} /> },
    { key: 'enabled', header: t('common.status'), width: 'status', render: item => <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>{item.enabled ? t('common.enabled') : t('common.disabled')}</StatusBadge> },
    { key: 'locale', header: t('notificationsPage.locale'), width: 'compact', render: item => item.locale || '-' },
    { key: 'actions', header: t('common.actions'), sticky: 'right', render: item => <Actions onDelete={() => deleteTemplate.mutate(item.id)} onEdit={() => setTemplateDialog(templateStateFromItem(item))} /> },
  ], [deleteTemplate, t])

  const ruleColumns = useMemo<DataListColumn<NotificationRule>[]>(() => [
    { key: 'name', header: t('notificationsPage.name'), width: 'primary', render: item => <NameCell name={item.name} sub={decodeStringList(item.eventTypesJson).join(', ')} /> },
    { key: 'enabled', header: t('common.status'), width: 'status', render: item => <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>{item.enabled ? t('common.enabled') : t('common.disabled')}</StatusBadge> },
    { key: 'channels', header: t('notificationsPage.channels'), width: 'normal', render: item => decodeStringList(item.channelIdsJson).length },
    { key: 'actions', header: t('common.actions'), sticky: 'right', render: item => <Actions onDelete={() => deleteRule.mutate(item.id)} onEdit={() => setRuleDialog(ruleStateFromItem(item))} /> },
  ], [deleteRule, t])

  const deliveryColumns = useMemo<DataListColumn<NotificationDelivery>[]>(() => [
    { key: 'eventType', header: t('notificationsPage.eventType'), width: 'normal', render: item => <NameCell name={item.eventType} sub={item.eventId} /> },
    { key: 'status', header: t('common.status'), width: 'status', render: item => <StatusValueBadge value={item.status} /> },
    { key: 'attemptCount', header: t('notificationsPage.attempts'), width: 'number', render: item => item.attemptCount },
    { key: 'errorMessage', header: t('notificationsPage.error'), width: 'normal', render: item => item.errorMessage || '-' },
  ], [t])

  return (
    <div className="grid gap-5">
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
                <Button id="notification-channel-templates" variant="outline" onClick={openPresetDialog}>
                  <Bell className="size-4" />
                  {t('notificationsPage.fromPreset')}
                </Button>
                <Button onClick={() => setChannelDialog(emptyChannelState())}>
                  <Plus className="size-4" />
                  {t('notificationsPage.createChannel')}
                </Button>
              </>
            )}
            {activeTab === 'templates' && (
              <Button onClick={() => setTemplateDialog(emptyTemplateState())}>
                <Plus className="size-4" />
                {t('notificationsPage.createTemplate')}
              </Button>
            )}
            {activeTab === 'rules' && (
              <Button onClick={() => setRuleDialog(emptyRuleState())}>
                <Plus className="size-4" />
                {t('notificationsPage.createRule')}
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
            emptyTitle={t('notificationsPage.emptyChannels')}
            items={channels.data?.items ?? []}
            pagination={pagination(channels.data, channelPage, setChannelPage, t('notificationsPage.pageInfo', { page: channels.data?.page ?? channelPage, totalPages: channels.data?.totalPages ?? 1, total: channels.data?.total ?? 0 }))}
            rowKey={item => item.id}
          />
        </TabsContent>
        <TabsContent value="templates">
          <DataList columns={templateColumns} emptyTitle={t('notificationsPage.emptyTemplates')} items={templates.data?.items ?? []} pagination={pagination(templates.data, templatePage, setTemplatePage, t('notificationsPage.pageInfo', { page: templates.data?.page ?? templatePage, totalPages: templates.data?.totalPages ?? 1, total: templates.data?.total ?? 0 }))} rowKey={item => item.id} />
        </TabsContent>
        <TabsContent value="rules">
          <DataList columns={ruleColumns} emptyTitle={t('notificationsPage.emptyRules')} items={rules.data?.items ?? []} pagination={pagination(rules.data, rulePage, setRulePage, t('notificationsPage.pageInfo', { page: rules.data?.page ?? rulePage, totalPages: rules.data?.totalPages ?? 1, total: rules.data?.total ?? 0 }))} rowKey={item => item.id} />
        </TabsContent>
        <TabsContent value="deliveries">
          <DataList columns={deliveryColumns} emptyTitle={t('notificationsPage.emptyDeliveries')} items={deliveries.data?.items ?? []} pagination={pagination(deliveries.data, deliveryPage, setDeliveryPage, t('notificationsPage.pageInfo', { page: deliveries.data?.page ?? deliveryPage, totalPages: deliveries.data?.totalPages ?? 1, total: deliveries.data?.total ?? 0 }))} rowKey={item => item.id} />
        </TabsContent>
      </ContentTabs>

      <ChannelDialog openState={channelDialog} saving={saveChannel.isPending} onClose={() => setChannelDialog(null)} onSave={state => saveChannel.mutate(state)} onUpdate={setChannelDialog} />
      <PresetDialog openState={presetDialog} presets={presets.data ?? []} saving={createPreset.isPending} onClose={() => setPresetDialog(null)} onSave={state => createPreset.mutate(state)} onUpdate={setPresetDialog} />
      <TemplateDialog openState={templateDialog} saving={saveTemplate.isPending} onClose={() => setTemplateDialog(null)} onSave={state => saveTemplate.mutate(state)} onUpdate={setTemplateDialog} />
      <RuleDialog channels={channelOptions.data?.items ?? []} openState={ruleDialog} saving={saveRule.isPending} templates={templateOptions.data?.items ?? []} onClose={() => setRuleDialog(null)} onSave={state => saveRule.mutate(state)} onUpdate={setRuleDialog} />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('notificationsPage.sendTest')}
        confirmVariant="default"
        content={<TestNotificationVariables />}
        description={t('notificationsPage.testChannelDescription', { name: testChannelTarget?.name ?? '' })}
        open={Boolean(testChannelTarget)}
        pending={testChannel.isPending}
        title={t('notificationsPage.testChannelTitle')}
        onConfirm={() => testChannelTarget && testChannel.mutate(testChannelTarget.id)}
        onOpenChange={open => !open && setTestChannelTarget(null)}
      />
    </div>
  )
}

interface ChannelFormState { adapterKind: string, configText: string, enabled: boolean, id?: string, name: string, secretKeys: string[], secretText: string }
interface PresetFormState { enabled: boolean, name: string, presetId: string, secretText: string }
interface TemplateFormState { adapterKind: string, bodyTemplate: string, enabled: boolean, eventType: string, id?: string, jsonBodyTemplate: string, locale: string, name: string, subjectTemplate: string }
interface RuleFormState { channelIds: string[], enabled: boolean, eventTypes: string[], filterText: string, id?: string, locale: string, name: string, templateId: string }

function ChannelDialog({ onClose, onSave, onUpdate, openState, saving }: { onClose: () => void, onSave: (state: ChannelFormState) => void, onUpdate: (state: ChannelFormState) => void, openState: ChannelFormState | null, saving: boolean }) {
  const { t } = useTranslation()
  if (!openState)
    return null
  const secretPlaceholder = openState.secretKeys.length > 0
    ? openState.secretKeys.map(key => `${key}=${t('common.secretSetPlaceholder')}`).join('\n')
    : t('notificationsPage.secretPlaceholder')
  return (
    <Dialog open onOpenChange={open => !open && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('notificationsPage.channelDialog')}</DialogTitle>
          <DialogDescription>{t('notificationsPage.channelDialogDescription')}</DialogDescription>
        </DialogHeader>
        <FormGrid>
          <Field label={t('notificationsPage.name')}><Input value={openState.name} onChange={event => onUpdate({ ...openState, name: event.target.value })} /></Field>
          <Field label={t('notificationsPage.adapter')}>
            <Select value={openState.adapterKind} onChange={event => onUpdate({ ...openState, adapterKind: event.target.value })}>
              <option value="webhook">{t('notificationsPage.adapters.webhook')}</option>
              <option value="smtp">{t('notificationsPage.adapters.smtp')}</option>
            </Select>
          </Field>
          <Field wide label={t('notificationsPage.configJson')}><Textarea className="min-h-40 font-mono" value={openState.configText} onChange={event => onUpdate({ ...openState, configText: event.target.value })} /></Field>
          <Field wide label={t('notificationsPage.secretLines')}><Textarea className="min-h-24 font-mono" placeholder={secretPlaceholder} value={openState.secretText} onChange={event => onUpdate({ ...openState, secretText: event.target.value })} /></Field>
        </FormGrid>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={saving} onClick={() => onSave(openState)}>{t('common.save')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PresetDialog({ onClose, onSave, onUpdate, openState, presets, saving }: { onClose: () => void, onSave: (state: PresetFormState) => void, onUpdate: (state: PresetFormState) => void, openState: PresetFormState | null, presets: Array<{ id: string, name: string, secretFields: string[] }>, saving: boolean }) {
  const { t } = useTranslation()
  if (!openState)
    return null
  const preset = presets.find(item => item.id === openState.presetId)
  return (
    <Dialog open onOpenChange={open => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('notificationsPage.presetDialog')}</DialogTitle>
          <DialogDescription>{t('notificationsPage.presetDialogDescription')}</DialogDescription>
        </DialogHeader>
        <FormGrid>
          <Field label={t('notificationsPage.preset')}><Select value={openState.presetId} onChange={event => onUpdate({ ...openState, presetId: event.target.value })}>{presets.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</Select></Field>
          <Field label={t('notificationsPage.name')}><Input value={openState.name} onChange={event => onUpdate({ ...openState, name: event.target.value })} /></Field>
          <Field wide label={t('notificationsPage.secretLines')}><Textarea className="font-mono" placeholder={(preset?.secretFields ?? []).map(field => `${field}=`).join('\n')} value={openState.secretText} onChange={event => onUpdate({ ...openState, secretText: event.target.value })} /></Field>
        </FormGrid>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={saving || !openState.presetId} onClick={() => onSave(openState)}>{t('common.create')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function TemplateDialog({ onClose, onSave, onUpdate, openState, saving }: { onClose: () => void, onSave: (state: TemplateFormState) => void, onUpdate: (state: TemplateFormState) => void, openState: TemplateFormState | null, saving: boolean }) {
  const { t } = useTranslation()
  if (!openState)
    return null
  return (
    <Dialog open onOpenChange={open => !open && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('notificationsPage.templateDialog')}</DialogTitle>
          <DialogDescription>{t('notificationsPage.templateDialogDescription')}</DialogDescription>
        </DialogHeader>
        <FormGrid>
          <Field label={t('notificationsPage.name')}><Input value={openState.name} onChange={event => onUpdate({ ...openState, name: event.target.value })} /></Field>
          <Field label={t('notificationsPage.eventType')}><Select value={openState.eventType} onChange={event => onUpdate({ ...openState, eventType: event.target.value })}>{FAILURE_EVENTS.map(event => <option key={event} value={event}>{event}</option>)}</Select></Field>
          <Field label={t('notificationsPage.adapter')}>
            <Select value={openState.adapterKind} onChange={event => onUpdate({ ...openState, adapterKind: event.target.value })}>
              <option value="webhook">{t('notificationsPage.adapters.webhook')}</option>
              <option value="smtp">{t('notificationsPage.adapters.smtp')}</option>
            </Select>
          </Field>
          <Field label={t('notificationsPage.locale')}><Input value={openState.locale} onChange={event => onUpdate({ ...openState, locale: event.target.value })} /></Field>
          <Field wide label={t('notificationsPage.subjectTemplate')}><Input value={openState.subjectTemplate} onChange={event => onUpdate({ ...openState, subjectTemplate: event.target.value })} /></Field>
          <Field wide label={t('notificationsPage.bodyTemplate')}><Textarea className="min-h-28 font-mono" value={openState.bodyTemplate} onChange={event => onUpdate({ ...openState, bodyTemplate: event.target.value })} /></Field>
          <Field wide label={t('notificationsPage.jsonBodyTemplate')}><Textarea className="min-h-40 font-mono" value={openState.jsonBodyTemplate} onChange={event => onUpdate({ ...openState, jsonBodyTemplate: event.target.value })} /></Field>
        </FormGrid>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={saving} onClick={() => onSave(openState)}>{t('common.save')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function RuleDialog({ channels, onClose, onSave, onUpdate, openState, saving, templates }: { channels: NotificationChannel[], onClose: () => void, onSave: (state: RuleFormState) => void, onUpdate: (state: RuleFormState) => void, openState: RuleFormState | null, saving: boolean, templates: NotificationTemplate[] }) {
  const { t } = useTranslation()
  if (!openState)
    return null
  const toggleChannel = (channelId: string, checked: boolean) => onUpdate({ ...openState, channelIds: checked ? [...openState.channelIds, channelId] : openState.channelIds.filter(id => id !== channelId) })
  return (
    <Dialog open onOpenChange={open => !open && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('notificationsPage.ruleDialog')}</DialogTitle>
          <DialogDescription>{t('notificationsPage.ruleDialogDescription')}</DialogDescription>
        </DialogHeader>
        <FormGrid>
          <Field label={t('notificationsPage.name')}><Input value={openState.name} onChange={event => onUpdate({ ...openState, name: event.target.value })} /></Field>
          <Field label={t('notificationsPage.template')}>
            <Select value={openState.templateId} onChange={event => onUpdate({ ...openState, templateId: event.target.value })}>
              <option value="">{t('notificationsPage.defaultTemplate')}</option>
              {templates.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
            </Select>
          </Field>
          <Field wide label={t('notificationsPage.eventTypes')}><Input value={openState.eventTypes.join(', ')} onChange={event => onUpdate({ ...openState, eventTypes: splitList(event.target.value) })} /></Field>
          <Field wide label={t('notificationsPage.filterJson')}><Textarea className="min-h-28 font-mono" value={openState.filterText} onChange={event => onUpdate({ ...openState, filterText: event.target.value })} /></Field>
          <div className="grid gap-2 md:col-span-2">
            <Label>{t('notificationsPage.channels')}</Label>
            <div className="grid gap-2 rounded-lg border border-border p-3">
              {channels.map(channel => (
                <label key={channel.id} className="flex items-center gap-2 text-sm">
                  <input checked={openState.channelIds.includes(channel.id)} type="checkbox" onChange={event => toggleChannel(channel.id, event.target.checked)} />
                  <span>{channel.name}</span>
                </label>
              ))}
            </div>
          </div>
        </FormGrid>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={saving} onClick={() => onSave(openState)}>{t('common.save')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Actions({ onDelete, onEdit, onTest }: { onDelete: () => void, onEdit: () => void, onTest?: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-end gap-1">
      {onTest && <Button aria-label={t('notificationsPage.testChannel')} size="icon" variant="ghost" onClick={onTest}><FlaskConical className="size-4" /></Button>}
      <Button aria-label={t('common.edit')} size="icon" variant="ghost" onClick={onEdit}><Pencil className="size-4" /></Button>
      <Button aria-label={t('common.delete')} size="icon" variant="ghost" onClick={onDelete}><Trash2 className="size-4" /></Button>
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
    '.Event.Build.ID',
    '.Event.Release.ID',
    '.Event.Hook.Name',
    '.Event.Gateway.Domain',
  ]
  return (
    <div className="grid gap-2 rounded-lg border border-border bg-muted/30 p-3 text-sm">
      <p className="text-muted-foreground">{t('notificationsPage.testVariablesIntro')}</p>
      <div className="flex flex-wrap gap-2">
        {variables.map(variable => (
          <code key={variable} className="rounded-md bg-surface px-2 py-1 font-mono text-xs text-foreground">
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

function Field({ children, label, wide }: { children: ReactNode, label: string, wide?: boolean }) {
  return (
    <div className={wide ? 'grid gap-2 md:col-span-2' : 'grid gap-2'}>
      <Label>{label}</Label>
      {children}
    </div>
  )
}

function FormGrid({ children }: { children: ReactNode }) {
  return <div className="grid gap-4 md:grid-cols-2">{children}</div>
}

function emptyChannelState(): ChannelFormState {
  return { adapterKind: 'webhook', configText: '{\n  "method": "POST",\n  "url": "https://example.com/webhook",\n  "headers": {\n    "Content-Type": "application/json"\n  }\n}', enabled: true, name: '', secretKeys: [], secretText: '' }
}

function emptyPresetState(presetId: string): PresetFormState {
  return { enabled: true, name: '', presetId, secretText: '' }
}

function emptyTemplateState(): TemplateFormState {
  return { adapterKind: 'webhook', bodyTemplate: '', enabled: true, eventType: 'build.failed', jsonBodyTemplate: '{\n  "text": "[{{.Event.Severity}}] {{.Event.Type}}\\n{{.Event.Message}}"\n}', locale: '', name: '', subjectTemplate: '[{{.Event.Severity}}] {{.Event.Type}}' }
}

function emptyRuleState(): RuleFormState {
  return { channelIds: [], enabled: true, eventTypes: [...FAILURE_EVENTS], filterText: '{}', locale: '', name: '', templateId: '' }
}

function channelStateFromItem(item: NotificationChannel): ChannelFormState {
  const secretKeys = Object.keys(item.secretSet ?? {}).filter(key => item.secretSet?.[key])
  return { adapterKind: item.adapterKind, configText: JSON.stringify(item.config ?? parseJSON(item.configJson, {}), null, 2), enabled: item.enabled, id: item.id, name: item.name, secretKeys, secretText: '' }
}

function templateStateFromItem(item: NotificationTemplate): TemplateFormState {
  return { adapterKind: item.adapterKind, bodyTemplate: item.bodyTemplate, enabled: item.enabled, eventType: item.eventType, id: item.id, jsonBodyTemplate: item.jsonBodyTemplate, locale: item.locale, name: item.name, subjectTemplate: item.subjectTemplate }
}

function ruleStateFromItem(item: NotificationRule): RuleFormState {
  return { channelIds: decodeStringList(item.channelIdsJson), enabled: item.enabled, eventTypes: decodeStringList(item.eventTypesJson), filterText: JSON.stringify(parseJSON(item.filterJson, {}), null, 2), id: item.id, locale: item.locale, name: item.name, templateId: item.templateId }
}

function parseJSON<T>(value: string | unknown, fallback: T): T {
  if (typeof value !== 'string')
    return (value as T) ?? fallback
  try {
    return JSON.parse(value) as T
  }
  catch {
    return fallback
  }
}

function parseKeyValueLines(value: string) {
  return Object.fromEntries(value.split('\n').map((line) => {
    const index = line.indexOf('=')
    return index < 0 ? ['', ''] : [line.slice(0, index).trim(), line.slice(index + 1).trim()]
  }).filter(([key, val]) => key && val))
}

function splitList(value: string) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}

function decodeStringList(value: string) {
  return parseJSON<string[]>(value, [])
}

function pagination(data: { page?: number, pageSize?: number, total?: number, totalPages?: number } | undefined, page: number, onPageChange: (page: number) => void, pageInfoLabel: string) {
  return { page: data?.page ?? page, pageInfoLabel, pageSize: data?.pageSize ?? PAGE_SIZE, total: data?.total ?? 0, totalPages: data?.totalPages ?? 1, onPageChange }
}
